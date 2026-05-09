package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"github.com/aibank/platform/services/document-service/internal/domain"
	"github.com/aibank/platform/services/document-service/internal/repository"
	"github.com/aibank/platform/services/document-service/internal/storage"
)

// MaxFileSize — жёсткий потолок одного загружаемого файла.
const MaxFileSize int64 = 25 * 1024 * 1024

// validTenantID — синхронизировано с repository.validTenantID (ADR-0002).
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// allowedMimeTypes — whitelist форматов документов.
// Иные типы (например, application/zip) явно отклоняются.
var allowedMimeTypes = map[string]struct{}{
	"application/pdf": {},
	"image/png":       {},
	"image/jpeg":      {},
	"image/tiff":      {},
}

// DocumentHandler — HTTP-эндпоинты документ-сервиса.
type DocumentHandler struct {
	repo        domain.DocumentRepository
	storage     storage.Storage
	auditClient *auditsdk.Client
	log         *slog.Logger
}

// NewDocumentHandler собирает handler с инжекцией зависимостей.
// auditClient может быть nil — в таком случае audit-эмит пропускается
// (best-effort через ADR-0010).
func NewDocumentHandler(
	repo domain.DocumentRepository,
	st storage.Storage,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *DocumentHandler {
	return &DocumentHandler{repo: repo, storage: st, auditClient: auditClient, log: log}
}

// withPlatformAuth прикладывает AuthInfo для платформенных операций.
// document-service пока не интегрирован с JWT (ADR-0010 follow-up).
// TenantID берётся из X-Tenant-ID или формы (для multipart — позже),
// ActorID из X-Actor-ID или "system".
func withPlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID := r.Header.Get("X-Actor-ID")
		if actorID == "" {
			actorID = "system"
		}
		ctx := auditsdk.WithAuthInfo(r.Context(), auditsdk.AuthInfo{
			TenantID:  r.Header.Get("X-Tenant-ID"),
			ActorID:   actorID,
			ActorType: auditsdk.ActorTypeSystem,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Routes регистрирует маршруты под /v1/documents.
func (h *DocumentHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "document.uploaded",
			h.resolveDocumentUpload, h.log)).Post("/", h.UploadDocument)
	} else {
		r.Post("/", h.UploadDocument)
	}
	r.Get("/", h.ListDocuments)
	r.Get("/checklist", h.Checklist)
	r.Get("/{id}", h.GetDocument)
	r.Get("/{id}/content", h.GetContent)

	// PATCH /v1/documents/{id}/state — смена состояния документа
	// (parsed | rejected). Мутирующая операция → audit-emit обязателен по
	// ADR-0010 (event_type=document.state_changed).
	if h.auditClient != nil {
		r.With(auditsdk.EmitOnSuccess(h.auditClient, "document.state_changed",
			h.resolveDocumentStateChange, h.log)).Patch("/{id}/state", h.UpdateState)
	} else {
		r.Patch("/{id}/state", h.UpdateState)
	}
	return r
}

// resolveDocumentUpload — EntityResolver для POST /v1/documents.
// На post-этапе тенант уже известен из формы; ID документа handler
// положил в context.
func (h *DocumentHandler) resolveDocumentUpload(r *http.Request) (string, string, json.RawMessage, bool) {
	id := documentIDFromContext(r.Context())
	if id == "" {
		return "", "", nil, false
	}
	tenantID := tenantIDFromContext(r.Context())
	payload, _ := json.Marshal(map[string]any{
		"document_id": id,
		"tenant_id":   tenantID,
	})
	return "document", id, payload, true
}

// resolveDocumentStateChange — EntityResolver для PATCH /v1/documents/{id}/state.
// Используется как fallback, если handler не вызвал auditsdk.SetEntity
// (например, валидация прошла, но dom-операция вернула ErrNotFound — тогда
// emit пропускается через 4xx-статус).
func (h *DocumentHandler) resolveDocumentStateChange(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"document_id": id, "action": "state_changed"})
	return "document", id, payload, true
}

type ctxKey string

