package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"github.com/aibank/platform/services/tenant-service/internal/domain"
	"github.com/aibank/platform/services/tenant-service/internal/repository"
)

type TenantHandler struct {
	repo        domain.TenantRepository
	configRepo  domain.TenantConfigRepository
	provisioner domain.SchemaProvisioner
	auditClient *auditsdk.Client
	log         *slog.Logger
}

func NewTenantHandler(
	repo domain.TenantRepository,
	configRepo domain.TenantConfigRepository,
	provisioner domain.SchemaProvisioner,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *TenantHandler {
	return &TenantHandler{
		repo:        repo,
		configRepo:  configRepo,
		provisioner: provisioner,
		auditClient: auditClient,
		log:         log,
	}
}

// auditCtxKey — ключ для мутабельного holder'а tenantID, который handler
// заполняет после успешной обработки тела запроса (тело читается уже
// внутри handler'а, поэтому middleware не видит tenant_id).
type auditCtxKey struct{}

// auditCtx — holder, разделяемый между handler и audit-middleware.
type auditCtx struct {
	tenantID string
	actorID  string
}

// withPlatformAuth прикладывает audit-holder для платформенных операций.
// tenant-service пока не интегрирован с JWT (ADR-0010 follow-up по
// corr-id и actor signing).  Actor идентифицируется по заголовку
// X-Actor-ID; в проде он будет приходить из api-gateway после JWT-парсинга.
//
// Сам AuthInfo НЕ кладём здесь — tenant_id не известен до парсинга тела.
// Вместо этого используем holder, обновляемый handler'ом, и audit-emit
// middleware через rememberAuthInfo вызывает auditsdk.WithAuthInfo
// прямо перед вызовом next в auditEmitMiddleware.
func withPlatformAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID := r.Header.Get("X-Actor-ID")
		if actorID == "" {
			actorID = "system"
		}
		holder := &auditCtx{
			tenantID: r.Header.Get("X-Tenant-ID"),
			actorID:  actorID,
		}
		ctx := context.WithValue(r.Context(), auditCtxKey{}, holder)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// rememberTenantID — handler вызывает после валидации тела, чтобы
// audit-emit middleware увидел корректный tenant_id через
// auditsdk.SetTenantID + AuthInfo из holder.
func rememberTenantID(r *http.Request, tenantID string) *http.Request {
	if h, ok := r.Context().Value(auditCtxKey{}).(*auditCtx); ok {
		h.tenantID = tenantID
	}
	auditsdk.SetTenantID(r.Context(), tenantID)
	return r
}

// emitOnSuccessFromHolder — переходник: до вызова handler'а кладёт в
// AuthInfo базовый actor (tenant_id заполнится handler'ом через
// auditsdk.SetTenantID); затем вызывает auditsdk.EmitOnSuccess.
func emitOnSuccessFromHolder(client *auditsdk.Client, eventType string,
	resolve auditsdk.EntityResolver, log *slog.Logger,
) func(http.Handler) http.Handler {
	emit := auditsdk.EmitOnSuccess(client, eventType, resolve, log)
	return func(next http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if h, ok := r.Context().Value(auditCtxKey{}).(*auditCtx); ok {
					ctx := auditsdk.WithAuthInfo(r.Context(), auditsdk.AuthInfo{
						TenantID:  h.tenantID,
						ActorID:   h.actorID,
						ActorType: auditsdk.ActorTypeSystem,
					})
					r = r.WithContext(ctx)
				}
				next.ServeHTTP(w, r)
			})
		}(emit(next))
	}
}

// tenantIDFromContext читает tenant_id из holder, заполненного handler'ом.
func tenantIDFromContext(ctx context.Context) string {
	if h, ok := ctx.Value(auditCtxKey{}).(*auditCtx); ok {
		return h.tenantID
	}
	return ""
}

func (h *TenantHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Get("/", h.ListTenants)

	// CreateTenant: audit emit после успешного 201.  tenantID/entityID
	// резолвится через chi-route context — handler пишет id в ответ.
	if h.auditClient != nil {
		r.With(emitOnSuccessFromHolder(h.auditClient, "tenant.created",
			h.resolveTenantCreate, h.log)).Post("/", h.CreateTenant)
	} else {
		r.Post("/", h.CreateTenant)
	}

	r.Get("/{id}", h.GetTenant)
	r.Get("/{id}/config", h.GetConfig)

	if h.auditClient != nil {
		r.With(emitOnSuccessFromHolder(h.auditClient, "tenant.config_updated",
			h.resolveConfigUpdate, h.log)).Put("/{id}/config", h.UpdateConfig)
	} else {
		r.Put("/{id}/config", h.UpdateConfig)
	}
	return r
}

