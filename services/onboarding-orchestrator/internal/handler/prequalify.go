package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"aibank/onboarding-orchestrator/internal/extclients"
)

// validINN10 — ИНН ЮЛ. validOGRN — ОГРН.
var (
	validINN10 = regexp.MustCompile(`^[0-9]{10}$`)
	validOGRN  = regexp.MustCompile(`^[0-9]{13}$`)
)

// PrequalifyClient — узкий интерфейс для тестов; реализация — *extclients.Clients.
type PrequalifyClient interface {
	EGRULByINN(ctx context.Context, inn string) (*extclients.EGRULData, error)
	RosfinmonScreen(ctx context.Context, inn string) (*extclients.RosfinmonResult, error)
	FSSPByINN(ctx context.Context, inn string) (*extclients.FSSPResult, error)
}

// PrequalifyHandler — этап 1 формы онбординга. Принимает {tenant_id, inn,
// ogrn, short_name}, параллельно ходит в ext-egrul / ext-rosfinmon / ext-fssp,
// агрегирует ответ в PrequalificationCheck-совместимый JSON и возвращает
// финальное решение proceed/manual_review/reject. Запись в БД пока не делается
// — это будет добавлено вместе с persistence-слоем для PrequalificationCheck.
type PrequalifyHandler struct {
	clients PrequalifyClient
	log     *slog.Logger
}

func NewPrequalifyHandler(clients PrequalifyClient, log *slog.Logger) *PrequalifyHandler {
	return &PrequalifyHandler{clients: clients, log: log}
}

type prequalifyRequest struct {
	TenantID  string `json:"tenant_id"`
	INN       string `json:"inn"`
	OGRN      string `json:"ogrn"`
	ShortName string `json:"short_name"`
}

func (req prequalifyRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid (regex ^[a-z][a-z0-9_]{1,31}$)")
	}
	if !validINN10.MatchString(req.INN) {
		return errors.New("inn must be 10 digits (legal entity)")
	}
	if !validOGRN.MatchString(req.OGRN) {
		return errors.New("ogrn must be 13 digits")
	}
	if strings.TrimSpace(req.ShortName) == "" {
		return errors.New("short_name is required")
	}
	return nil
}

// PrequalifyResponse зеркалит подмножество $defs/PrequalificationCheck из
// packages/domain-model/schema.json (минус persistence-поля id/application_id).
type PrequalifyResponse struct {
	INN                   string    `json:"inn"`
	OGRN                  string    `json:"ogrn"`
	ShortNameHint         string    `json:"short_name_hint,omitempty"`
	EGRULStatus           string    `json:"egrul_status,omitempty"`
	EGRULRegistrationDate string    `json:"egrul_registration_date,omitempty"`
	EGRULAddress          string    `json:"egrul_address,omitempty"`
	EGRULCEOName          string    `json:"egrul_ceo_name,omitempty"`
	EGRULFullName         string    `json:"egrul_full_name,omitempty"`
	NameMatchesEGRUL      bool      `json:"name_matches_egrul"`
	RosfinmonPresent      bool      `json:"rosfinmon_present"`
	FSSPProceedingsCount  int       `json:"fssp_proceedings_count"`
	FSSPTotalDebtKopecks  int64     `json:"fssp_total_debt_kopecks"`
	Decision              string    `json:"decision"`
	DecisionReason        string    `json:"decision_reason,omitempty"`
	UnavailableSources    []string  `json:"unavailable_sources,omitempty"`
	CheckedAt             time.Time `json:"checked_at"`
}