const (
	ctxKeyDocumentID ctxKey = "document_id"
	ctxKeyTenantID   ctxKey = "tenant_id"
)

func documentIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyDocumentID).(string)
	return v
}

func tenantIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyTenantID).(string)
	return v
}

// UploadDocument принимает multipart форму, кладёт файл в storage и сохраняет метаданные.
func (h *DocumentHandler) UploadDocument(w http.ResponseWriter, r *http.Request) {
	// Жёсткий лимит на тело запроса. +4KB на multipart-границы и поля формы.
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+4*1024)

	if err := r.ParseMultipartForm(8 * 1024 * 1024); err != nil {
		// http.MaxBytesError превращается в ошибку при ParseMultipartForm.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
				fmt.Sprintf("file exceeds %d bytes", MaxFileSize))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	tenantID := strings.TrimSpace(r.FormValue("tenant_id"))
	applicationID := strings.TrimSpace(r.FormValue("application_id"))
	docTypeStr := strings.TrimSpace(r.FormValue("type"))

	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "missing_tenant", "tenant_id is required")
		return
	}
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
		return
	}
	if applicationID == "" {
		writeError(w, http.StatusBadRequest, "missing_application", "application_id is required")
		return
	}
	docType := domain.DocumentType(docTypeStr)
	if !domain.IsValidDocumentType(docType) {
		writeError(w, http.StatusBadRequest, "invalid_type",
			fmt.Sprintf("unsupported document type %q", docTypeStr))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_file", "file field is required")
		return
	}
	defer file.Close()

	if header.Size > MaxFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			fmt.Sprintf("file exceeds %d bytes", MaxFileSize))
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if _, ok := allowedMimeTypes[mimeType]; !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type",
			fmt.Sprintf("mime type %q is not allowed", mimeType))
		return
	}

	filename := path.Base(header.Filename)
	if filename == "" || filename == "." || filename == "/" {
		writeError(w, http.StatusBadRequest, "invalid_filename", "filename is required")
		return
	}

	fileUUID := uuid.NewString()
	docID := "doc_" + uuid.NewString()
	storageKey := fmt.Sprintf("tenants/%s/applications/%s/%s-%s",
		tenantID, applicationID, fileUUID, filename)

	// Стримово считаем sha256 одновременно с заливкой в storage:
	// io.MultiWriter копирует то, что читает Storage.Put через TeeReader.
	hasher := sha256.New()
	counter := &countingWriter{}
	tee := io.TeeReader(io.LimitReader(file, MaxFileSize+1), io.MultiWriter(hasher, counter))

	storagePath, err := h.storage.Put(r.Context(), storageKey, tee, mimeType)
	if err != nil {
		h.log.Error("storage put", "tenant", tenantID, "key", storageKey, "err", err)
		writeError(w, http.StatusInternalServerError, "storage_error", "failed to store file")
		return
	}
	if counter.n > MaxFileSize {
		// Заливка прошла, но реальный размер превысил потолок (Content-Length мог врать).
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
			fmt.Sprintf("file exceeds %d bytes", MaxFileSize))
		return
	}

	doc := &domain.Document{
		ID:            docID,
		TenantID:      tenantID,
		ApplicationID: applicationID,
		Type:          docType,
		State:         domain.DocumentStateUploaded,
		FileID:        "file_" + fileUUID,
		Filename:      filename,
		MimeType:      mimeType,
		SizeBytes:     counter.n,
		SHA256:        hex.EncodeToString(hasher.Sum(nil)),
		StoragePath:   storagePath,
		SourceType:    domain.SourceClientUpload,
	}
	if err := h.repo.Create(r.Context(), doc); err != nil {
		if errors.Is(err, repository.ErrInvalidTenant) {
			writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
			return
		}
		h.log.Error("repo create", "tenant", tenantID, "id", doc.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to persist document")
		return
	}

	// Audit-emit: tenant_id из формы (X-Tenant-ID может быть пуст для
	// public-API), entity_id = id документа.
	auditsdk.SetTenantID(r.Context(), doc.TenantID)
	auditPayload, _ := json.Marshal(map[string]any{
		"document_id":    doc.ID,
		"document_type":  doc.Type,
		"application_id": doc.ApplicationID,
		"size_bytes":     doc.SizeBytes,
		"sha256":         doc.SHA256,
	})
	auditsdk.SetEntity(r.Context(), "document", doc.ID, auditPayload)

	writeJSON(w, http.StatusCreated, doc)
}

