package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/identity-service/internal/auth"
	"aibank/identity-service/internal/domain"
	"aibank/identity-service/internal/repository"
)

// validTenantID — тот же whitelist, что и в repository (см. ADR-0002).
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// validINN — 12 цифр для физлица.
var validINN = regexp.MustCompile(`^[0-9]{12}$`)

// validEmail — упрощённая RFC 5322 lite-проверка (server-side; основная
// валидация — на фронте). Достаточная для отсечения опечаток.
var validEmail = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)

const maxBodyBytes = 64 * 1024

// AuthHandler обслуживает /v1/auth/*, /v1/applicants[/...] и /v1/me.
type AuthHandler struct {
	users       domain.UserRepository
	applicants  domain.ApplicantRepository
	consents    domain.ConsentRepository
	issuer      *auth.Issuer
	verifier    *auth.Verifier
	auditClient *auditsdk.Client
	log         *slog.Logger
}

func NewAuthHandler(
	users domain.UserRepository,
	applicants domain.ApplicantRepository,
	consents domain.ConsentRepository,
	issuer *auth.Issuer,
	verifier *auth.Verifier,
	auditClient *auditsdk.Client,
	log *slog.Logger,
) *AuthHandler {
	return &AuthHandler{
		users:       users,
		applicants:  applicants,
		consents:    consents,
		issuer:      issuer,
		verifier:    verifier,
		auditClient: auditClient,
		log:         log,
	}
}

// Routes монтирует маршруты под /v1.  Mutating-эндпоинты обёрнуты
// audit-middleware: emit вызывается ТОЛЬКО на 2xx и не блокирует ответ
// клиенту.
func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withAuditHolder)

	if h.auditClient != nil {
		r.With(emitOnSuccessFromHolder(h.auditClient, "identity.login",
			h.resolveLogin, h.log)).Post("/auth/login", h.Login)
		r.With(emitOnSuccessFromHolder(h.auditClient, "identity.refresh",
			h.resolveRefresh, h.log)).Post("/auth/refresh", h.Refresh)
		r.With(emitOnSuccessFromHolder(h.auditClient, "applicant.registered",
			h.resolveApplicantCreated, h.log)).Post("/applicants", h.CreateApplicant)
		r.With(emitOnSuccessFromHolder(h.auditClient, "consent.revoked",
			h.resolveConsentRevoked, h.log)).Post("/applicants/{id}/consents/revoke", h.RevokeConsent)
		r.With(emitOnSuccessFromHolder(h.auditClient, "applicant.forgotten",
			h.resolveApplicantForgotten, h.log)).Patch("/applicants/{id}/forget", h.ForgetApplicant)
	} else {
		r.Post("/auth/login", h.Login)
		r.Post("/auth/refresh", h.Refresh)
		r.Post("/applicants", h.CreateApplicant)
		r.Post("/applicants/{id}/consents/revoke", h.RevokeConsent)
		r.Patch("/applicants/{id}/forget", h.ForgetApplicant)
	}

	r.Get("/applicants/{id}/consents", h.ListConsents)
	r.Get("/me", h.Me)
	return r
}

// resolveLogin — EntityResolver POST /v1/auth/login.  Тело уже прочитано
// handler'ом, поэтому опираемся на holder.
func (h *AuthHandler) resolveLogin(r *http.Request) (string, string, json.RawMessage, bool) {
	holder, _ := r.Context().Value(auditCtxKey{}).(*auditCtx)
	if holder == nil || holder.actorID == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"action": "login"})
	return "user", holder.actorID, payload, true
}

func (h *AuthHandler) resolveRefresh(r *http.Request) (string, string, json.RawMessage, bool) {
	holder, _ := r.Context().Value(auditCtxKey{}).(*auditCtx)
	if holder == nil || holder.actorID == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"action": "refresh"})
	return "user", holder.actorID, payload, true
}

