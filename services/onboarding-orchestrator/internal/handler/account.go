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

var (
	validCurrency       = regexp.MustCompile(`^[A-Z]{3}$`)
	validBIK            = regexp.MustCompile(`^[0-9]{9}$`)
	validAccountNumber  = regexp.MustCompile(`^[0-9]{20}$`)
	validAccountType    = map[string]bool{
		"settlement":       true,
		"deposit":          true,
		"loan":             true,
		"special":          true,
		"foreign_currency": true,
		"escrow":           true,
		"nominal":          true,
	}
	validSigningMethod = map[string]bool{
		"ukep": true, "sms_code": true, "handwritten": true,
	}
	validDBOChannel = map[string]bool{
		"web": true, "mobile": true, "api": true,
	}
)

// AccountHandler — этап 9 формы онбординга.
type AccountHandler struct {
	repo domain.AccountRepository
	log  *slog.Logger
}

func NewAccountHandler(repo domain.AccountRepository, log *slog.Logger) *AccountHandler {
	return &AccountHandler{repo: repo, log: log}
}

func (h *AccountHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.GetByApplication)
	return r
}

type accountRequest struct {
	TenantID             string                   `json:"tenant_id"`
	ApplicationID        string                   `json:"application_id"`
	LegalEntityID        string                   `json:"legal_entity_id"`
	AccountNumber        string                   `json:"account_number,omitempty"`
	BIK                  string                   `json:"bik,omitempty"`
	BankName             string                   `json:"bank_name,omitempty"`
	Currency             string                   `json:"currency"`
	AccountType          string                   `json:"account_type"`
	CorrespondentAccount string                   `json:"correspondent_account,omitempty"`
	TariffPlan           string                   `json:"tariff_plan,omitempty"`
	Agreements           domain.AccountAgreements `json:"agreements"`
	MonitoringProfileID  string                   `json:"monitoring_profile_id,omitempty"`
	ABSReference         string                   `json:"abs_reference,omitempty"`
	OpenedAt             *time.Time               `json:"opened_at,omitempty"`
}

func (req accountRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if strings.TrimSpace(req.ApplicationID) == "" || strings.TrimSpace(req.LegalEntityID) == "" {
		return errors.New("application_id and legal_entity_id are required")
	}
	if !validCurrency.MatchString(req.Currency) {
		return errors.New("currency must be 3 letters (ISO 4217)")
	}
	if !validAccountType[req.AccountType] {
		return errors.New("account_type must be settlement|deposit|loan|special|foreign_currency|escrow|nominal")
	}
	if req.AccountNumber != "" && !validAccountNumber.MatchString(req.AccountNumber) {
		return errors.New("account_number must be 20 digits")
	}
	if req.BIK != "" && !validBIK.MatchString(req.BIK) {
		return errors.New("bik must be 9 digits")
	}
	if req.CorrespondentAccount != "" && !validAccountNumber.MatchString(req.CorrespondentAccount) {
		return errors.New("correspondent_account must be 20 digits")
	}
	// AccountAgreements валидируется обязательно — это фокус этапа 9.
	a := req.Agreements
	if !a.AgreementAcceptance {
		return errors.New("agreements.agreement_acceptance is required (true)")
	}
	if a.AgreementAcceptedAt.IsZero() {
		return errors.New("agreements.agreement_accepted_at is required")
	}
	if !a.PersonalDataConsent {
		return errors.New("agreements.personal_data_consent is required (true) per 152-FZ")
	}
	if !validSigningMethod[a.SigningMethod] {
		return errors.New("agreements.signing_method must be ukep|sms_code|handwritten")
	}
	if a.SigningMethod == "ukep" && strings.TrimSpace(a.UKEPCertificateSerial) == "" {
		return errors.New("agreements.ukep_certificate_serial is required when signing_method=ukep")
	}
	if a.DBOAgreement && len(a.DBOChannels) == 0 {
		return errors.New("agreements.dbo_channels[] is required when dbo_agreement=true")
	}
	for i, ch := range a.DBOChannels {
		if !validDBOChannel[ch] {
			return errors.New("agreements.dbo_channels[" + intToStr(i) + "] must be web|mobile|api")
		}
	}
	return nil
}

func (h *AccountHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req accountRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := "acc_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	a := &domain.Account{
		ID:                   id,
		TenantID:             req.TenantID,
		ApplicationID:        req.ApplicationID,
		LegalEntityID:        req.LegalEntityID,
		AccountNumber:        req.AccountNumber,
		BIK:                  req.BIK,
		BankName:             req.BankName,
		Currency:             req.Currency,
		AccountType:          req.AccountType,
		CorrespondentAccount: req.CorrespondentAccount,
		TariffPlan:           req.TariffPlan,
		Agreements:           req.Agreements,
		MonitoringProfileID:  req.MonitoringProfileID,
		ABSReference:         req.ABSReference,
		OpenedAt:             req.OpenedAt,
		CreatedAt:            time.Now().UTC(),
	}
	if err := h.repo.Upsert(r.Context(), a); err != nil {
		h.log.Error("upsert account", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save account")
		return
	}
	stored, err := h.repo.GetByApplication(r.Context(), req.TenantID, req.ApplicationID)
	if err != nil {
		h.log.Error("read back account", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *AccountHandler) GetByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	a, err := h.repo.GetByApplication(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}
	if err != nil {
		h.log.Error("get account", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, a)
}