// updateStateRequest — тело PATCH /v1/documents/{id}/state.
//
// Поддерживаются переходы:
//   - state="parsed"   — handler вызывает repo.MarkParsed(parsed_data из поля reason
//     не используется; парс-результат должен идти отдельным каналом, см. TODO).
//   - state="rejected" — handler вызывает repo.MarkRejected(reason).
type updateStateRequest struct {
	State  domain.DocumentState `json:"state"`
	Reason string               `json:"reason"`
}

// UpdateState: PATCH /v1/documents/{id}/state?tenant_id=X.
//
// Меняет state документа в рамках конечного автомата (uploaded → parsed |
// rejected). По ADR-0010 каждое изменение состояния — это аудит-событие
// `document.state_changed`. EmitOnSuccess отправляет audit при 2xx; в
// success-path handler заполняет SetTenantID/SetEntity для точного payload.
func (h *DocumentHandler) UpdateState(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "id is required")
		return
	}

	var req updateStateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}

	switch req.State {
	case domain.DocumentStateParsed:
		// Полный parsed_data приходит отдельным каналом (document-intake-agent
		// пишет напрямую в repo.MarkParsed). PATCH-смена состояния без данных
		// записывается с пустым parsed payload — фиксируем сам факт перехода.
		if err := h.repo.MarkParsed(r.Context(), tenantID, id, json.RawMessage(`{}`)); err != nil {
			h.handleRepoErr(w, "mark parsed", tenantID, id, err)
			return
		}
	case domain.DocumentStateRejected:
		if req.Reason == "" {
			writeError(w, http.StatusBadRequest, "validation_failed", "reason is required for rejected")
			return
		}
		if err := h.repo.MarkRejected(r.Context(), tenantID, id, req.Reason); err != nil {
			h.handleRepoErr(w, "mark rejected", tenantID, id, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "validation_failed",
			"state must be parsed|rejected")
		return
	}

	// Audit-emit: фиксируем тенант (X-Tenant-ID может быть пуст) и
	// содержательный payload перехода.
	auditsdk.SetTenantID(r.Context(), tenantID)
	auditPayload, _ := json.Marshal(map[string]any{
		"document_id": id,
		"new_state":   req.State,
		"reason":      req.Reason,
	})
	auditsdk.SetEntity(r.Context(), "document", id, auditPayload)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":    id,
		"state": req.State,
	})
}

