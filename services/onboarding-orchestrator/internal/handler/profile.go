package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"aibank/onboarding-orchestrator/internal/domain"
	"aibank/onboarding-orchestrator/internal/repository"
)

// validOKVED — XX, XX.XX или XX.XX.XX.
var validOKVED = regexp.MustCompile(`^[0-9]{2}(\.[0-9]{1,2}){0,2}$`)

// ProfileHandler — этап 2 формы онбординга. Принимает анкету юрлица,
// делает upsert в legal_entity_profiles, возвращает свежее состояние.
type ProfileHandler struct {
	repo domain.LegalEntityProfileRepository
	log  *slog.Logger
}

func NewProfileHandler(repo domain.LegalEntityProfileRepository, log *slog.Logger) *ProfileHandler {
	return &ProfileHandler{repo: repo, log: log}
}

// Routes — chi-роутер для /v1/legal-entity-profiles.
func (h *ProfileHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

// upsertRequest зеркалит подмножество $defs.LegalEntityProfile из
// packages/domain-model/schema.json. id и created_at сервер выставляет сам.
type upsertRequest struct {
	TenantID              string                    `json:"tenant_id"`
	ApplicationID         string                    `json:"application_id"`
	LegalEntityID         string                    `json:"legal_entity_id"`
	OPFCode               string                    `json:"opf_code,omitempty"`
	RegistrationAuthority string                    `json:"registration_authority,omitempty"`
	AuthorizedCapital     *domain.MoneyAmount       `json:"authorized_capital,omitempty"`
	LegalAddress          *domain.StructuredAddress `json:"legal_address_struct,omitempty"`
	ActualAddress         *domain.StructuredAddress `json:"actual_address_struct,omitempty"`
	ActualSameAsLegal     bool                      `json:"actual_same_as_legal"`
	PostalAddress         *domain.StructuredAddress `json:"postal_address_struct,omitempty"`
	PostalSameAsLegal     bool                      `json:"postal_same_as_legal"`
	OKVEDMain             string                    `json:"okved_main_v2,omitempty"`
	OKVEDAdditional       []string                  `json:"okved_additional_v2,omitempty"`
	Licenses              []domain.LicenseInfo      `json:"licenses,omitempty"`
	SROMembership         []domain.SROMembership    `json:"sro_membership,omitempty"`
	Contacts              *domain.ContactInfo       `json:"contacts,omitempty"`
	EmployeesCount        *int                      `json:"employees_count,omitempty"`
	RevenueLastYear       *domain.MoneyAmount       `json:"revenue_last_year,omitempty"`
	TaxRegime             string                    `json:"tax_regime,omitempty"`
}

func (req upsertRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid (regex ^[a-z][a-z0-9_]{1,31}$)")
	}
	if strings.TrimSpace(req.ApplicationID) == "" {
		return errors.New("application_id is required")
	}
	if strings.TrimSpace(req.LegalEntityID) == "" {
		return errors.New("legal_entity_id is required")
	}
	if req.OKVEDMain != "" && !validOKVED.MatchString(req.OKVEDMain) {
		return errors.New("okved_main_v2 must match XX[.XX[.XX]]")
	}
	for i, code := range req.OKVEDAdditional {
		if !validOKVED.MatchString(code) {
			return errors.New("okved_additional_v2[" + intToStr(i) + "] invalid")
		}
	}
	if req.LegalAddress != nil && (req.LegalAddress.CountryCode == "" || req.LegalAddress.City == "") {
		return errors.New("legal_address_struct: country_code and city are required")
	}
	for i, l := range req.Licenses {
		if strings.TrimSpace(l.Number) == "" || strings.TrimSpace(l.Issuer) == "" || strings.TrimSpace(l.IssueDate) == "" {
			return errors.New("licenses[" + intToStr(i) + "]: number, issuer, issue_date are required")
		}
	}
	for i, m := range req.SROMembership {
		if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.RegNumber) == "" {
			return errors.New("sro_membership[" + intToStr(i) + "]: name and reg_number are required")
		}
	}
	if req.AuthorizedCapital != nil && (req.AuthorizedCapital.Currency == "" || len(req.AuthorizedCapital.Currency) != 3) {
		return errors.New("authorized_capital.currency must be 3 letters (ISO 4217)")
	}
	if req.RevenueLastYear != nil && (req.RevenueLastYear.Currency == "" || len(req.RevenueLastYear.Currency) != 3) {
		return errors.New("revenue_last_year.currency must be 3 letters (ISO 4217)")
	}
	return nil
}

// Upsert — POST /v1/legal-entity-profiles.
func (h *ProfileHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req upsertRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}

	// id = lep_<ULID-like uuid>. Если профиль уже существует, ON CONFLICT
	// в SQL обновит его — реальный id определит Postgres (мы можем сюда
	// передать любой, он будет конфликтовать по application_id и
	// проигнорируется EXCLUDED'ом). Для consistency читаем результат назад.
	profileID := "lep_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]

	p := &domain.LegalEntityProfile{
		ID:                    profileID,
		TenantID:              req.TenantID,
		ApplicationID:         req.ApplicationID,
		LegalEntityID:         req.LegalEntityID,
		OPFCode:               req.OPFCode,
		RegistrationAuthority: req.RegistrationAuthority,
		AuthorizedCapital:     req.AuthorizedCapital,
		LegalAddress:          req.LegalAddress,
		ActualAddress:         req.ActualAddress,
		ActualSameAsLegal:     req.ActualSameAsLegal,
		PostalAddress:         req.PostalAddress,
		PostalSameAsLegal:     req.PostalSameAsLegal,
		OKVEDMain:             req.OKVEDMain,
		OKVEDAdditional:       req.OKVEDAdditional,
		Licenses:              req.Licenses,
		SROMembership:         req.SROMembership,
		Contacts:              req.Contacts,
		EmployeesCount:        req.EmployeesCount,
		RevenueLastYear:       req.RevenueLastYear,
		TaxRegime:             req.TaxRegime,
		CreatedAt:             time.Now().UTC(),
	}
	if err := h.repo.Upsert(r.Context(), p); err != nil {
		h.log.Error("upsert profile", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save profile")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back profile", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

// GetByApplication — GET /v1/legal-entity-profiles/by-application/{applicationID}?tenant_id=...
func (h *ProfileHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	if id == "" {
		writeError(w, http.StatusBadRequest, "validation_failed", "applicationID is required")
		return
	}
	p, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "profile not found")
		return
	}
	if err != nil {
		h.log.Error("get profile", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func intToStr(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	out := ""
	for i > 0 {
		out = string(digits[i%10]) + out
		i /= 10
	}
	return out
}
