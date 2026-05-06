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
	validBirthDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	validIDDocType = map[string]bool{
		"passport_ru": true, "passport_foreign": true, "national_passport": true, "refugee_certificate": true,
	}
	validAuthorityBasis = map[string]bool{
		"charter": true, "protocol": true, "power_of_attorney": true, "order": true,
	}
	validPDLCategory = map[string]bool{
		"foreign": true, "russian": true, "international_organization": true,
	}
	validPDLRelation = map[string]bool{
		"self": true, "relative": true, "representative": true,
	}
)

// RepresentativeHandler — этап 4 формы онбординга.
type RepresentativeHandler struct {
	repo domain.RepresentativeRepository
	log  *slog.Logger
}

func NewRepresentativeHandler(repo domain.RepresentativeRepository, log *slog.Logger) *RepresentativeHandler {
	return &RepresentativeHandler{repo: repo, log: log}
}

func (h *RepresentativeHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Use(withPlatformAuth)
	r.Post("/", h.Upsert)
	r.Get("/by-application/{applicationID}", h.ListByApplication)
	r.Get("/{id}", h.Get)
	return r
}

type representativeRequest struct {
	ID                  string                    `json:"id,omitempty"`
	TenantID            string                    `json:"tenant_id"`
	ApplicationID       string                    `json:"application_id"`
	LegalEntityID       string                    `json:"legal_entity_id"`
	LastName            string                    `json:"last_name"`
	FirstName           string                    `json:"first_name"`
	MiddleName          string                    `json:"middle_name,omitempty"`
	BirthDate           string                    `json:"birth_date"`
	BirthPlace          string                    `json:"birth_place,omitempty"`
	Citizenship         []string                  `json:"citizenship,omitempty"`
	INN                 string                    `json:"inn,omitempty"`
	SNILS               string                    `json:"snils,omitempty"`
	IDDocument          domain.IDDocument         `json:"id_document"`
	RegistrationAddress *domain.StructuredAddress `json:"registration_address,omitempty"`
	ActualAddress       *domain.StructuredAddress `json:"actual_address,omitempty"`
	ForeignerInfo       *domain.ForeignerInfo     `json:"foreigner_info,omitempty"`
	Authority           domain.AuthorityInfo      `json:"authority"`
	PDLDeclaration      *domain.PDLDeclaration    `json:"pdl_declaration,omitempty"`
	IsPrimary           bool                      `json:"is_primary"`
	IsSignatory         bool                      `json:"is_signatory"`
}

func (req representativeRequest) validate() error {
	if !validTenantID.MatchString(req.TenantID) {
		return errors.New("tenant_id is invalid")
	}
	if strings.TrimSpace(req.ApplicationID) == "" || strings.TrimSpace(req.LegalEntityID) == "" {
		return errors.New("application_id and legal_entity_id are required")
	}
	if strings.TrimSpace(req.LastName) == "" || strings.TrimSpace(req.FirstName) == "" {
		return errors.New("last_name and first_name are required")
	}
	if !validBirthDate.MatchString(req.BirthDate) {
		return errors.New("birth_date must be YYYY-MM-DD")
	}
	if !validIDDocType[req.IDDocument.DocType] {
		return errors.New("id_document.doc_type must be passport_ru|passport_foreign|national_passport|refugee_certificate")
	}
	if strings.TrimSpace(req.IDDocument.Number) == "" {
		return errors.New("id_document.number is required")
	}
	if strings.TrimSpace(req.Authority.Position) == "" {
		return errors.New("authority.position is required")
	}
	if !validAuthorityBasis[req.Authority.AuthorityBasis] {
		return errors.New("authority.authority_basis must be charter|protocol|power_of_attorney|order")
	}
	if req.PDLDeclaration != nil && req.PDLDeclaration.IsPDL {
		if !validPDLCategory[req.PDLDeclaration.Category] {
			return errors.New("pdl_declaration.category must be foreign|russian|international_organization when is_pdl=true")
		}
		if !validPDLRelation[req.PDLDeclaration.Relation] {
			return errors.New("pdl_declaration.relation must be self|relative|representative")
		}
		if strings.TrimSpace(req.PDLDeclaration.Position) == "" {
			return errors.New("pdl_declaration.position is required when is_pdl=true")
		}
	}
	for _, c := range req.Citizenship {
		if len(c) != 2 {
			return errors.New("citizenship[*] must be ISO 3166 alpha-2 (2 letters)")
		}
	}
	return nil
}

func (h *RepresentativeHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	var req representativeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
		return
	}
	id := req.ID
	if id == "" {
		id = "rep_" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:30]
	}
	rep := &domain.Representative{
		ID:                  id,
		TenantID:            req.TenantID,
		ApplicationID:       req.ApplicationID,
		LegalEntityID:       req.LegalEntityID,
		LastName:            req.LastName,
		FirstName:           req.FirstName,
		MiddleName:          req.MiddleName,
		BirthDate:           req.BirthDate,
		BirthPlace:          req.BirthPlace,
		Citizenship:         req.Citizenship,
		INN:                 req.INN,
		SNILS:               req.SNILS,
		IDDocument:          req.IDDocument,
		RegistrationAddress: req.RegistrationAddress,
		ActualAddress:       req.ActualAddress,
		ForeignerInfo:       req.ForeignerInfo,
		Authority:           req.Authority,
		PDLDeclaration:      req.PDLDeclaration,
		IsPrimary:           req.IsPrimary,
		IsSignatory:         req.IsSignatory,
		CreatedAt:           time.Now().UTC(),
	}
	if err := h.repo.Upsert(r.Context(), rep); err != nil {
		h.log.Error("upsert representative", "tenant", req.TenantID, "application", req.ApplicationID, "err", err)
		writeError(w, http.StatusInternalServerError, "upsert_failed", "failed to save representative")
		return
	}
	stored, err := h.repo.GetByID(r.Context(), req.TenantID, id)
	if err != nil {
		h.log.Error("read back representative", "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *RepresentativeHandler) ListByApplication(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "applicationID")
	out, err := h.repo.ListByApplication(r.Context(), tenantID, id)
	if err != nil {
		h.log.Error("list representatives", "application", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *RepresentativeHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if !validTenantID.MatchString(tenantID) {
		writeError(w, http.StatusBadRequest, "validation_failed", "tenant_id query param is invalid")
		return
	}
	id := chi.URLParam(r, "id")
	rep, err := h.repo.GetByID(r.Context(), tenantID, id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "representative not found")
		return
	}
	if err != nil {
		h.log.Error("get representative", "id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