func (h *AuthHandler) resolveApplicantCreated(r *http.Request) (string, string, json.RawMessage, bool) {
	holder, _ := r.Context().Value(auditCtxKey{}).(*auditCtx)
	if holder == nil || holder.actorID == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"action": "applicant_registered"})
	return "applicant", holder.actorID, payload, true
}

func (h *AuthHandler) resolveConsentRevoked(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{"applicant_id": id, "action": "consent_revoked"})
	return "applicant", id, payload, true
}

func (h *AuthHandler) resolveApplicantForgotten(r *http.Request) (string, string, json.RawMessage, bool) {
	id := chi.URLParam(r, "id")
	if id == "" {
		return "", "", nil, false
	}
	// Payload без PII — только id и action.
	payload, _ := json.Marshal(map[string]any{"applicant_id": id, "action": "right_to_be_forgotten"})
	return "applicant", id, payload, true
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TenantID string `json:"tenant_id"`
}

// Login — POST /v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "email and password are required")
		return
	}
	// tenant_id допустим пустой только для платформенных ролей; иначе валидируем формат.
	if req.TenantID != "" && !validTenantID.MatchString(req.TenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid tenant_id format")
		return
	}

	u, err := h.users.GetByEmail(r.Context(), req.Email)
	if errors.Is(err, repository.ErrNotFound) {
		// Не раскрываем, какой именно фактор не сошёлся.
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if err != nil {
		h.log.Error("get user by email", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if !u.IsActive {
		writeError(w, http.StatusForbidden, "user_inactive", "user is not active")
		return
	}

	// Платформенный админ: tenant_id в payload игнорируется (но если передан, должен совпасть с пустым).
	// Банковский пользователь: tenant_id обязателен и должен совпадать.
	if u.Role.IsPlatform() {
		if req.TenantID != "" {
			writeError(w, http.StatusBadRequest, "validation_failed",
				"platform users must not pass tenant_id")
			return
		}
	} else {
		if u.TenantID == nil || *u.TenantID != req.TenantID {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
			return
		}
	}

	if err := auth.Verify(u.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}

	pair, err := h.issuer.Issue(auth.IssueParams{
		Subject:  u.ID,
		TenantID: tenantIDOf(u),
		Role:     string(u.Role),
	})
	if err != nil {
		h.log.Error("issue tokens", "user_id", u.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to issue tokens")
		return
	}

	// Audit-emit: успешный login.
	rememberActor(r, tenantIDOf(u), u.ID, auditsdk.ActorTypeUser)
	loginPayload, _ := json.Marshal(map[string]any{
		"user_id": u.ID,
		"role":    u.Role,
	})
	auditsdk.SetEntity(r.Context(), "user", u.ID, loginPayload)

	writeJSON(w, http.StatusOK, pair)
}

func tenantIDOf(u *domain.User) string {
	if u.TenantID == nil {
		return ""
	}
	return *u.TenantID
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Refresh — POST /v1/auth/refresh: проверяет refresh-токен и выдаёт новый access.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "refresh_token is required")
		return
	}

	claims, err := h.verifier.Parse(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "refresh token is invalid")
		return
	}
	if claims.Type != domain.TokenRefresh {
		writeError(w, http.StatusUnauthorized, "invalid_token", "expected a refresh token")
		return
	}

	pair, err := h.issuer.Issue(auth.IssueParams{
		Subject:  claims.Subject,
		TenantID: claims.TenantID,
		Role:     claims.Role,
	})
	if err != nil {
		h.log.Error("issue access on refresh", "subject", claims.Subject, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to refresh")
		return
	}
	writeJSON(w, http.StatusOK, refreshResponse{
		AccessToken: pair.AccessToken,
		ExpiresIn:   pair.ExpiresIn,
	})
}

// Me — GET /v1/me: возвращает данные текущего пользователя.
//
// Для applicant-токенов отдаём 404: applicant'ы не имеют записи в platform.users.
// Их собственный профиль — отдельная задача onboarding-сервиса.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	tokenStr, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_token", "Authorization: Bearer <token> required")
		return
	}
	claims, err := h.verifier.Parse(tokenStr)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "token is invalid or expired")
		return
	}
	if claims.Type != domain.TokenAccess {
		writeError(w, http.StatusUnauthorized, "invalid_token", "expected an access token")
		return
	}

	u, err := h.users.GetByID(r.Context(), claims.Subject)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	if err != nil {
		h.log.Error("get user by id", "id", claims.Subject, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	t := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if t == "" {
		return "", false
	}
	return t, true
}

type consentInput struct {
	Type      string `json:"type"`
	Granted   bool   `json:"granted"`
	Version   string `json:"version"`
	Signature string `json:"signature,omitempty"`
}

type createApplicantRequest struct {
	TenantID string         `json:"tenant_id"`
	INN      string         `json:"inn"`
	Phone    string         `json:"phone"`
	FullName string         `json:"full_name"`
	Email    string         `json:"email,omitempty"`
	Password string         `json:"password,omitempty"`
	Consents []consentInput `json:"consents"`
}

func (req createApplicantRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("invalid tenant_id format")
	}
	if !validINN.MatchString(req.INN) {
		return errors.New("inn must be 12 digits")
	}
	if strings.TrimSpace(req.Phone) == "" {
		return errors.New("phone is required")
	}
	if strings.TrimSpace(req.FullName) == "" {
		return errors.New("full_name is required")
	}
	// Email + password — необязательны (для обратной совместимости с
	// SMS/OTP flow), но если переданы — должны быть валидны и идти парой.
	if req.Email != "" || req.Password != "" {
		if !validEmail.MatchString(req.Email) {
			return errors.New("email must be a valid address when provided")
		}
		if len(req.Password) < 8 {
			return errors.New("password must be at least 8 chars when provided")
		}
	}
	for i, c := range req.Consents {
		if !domain.ConsentType(c.Type).IsValid() {
			return fmt.Errorf("consents[%d]: invalid type %q", i, c.Type)
		}
		if strings.TrimSpace(c.Version) == "" {
			return fmt.Errorf("consents[%d]: version is required", i)
		}
	}
	return nil
}