// Prequalify — POST /v1/applications/prequalify.
func (h *PrequalifyHandler) Prequalify(w http.ResponseWriter, r *http.Request) {
	var req prequalifyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	type egrulOut struct {
		data *extclients.EGRULData
		err  error
	}
	type rfmOut struct {
		data *extclients.RosfinmonResult
		err  error
	}
	type fsspOut struct {
		data *extclients.FSSPResult
		err  error
	}
	var (
		eg egrulOut
		rf rfmOut
		fs fsspOut
		wg sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		eg.data, eg.err = h.clients.EGRULByINN(r.Context(), req.INN)
	}()
	go func() {
		defer wg.Done()
		rf.data, rf.err = h.clients.RosfinmonScreen(r.Context(), req.INN)
	}()
	go func() {
		defer wg.Done()
		fs.data, fs.err = h.clients.FSSPByINN(r.Context(), req.INN)
	}()
	wg.Wait()

	resp := PrequalifyResponse{
		INN:           req.INN,
		OGRN:          req.OGRN,
		ShortNameHint: req.ShortName,
		CheckedAt:     time.Now().UTC(),
	}

	// EGRUL
	if eg.err != nil {
		resp.UnavailableSources = append(resp.UnavailableSources, "egrul")
		h.log.Warn("egrul lookup failed", "inn", req.INN, "err", eg.err)
	} else if eg.data != nil {
		resp.EGRULStatus = eg.data.Status
		resp.EGRULRegistrationDate = eg.data.RegisteredAt
		resp.EGRULAddress = eg.data.Address
		resp.EGRULCEOName = eg.data.CEO
		resp.EGRULFullName = eg.data.FullName
		resp.NameMatchesEGRUL = matchesShortName(req.ShortName, eg.data)
	}

	// Rosfinmon
	if rf.err != nil {
		resp.UnavailableSources = append(resp.UnavailableSources, "rosfinmon")
		h.log.Warn("rosfinmon lookup failed", "inn", req.INN, "err", rf.err)
	} else if rf.data != nil {
		resp.RosfinmonPresent = rf.data.Matched
	}

	// FSSP
	if fs.err != nil {
		resp.UnavailableSources = append(resp.UnavailableSources, "fssp")
		h.log.Warn("fssp lookup failed", "inn", req.INN, "err", fs.err)
	} else if fs.data != nil {
		resp.FSSPProceedingsCount = fs.data.Count
		resp.FSSPTotalDebtKopecks = fs.data.TotalDebtKopecks
	}

	resp.Decision, resp.DecisionReason = decide(&resp)
	writeJSON(w, http.StatusOK, resp)
}

// matchesShortName — простой fuzzy: убираем кавычки/тире/пробелы, сравниваем
// в нижнем регистре. Достаточно для базовой sanity-проверки соответствия
// заявленного и реального имени; полный fuzzy — за пределами этого PR.
func matchesShortName(claimed string, eg *extclients.EGRULData) bool {
	if eg == nil {
		return false
	}
	norm := func(s string) string {
		s = strings.ToLower(s)
		for _, r := range []string{"«", "»", "\"", "'", "-", " "} {
			s = strings.ReplaceAll(s, r, "")
		}
		return s
	}
	c := norm(claimed)
	return c != "" && (strings.Contains(norm(eg.ShortName), c) || strings.Contains(norm(eg.FullName), c))
}

// decide — derивация решения по внешним сигналам.
//
//   - Реджект: Росфинмон match, ЕГРЮЛ status != active, имя не совпадает.
//   - Manual review: значимые проблемы по ФССП (>10 производств).
//   - Proceed: всё остальное.
func decide(r *PrequalifyResponse) (string, string) {
	if r.RosfinmonPresent {
		return "reject", "Subject is on the 115-FZ Rosfinmonitoring list"
	}
	if r.EGRULStatus != "" && r.EGRULStatus != "active" {
		return "reject", "Legal entity status in EGRUL is not active: " + r.EGRULStatus
	}
	if r.EGRULFullName != "" && !r.NameMatchesEGRUL {
		return "manual_review", "Declared short_name does not match EGRUL record"
	}
	if r.FSSPProceedingsCount > 10 {
		return "manual_review", "More than 10 active FSSP proceedings"
	}
	return "proceed", ""
}