// resolveTenantCreate — EntityResolver для POST /v1/tenants.
// Извлекает tenant_id из тела через peek-копию; payload — минимальный
// JSON с публичными полями (без секретов).
func (h *TenantHandler) resolveTenantCreate(r *http.Request) (string, string, json.RawMessage, bool) {
	// На post-этапе тело уже прочитано handler'ом; берём id из path-info,
	// который handler положил в context.  Если context пуст — пропускаем.
	id := tenantIDFromContext(r.Context())
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"tenant_id": id})
	return "tenant", id, payload, true
}

// resolveConfigUpdate — EntityResolver для PUT /v1/tenants/{id}/config.
func (h *TenantHandler) resolveConfigUpdate(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"tenant_id": id, "action": "config_updated"})
	return "tenant_config", id, payload, true
}

type createTenantRequest struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	BIK            string                `json:"bik"`
	INN            string                `json:"inn"`
	DeploymentMode domain.DeploymentMode `json:"deployment_mode"`
}

func (req createTenantRequest) validate() error {
	if req.ID == "" {
		return errors.New("id is required")
	}
	if req.Name == "" {
		return errors.New("name is required")
	}
	if len(req.BIK) != 9 {
		return errors.New("bik must be 9 chars")
	}
	if len(req.INN) != 10 {
		return errors.New("inn must be 10 chars (legal entity)")
	}
	switch req.DeploymentMode {
	case domain.DeploymentModeSaaS, domain.DeploymentModeOnPrem, domain.DeploymentModeHybrid:
	default:
		return errors.New("deployment_mode must be saas, on_prem or hybrid")
	}
	return nil
}

func (h *TenantHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	// Сообщаем audit-middleware tenant_id из тела (резолвер увидит его
	// после успешного 2xx).
	r = rememberTenantID(r, req.ID)

	t := &domain.Tenant{
		ID:             req.ID,
		Name:           req.Name,
		BIK:            req.BIK,
		INN:            req.INN,
		Status:         domain.TenantStatusTrial,
		DeploymentMode: req.DeploymentMode,
	}

	// Шаг 1: метаданные в platform.tenants.
	if err := h.repo.Create(r.Context(), t); err != nil {
		h.log.Error("create tenant", "id", t.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create tenant")
		return
	}

	// Шаг 2: provisioning изолированной схемы (ADR-0002, шаги 1-3).
	// Применение DDL-миграций к схеме — ответственность db-migrator (ADR-0005).
	if err := h.provisioner.Provision(r.Context(), t.ID); err != nil {
		h.log.Error("provision schema", "id", t.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "provision_failed",
			"tenant created but schema provisioning failed; contact ops")
		return
	}

	// Передаём tenant_id audit-holder'у, чтобы emitOnSuccessFromHolder
	// заполнил AuthInfo и middleware EmitOnSuccess отправил событие.
	r = rememberTenantID(r, t.ID)

	writeJSON(w, http.StatusCreated, t)
}

func (h *TenantHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "tenant not found")
		return
	}
	if err != nil {
		h.log.Error("get tenant", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *TenantHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.repo.List(r.Context())
	if err != nil {
		h.log.Error("list tenants", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tenants, "count": len(tenants)})
}

func (h *TenantHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cfg, err := h.configRepo.GetConfig(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "config not found")
		return
	}
	if err != nil {
		h.log.Error("get config", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

type updateConfigRequest struct {
	RawConfig     []byte `json:"raw_config"`
	SchemaVersion string `json:"schema_version"`
}

func (h *TenantHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	r = rememberTenantID(r, id)

	if _, err := h.repo.GetByID(r.Context(), id); errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "tenant not found")
		return
	} else if err != nil {
		h.log.Error("verify tenant", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	var req updateConfigRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if len(req.RawConfig) == 0 {
		writeError(w, http.StatusBadRequest, "validation_failed", "raw_config is required")
		return
	}
	if req.SchemaVersion == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "schema_version is required")
		return
	}

	cfg := &domain.TenantConfig{
		TenantID:      id,
		RawConfig:     req.RawConfig,
		SchemaVersion: req.SchemaVersion,
	}
	if err := h.configRepo.SaveConfig(r.Context(), cfg); err != nil {
		h.log.Error("save config", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "save_failed", "failed to save config")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
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