// handleRepoErr сводит обработку repo-ошибок (NotFound/InvalidTenant/прочее)
// в одно место — общая логика для PATCH-эндпоинтов смены состояния.
func (h *DocumentHandler) handleRepoErr(w http.ResponseWriter, op, tenantID, id string, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if errors.Is(err, repository.ErrInvalidTenant) {
		writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
		return
	}
	h.log.Error(op, "tenant", tenantID, "id", id, "err", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
}

// GetDocument отдаёт метаданные документа.
func (h *DocumentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.repo.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if errors.Is(err, repository.ErrInvalidTenant) {
		writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
		return
	}
	if err != nil {
		h.log.Error("get document", "tenant", tenantID, "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// GetContent стримит содержимое файла из объектного хранилища.
func (h *DocumentHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.repo.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "document not found")
		return
	}
	if err != nil {
		h.log.Error("get document for content", "tenant", tenantID, "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	key := storageKeyFromPath(doc.StoragePath)
	body, err := h.storage.Get(r.Context(), key)
	if errors.Is(err, storage.ErrObjectNotFound) {
		writeError(w, http.StatusNotFound, "content_missing", "document content not found in storage")
		return
	}
	if err != nil {
		h.log.Error("storage get", "tenant", tenantID, "key", key, "err", err)
		writeError(w, http.StatusInternalServerError, "storage_error", "failed to fetch content")
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", doc.MimeType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename=%q`, doc.Filename))
	if _, err := io.Copy(w, body); err != nil {
		h.log.Warn("stream content", "tenant", tenantID, "id", id, "err", err)
	}
}

// ListDocuments возвращает документы заявки.
func (h *DocumentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	applicationID := strings.TrimSpace(r.URL.Query().Get("application_id"))
	if applicationID == "" {
		writeError(w, http.StatusBadRequest, "missing_application", "application_id is required")
		return
	}
	docs, err := h.repo.ListByApplication(r.Context(), tenantID, applicationID)
	if errors.Is(err, repository.ErrInvalidTenant) {
		writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
		return
	}
	if err != nil {
		h.log.Error("list documents", "tenant", tenantID, "app", applicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": docs, "count": len(docs)})
}

// ChecklistItem — позиция чеклиста: что требуется и что уже загружено.
type checklistItem struct {
	Type       string `json:"type"`
	Status     string `json:"status"` // "loaded" | "missing"
	DocumentID string `json:"document_id,omitempty"`
}

// Checklist — GET /v1/documents/checklist?tenant_id=...&application_id=...&legal_entity_form=LLC
//
// Возвращает чеклист обязательных документов в зависимости от юр.формы
// заявителя (этап 6 формы онбординга, см. docs/onboarding-form-spec.md §6).
// Для каждой позиции возвращает status (loaded/missing) и document_id если
// уже загружено. Опциональные документы возвращаются отдельным массивом.
func (h *DocumentHandler) Checklist(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	applicationID := strings.TrimSpace(r.URL.Query().Get("application_id"))
	if applicationID == "" {
		writeError(w, http.StatusBadRequest, "missing_application", "application_id is required")
		return
	}
	form := domain.LegalEntityForm(strings.TrimSpace(r.URL.Query().Get("legal_entity_form")))
	if !domain.IsValidLegalEntityForm(form) {
		writeError(w, http.StatusBadRequest, "invalid_legal_entity_form",
			"legal_entity_form must be IP|LLC|JSC|NPF")
		return
	}

	required := domain.RequiredDocumentTypes(form)
	docs, err := h.repo.ListByApplication(r.Context(), tenantID, applicationID)
	if err != nil {
		h.log.Error("checklist: list documents", "tenant", tenantID, "app", applicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	uploaded := make(map[domain.DocumentType]string, len(docs))
	for _, d := range docs {
		// Берём первый загруженный документ каждого типа. Если нужны дубли —
		// расширим API до []DocumentID per type.
		if _, exists := uploaded[d.Type]; !exists {
			uploaded[d.Type] = d.ID
		}
	}

	items := make([]checklistItem, 0, len(required))
	missing := 0
	for _, t := range required {
		item := checklistItem{Type: string(t), Status: "missing"}
		if id, ok := uploaded[t]; ok {
			item.Status = "loaded"
			item.DocumentID = id
		} else {
			missing++
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"legal_entity_form": string(form),
		"required":          items,
		"missing_count":     missing,
		"complete":          missing == 0,
	})
}

// requireTenant извлекает и валидирует tenant_id из query string.
func requireTenant(w http.ResponseWriter, r *http.Request) (string, bool) {
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "missing_tenant", "tenant_id is required")
		return "", false
	}
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "invalid_tenant", "tenant_id has invalid format")
		return "", false
	}
	return tenantID, true
}

// storageKeyFromPath обрезает префикс scheme://bucket/ из storage_path.
// Поддерживает форматы: memory://documents/<key>, s3://<bucket>/<key>.
func storageKeyFromPath(p string) string {
	if idx := strings.Index(p, "://"); idx >= 0 {
		rest := p[idx+3:]
		// первый сегмент — bucket/префикс, дальше — ключ
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash+1:]
		}
		return rest
	}
	return p
}

// countingWriter — io.Writer, считающий записанные байты.
type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": msg},
	})
}

// Compile-time guard, что context.Context используется.
var _ = context.Background