// CreateApplicant — POST /v1/applicants. Публичный эндпоинт без аутентификации
// (старт онбординга): клиент банка регистрируется до прохождения ЕСИА.
//
// 152-ФЗ ст. 9: тело запроса ОБЯЗАНО содержать массив consents хотя бы с
// одним элементом {type: "data_processing", granted: true, version: "..."}.
// Без явного согласия на обработку ПДн регистрация запрещается (HTTP 400).
// Опциональные согласия (marketing, biometrics) принимаются в любом виде.
//
// Запись applicant'а и его согласий выполняется атомарно (одна транзакция в
// схеме тенанта); IP-адрес и User-Agent захватываются из заголовков для
// подтверждения волеизъявления субъекта ПДн.
func (h *AuthHandler) CreateApplicant(w http.ResponseWriter, r *http.Request) {
	var req createApplicantRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	// Преобразуем входные согласия в доменную форму и валидируем
	// бизнес-правило 152-ФЗ (ОБЯЗАТЕЛЬНОЕ data_processing).
	domainConsents := make([]domain.Consent, 0, len(req.Consents))
	for _, ci := range req.Consents {
		domainConsents = append(domainConsents, domain.Consent{
			ConsentType: domain.ConsentType(ci.Type),
			Granted:     ci.Granted,
			Version:     strings.TrimSpace(ci.Version),
			Signature:   strings.TrimSpace(ci.Signature),
		})
	}
	if err := domain.ValidateConsentsForRegistration(domainConsents); err != nil {
		writeError(w, http.StatusBadRequest, "consent_required", err.Error())
		return
	}

	// Идемпотентность по (tenant_id, inn): если уже есть — возвращаем существующего.
	// Согласия повторно не записываем, чтобы не нарушить уникальность активной строки;
	// для повторной выдачи согласия используется отдельный flow (TODO).
	existing, err := h.applicants.GetByINN(r.Context(), req.TenantID, req.INN)
	if err == nil && existing != nil {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		h.log.Error("lookup applicant", "tenant_id", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	a := &domain.Applicant{
		ID:           uuid.New().String(),
		TenantID:     req.TenantID,
		INN:          req.INN,
		Phone:        strings.TrimSpace(req.Phone),
		FullName:     strings.TrimSpace(req.FullName),
		ESIAVerified: false,
	}

	// Достраиваем согласия контекстными полями (id, ip, ua) — на этом этапе
	// applicant_id ещё неизвестен; репозиторий проставит его в транзакции.
	ip := clientIP(r)
	ua := r.Header.Get("User-Agent")
	for i := range domainConsents {
		domainConsents[i].ID = "cns_" + uuid.NewString()
		domainConsents[i].IPAddress = ip
		domainConsents[i].UserAgent = ua
	}

	if err := h.applicants.CreateWithConsents(r.Context(), a, domainConsents); err != nil {
		h.log.Error("create applicant with consents", "tenant_id", a.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "create_failed", "failed to create applicant")
		return
	}

	// Если applicant передал email + password — создаём ему запись в
	// platform.users с role=applicant, чтобы тот же /v1/auth/login flow
	// мог вернуть JWT. Идемпотентность: GetByEmail → если уже есть, не
	// перезаписываем (повторная регистрация одного и того же applicant —
	// нормальный сценарий, см. logic выше).
	if req.Email != "" && req.Password != "" {
		emailNorm := strings.ToLower(strings.TrimSpace(req.Email))
		if existingUser, err := h.users.GetByEmail(r.Context(), emailNorm); err == nil && existingUser != nil {
			// Уже зарегистрирован — пропускаем, login сработает с прежним паролем.
			h.log.Info("applicant user already exists, skipping users insert", "email", emailNorm)
		} else {
			hash, hashErr := auth.Hash(req.Password)
			if hashErr != nil {
				h.log.Error("hash password", "err", hashErr)
				writeError(w, http.StatusBadRequest, "validation_failed", hashErr.Error())
				return
			}
			tenantID := a.TenantID
			user := &domain.User{
				ID:           "usr_" + uuid.NewString(),
				TenantID:     &tenantID,
				Email:        emailNorm,
				PasswordHash: hash,
				Role:         domain.RoleApplicant,
				IsActive:     true,
			}
			if err := h.users.Create(r.Context(), user); err != nil {
				h.log.Error("create applicant user", "email", emailNorm, "err", err)
				// Не отдаём 500 — applicant уже создан в tnt_<>.applicants;
				// users-запись повторно создастся при следующем register с
				// тем же email. Лог достаточен для диагностики.
			}
		}
	}

	// Audit-emit: новый applicant зарегистрирован.
	rememberActor(r, a.TenantID, a.ID, auditsdk.ActorTypeUser)
	applicantPayload, _ := json.Marshal(map[string]any{
		"applicant_id": a.ID,
		"inn":          a.INN,
		// В payload пишем только типы согласий и версии — БЕЗ ПДн (152-ФЗ §
		// 8.3 security-architecture запрещает ПДн в логах/payload).
		"consents": consentSummary(domainConsents),
	})
	auditsdk.SetEntity(r.Context(), "applicant", a.ID, applicantPayload)

	writeJSON(w, http.StatusCreated, a)
}

// ListConsents — GET /v1/applicants/{id}/consents?tenant_id=X.
//
// Возвращает АКТИВНЫЕ (revoked_at IS NULL) согласия применителя. Отозванные
// записи скрыты — для аудиторского обзора всех версий используется отдельный
// audit-инструмент (TODO: GET /consents/history).
func (h *AuthHandler) ListConsents(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "applicant id is required")
		return
	}
	if h.consents == nil {
		writeError(w, http.StatusServiceUnavailable, "consents_unavailable",
			"consent repository is not configured")
		return
	}
	items, err := h.consents.ListByApplicant(r.Context(), tenantID, id)
	if err != nil {
		h.log.Error("list consents", "tenant_id", tenantID, "applicant_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

type revokeConsentRequest struct {
	ConsentType string `json:"consent_type"`
}

// RevokeConsent — POST /v1/applicants/{id}/consents/revoke?tenant_id=X.
//
// Отзывает активное согласие указанного типа. Реализован как UPDATE
// revoked_at = NOW() (триггер БД допускает только этот переход для consents).
// Отзыв data_processing после регистрации технически возможен, но семантически
// должен сопровождаться правом-на-забвение (PATCH /v1/clients/{id}/forget в
// client-service); связь между этими двумя действиями — на уровне orchestrator.
func (h *AuthHandler) RevokeConsent(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is required")
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "applicant id is required")
		return
	}
	if h.consents == nil {
		writeError(w, http.StatusServiceUnavailable, "consents_unavailable",
			"consent repository is not configured")
		return
	}
	var req revokeConsentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	ct := domain.ConsentType(strings.TrimSpace(req.ConsentType))
	if !ct.IsValid() {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"consent_type must be data_processing|marketing|biometrics")
		return
	}
	if err := h.consents.Revoke(r.Context(), tenantID, id, ct); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "active consent not found")
			return
		}
		h.log.Error("revoke consent", "tenant_id", tenantID, "applicant_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	rememberActor(r, tenantID, id, auditsdk.ActorTypeUser)
	revokePayload, _ := json.Marshal(map[string]any{
		"applicant_id": id,
		"consent_type": string(ct),
	})
	auditsdk.SetEntity(r.Context(), "applicant", id, revokePayload)
	writeJSON(w, http.StatusOK, map[string]any{
		"applicant_id": id,
		"consent_type": string(ct),
		"revoked":      true,
	})
}

