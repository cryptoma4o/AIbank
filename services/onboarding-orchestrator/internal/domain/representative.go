package domain

import (
	"context"
	"time"
)

// Representative — ЕИО или иной представитель юрлица (этап 4 формы).
// Соответствует $defs.Representative + расширенному Person из
// packages/domain-model/schema.json (v1.1.0). На заявку — несколько
// представителей; ровно один is_primary=true.
type Representative struct {
	ID                  string             `json:"id"`
	TenantID            string             `json:"tenant_id"`
	ApplicationID       string             `json:"application_id"`
	LegalEntityID       string             `json:"legal_entity_id"`
	LastName            string             `json:"last_name"`
	FirstName           string             `json:"first_name"`
	MiddleName          string             `json:"middle_name,omitempty"`
	BirthDate           string             `json:"birth_date"`
	BirthPlace          string             `json:"birth_place,omitempty"`
	Citizenship         []string           `json:"citizenship,omitempty"`
	INN                 string             `json:"inn,omitempty"`
	SNILS               string             `json:"snils,omitempty"`
	IDDocument          IDDocument         `json:"id_document"`
	RegistrationAddress *StructuredAddress `json:"registration_address,omitempty"`
	ActualAddress       *StructuredAddress `json:"actual_address,omitempty"`
	ForeignerInfo       *ForeignerInfo     `json:"foreigner_info,omitempty"`
	Authority           AuthorityInfo      `json:"authority"`
	PDLDeclaration      *PDLDeclaration    `json:"pdl_declaration,omitempty"`
	IsPrimary           bool               `json:"is_primary"`
	IsSignatory         bool               `json:"is_signatory"`
	CreatedAt           time.Time          `json:"created_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

// IDDocument — документ, удостоверяющий личность.
type IDDocument struct {
	DocType        string `json:"doc_type"` // passport_ru | passport_foreign | national_passport | refugee_certificate
	Series         string `json:"series,omitempty"`
	Number         string `json:"number"`
	IssueDate      string `json:"issue_date,omitempty"`
	ExpiryDate     string `json:"expiry_date,omitempty"`
	IssuedBy       string `json:"issued_by,omitempty"`
	DepartmentCode string `json:"department_code,omitempty"`
}

// AuthorityInfo — полномочия представителя.
type AuthorityInfo struct {
	Position             string `json:"position"`
	AuthorityBasis       string `json:"authority_basis"` // charter | protocol | power_of_attorney | order
	AuthorityDocNumber   string `json:"authority_doc_number,omitempty"`
	AuthorityDocDate     string `json:"authority_doc_date,omitempty"`
	SignatureSampleDocID string `json:"signature_sample_doc_id,omitempty"`
}

// ForeignerInfo — для иностранных граждан.
type ForeignerInfo struct {
	MigrationCardNumber     string `json:"migration_card_number,omitempty"`
	MigrationCardIssuedAt   string `json:"migration_card_issued_at,omitempty"`
	MigrationCardExpiresAt  string `json:"migration_card_expires_at,omitempty"`
	ResidenceDocType        string `json:"residence_doc_type,omitempty"` // rvp | vnj | visa
	ResidenceDocNumber      string `json:"residence_doc_number,omitempty"`
	ResidenceDocIssuedAt    string `json:"residence_doc_issued_at,omitempty"`
	ResidenceDocExpiresAt   string `json:"residence_doc_expires_at,omitempty"`
}

// PDLDeclaration — декларация о публичном должностном лице.
type PDLDeclaration struct {
	IsPDL    bool   `json:"is_pdl"`
	Category string `json:"category,omitempty"` // foreign | russian | international_organization
	Position string `json:"position,omitempty"`
	Relation string `json:"relation,omitempty"` // self | relative | representative
}

// RepresentativeRepository — порт.
type RepresentativeRepository interface {
	Upsert(ctx context.Context, r *Representative) error
	ListByApplication(ctx context.Context, tenantID, applicationID string) ([]*Representative, error)
	GetByID(ctx context.Context, tenantID, id string) (*Representative, error)
}
