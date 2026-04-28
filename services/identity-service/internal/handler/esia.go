package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"aibank/identity-service/internal/auth"
	"aibank/identity-service/internal/domain"
	"aibank/identity-service/internal/esia"
	"aibank/identity-service/internal/repository"
)

// stateTTL — окно жизни связки (state → tenant_id), хранящейся между
// authorize и callback. Реальный пользовательский редирект через ЕСИА
// обычно укладывается в 1-2 минуты; 5 минут даёт запас на ввод 2FA.
const stateTTL = 5 * time.Minute

// stateRecord — содержимое мутабельного in-memory cache для OIDC-state.
//
// Production-deployment должен заменить sync.Map на Redis (state живёт
// между двумя HTTP-запросами разных подов), но для MVP/dev этого хватает.
// TODO(prod): заменить на Redis с per-key TTL.
type stateRecord struct {
	tenantID    string
	redirectURI string
	expiresAt   time.Time
}

// stateStore — потокобезопасный кэш связок (state → запись) с лениво-
// очищаемым TTL. Чистка ленивая, потому что записей мало (один на login)
// и удержание истекших не критично — Get их фильтрует.
type stateStore struct {
	mu      sync.Mutex
	records map[string]stateRecord
}

func newStateStore() *stateStore {
	return &stateStore{records: make(map[string]stateRecord)}
}

// Save кладёт запись и атомарно убирает истекшие.
func (s *stateStore) Save(state string, rec stateRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.records {
		if v.expiresAt.Before(now) {
			delete(s.records, k)
		}
	}
	s.records[state] = rec
}

// Pop возвращает запись и удаляет её (single-use). false если state не
// существует или просрочен.
func (s *stateStore) Pop(state string) (stateRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[state]
	if !ok {
		return stateRecord{}, false
	}
	delete(s.records, state)
	if rec.expiresAt.Before(time.Now()) {
		return stateRecord{}, false
	}
	return rec, true
}

// ESIAHandler обслуживает /v1/auth/esia/authorize и /callback.
//
// Flow:
//  1. Клиент дёргает GET /authorize?tenant_id=X → handler генерирует
//     state+nonce, кладёт state→tenant_id в кэш, отдаёт 302 redirect на
//     provider.BuildAuthURL.
//  2. Пользователь логинится в ЕСИА и возвращается на /callback?code=…&state=…
//  3. Handler валидирует state, тащит UserInfo через provider.Exchange,
//     находит/создаёт applicant'а с ESIAVerified=true, ESIASubject=info.Subject,
//     выпускает JWT-пару и эмитит audit-событие "identity.esia_login".
type ESIAHandler struct {
	provider    esia.Provider
	applicants  domain.ApplicantRepository
	issuer      *auth.Issuer
	auditClient *auditsdk.Client
	redirectURI string
	store       *stateStore
	log         *slog.Logger
}

// NewESIAHandler — конструктор. redirectURI должен совпадать с тем, что
// зарегистрирован у ЕСИА (или — в dev — с реальным URL callback'а).
// auditClient может быть nil (best-effort emit).
func NewESIAHandler(
	provider esia.Provider,
	applicants domain.ApplicantRepository,
	issuer *auth.Issuer,
	auditClient *auditsdk.Client,
	redirectURI string,
	log *slog.Logger,
) *ESIAHandler {
	return &ESIAHandler{
		provider:    provider,
		applicants:  applicants,
		issuer:      issuer,
		auditClient: auditClient,
		redirectURI: redirectURI,
		store:       newStateStore(),
		log:         log,
	}
}

// Routes — chi-роутер для /v1/auth/esia/*.
//
// callback обёрнут audit-emit'ом ("identity.esia_login"). authorize — нет:
// до получения user-info мы не знаем, какой applicant залогинился.
func (h *ESIAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withAuditHolder)
	r.Get("/authorize", h.Authorize)
	if h.auditClient != nil {
		r.With(emitOnSuccessFromHolder(h.auditClient, "identity.esia_login",
			h.resolveESIALogin, h.log)).Get("/callback", h.Callback)
	} else {
		r.Get("/callback", h.Callback)
	}
	return r
}

// resolveESIALogin — EntityResolver для /callback. На post-этапе applicant
// уже создан/найден handler'ом и положен в audit-holder через rememberActor.
func (h *ESIAHandler) resolveESIALogin(r *http.Request) (string, string, json.RawMessage, bool) {
	holder, _ := r.Context().Value(auditCtxKey{}).(*auditCtx)
	if holder == nil || holder.actorID == "" {
		return "", "", nil, false
	}
	payload, _ := json.Marshal(map[string]any{
		"applicant_id": holder.actorID,
		"flow":         "esia",
	})
	return "applicant", holder.actorID, payload, true
}