// consentSummary — для audit payload: список {type, granted, version} БЕЗ
// ПДн и БЕЗ IP/UA (для compliance достаточно факта).
func consentSummary(cs []domain.Consent) []map[string]any {
	out := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		out = append(out, map[string]any{
			"type":    string(c.ConsentType),
			"granted": c.Granted,
			"version": c.Version,
		})
	}
	return out
}

// clientIP возвращает IP-адрес клиента: предпочитает X-Forwarded-For (первый
// hop), затем X-Real-IP, затем r.RemoteAddr (без порта). Подходит для записи
// «волеизъявления субъекта ПДн» при выдаче согласия 152-ФЗ.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Берём первый, незаведомо-доверенный hop.
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

type forgetApplicantRequest struct {
	TenantID  string `json:"tenant_id"`
	Requester string `json:"requester"`
}

// ForgetApplicant — PATCH /v1/applicants/{id}/forget.
// Реализация 152-ФЗ ст. 14: PII-поля applicant'а сбрасываются в пустые
// значения, ставится forgotten_at/forgotten_by. Запись сохраняется
// для FK-целостности audit-логов (5 лет по 115-ФЗ — это другое требование).
//
// Идемпотентно: повторный вызов на already-forgotten applicant возвращает 200.
func (h *AuthHandler) ForgetApplicant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "id is required")
		return
	}
	var req forgetApplicantRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if !validTenantID.MatchString(req.TenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "invalid tenant_id format")
		return
	}
	if req.Requester == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "requester is required")
		return
	}

	if err := h.applicants.Forget(r.Context(), req.TenantID, id, req.Requester); err != nil {
		h.log.Error("forget applicant", "id", id, "tenant", req.TenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to forget applicant")
		return
	}

	rememberActor(r, req.TenantID, req.Requester, auditsdk.ActorTypeUser)

	writeJSON(w, http.StatusOK, map[string]any{
		"applicant_id": id,
		"forgotten":    true,
	})
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