// Authorize: GET /v1/auth/esia/authorize?tenant_id=X
//
// Генерирует криптостойкие state и nonce, сохраняет state→tenant_id,
// возвращает 302 на provider.BuildAuthURL. Запрос не требует JWT.
func (h *ESIAHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"tenant_id is required and must match ^[a-z][a-z0-9_]{1,31}$")
		return
	}
	state, err := randomToken(32)
	if err != nil {
		h.log.Error("esia: random state", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate state")
		return
	}
	nonce, err := randomToken(32)
	if err != nil {
		h.log.Error("esia: random nonce", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate nonce")
		return
	}
	h.store.Save(state, stateRecord{
		tenantID:    tenantID,
		redirectURI: h.redirectURI,
		expiresAt:   time.Now().Add(stateTTL),
	})
	authURL := h.provider.BuildAuthURL(state, nonce, h.redirectURI)
	w.Header().Set("Location", authURL)
	w.WriteHeader(http.StatusFound)
}

type esiaCallbackResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ApplicantID  string `json:"applicant_id"`
	ESIASubject  string `json:"esia_subject"`
}

// Callback: GET /v1/auth/esia/callback?code=…&state=…
//
// Проверяет state, обменивает code на UserInfo, находит applicant'а
// по ESIA Subject (или создаёт нового), выпускает JWT-пару.
func (h *ESIAHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "validation_failed",
			"code and state are required")
		return
	}
	rec, ok := h.store.Pop(state)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_state",
			"state not found or expired")
		return
	}

	info, err := h.provider.Exchange(r.Context(), code, rec.redirectURI)
	if err != nil {
		if errors.Is(err, esia.ErrNotImplemented) {
			h.log.Warn("esia: live provider not implemented")
			writeError(w, http.StatusServiceUnavailable, "esia_not_wired",
				"ESIA live integration is not yet wired; set ESIA_LIVE=false for dev")
			return
		}
		h.log.Error("esia: exchange", "err", err)
		writeError(w, http.StatusBadGateway, "esia_exchange_failed", err.Error())
		return
	}

	// Сначала ищем applicant'а по INN (предположим, что INN — стабильный
	// бизнес-идентификатор). Если не нашли — создаём нового с ESIA-маркерами.
	// В future-iteration: ввести индекс по ESIASubject и искать по нему.
	a, err := h.applicants.GetByINN(r.Context(), rec.tenantID, info.INN)
	if errors.Is(err, repository.ErrNotFound) {
		a = &domain.Applicant{
			ID:           "appl_" + uuid.NewString(),
			TenantID:     rec.tenantID,
			INN:          info.INN,
			Phone:        info.Phone,
			FullName:     info.FullName,
			ESIAVerified: true,
			ESIASubject:  info.Subject,
		}
		// Минимальное согласие — applicant сам нажал «войти через ЕСИА»,
		// что эквивалентно явному согласию (152-ФЗ ст. 9).
		consents := []domain.Consent{{
			ConsentType: domain.ConsentDataProcessing,
			Granted:     true,
			Version:     "esia-implicit-v1",
			RecordedAt:  time.Now().UTC(),
			IPAddress:   clientIP(r),
			UserAgent:   r.UserAgent(),
		}}
		if err := h.applicants.CreateWithConsents(r.Context(), a, consents); err != nil {
			h.log.Error("esia: create applicant", "tenant", rec.tenantID, "err", err)
			writeError(w, http.StatusInternalServerError, "create_failed",
				"failed to create applicant")
			return
		}
	} else if err != nil {
		h.log.Error("esia: lookup applicant", "tenant", rec.tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	// Выписываем JWT-пару. Subject = applicant.ID; role условно "client.applicant".
	pair, err := h.issuer.Issue(auth.IssueParams{
		Subject:  a.ID,
		TenantID: a.TenantID,
		Role:     "client.applicant",
	})
	if err != nil {
		h.log.Error("esia: issue tokens", "applicant_id", a.ID, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error",
			"failed to issue tokens")
		return
	}

	rememberActor(r, a.TenantID, a.ID, auditsdk.ActorTypeUser)

	writeJSON(w, http.StatusOK, esiaCallbackResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		ApplicantID:  a.ID,
		ESIASubject:  info.Subject,
	})
}

// randomToken возвращает URL-safe base64 строку из n случайных байт.
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
