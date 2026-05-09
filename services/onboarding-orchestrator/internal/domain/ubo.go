package domain

import (
	"context"
	"time"
)

// UBOGraph — граф владения юрлицом (этап 5 формы онбординга).
// Соответствует packages/domain-model/schema.json $defs.UBOGraph (v1.1.0).
// Один граф на заявку (UNIQUE INDEX в БД).
type UBOGraph struct {
	ID                   string             `json:"id"`
	TenantID             string             `json:"tenant_id"`
	ApplicationID        string             `json:"application_id"`
	LegalEntityID        string             `json:"legal_entity_id"`
	Nodes                []UBONode          `json:"nodes"`
	Edges                []UBOEdge          `json:"edges"`
	OwnershipChains      []UBOOwnershipChain `json:"ownership_chains,omitempty"`
	NoUBOReason          string             `json:"no_ubo_reason,omitempty"`
	EIOAsUBOConfirmation bool               `json:"eio_as_ubo_confirmation"`
	DiagramDocID         string             `json:"diagram_doc_id,omitempty"`
	ComputedAt           *time.Time         `json:"computed_at,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// UBONode — узел графа владения (физлицо или ЮЛ).
type UBONode struct {
	ID             string  `json:"id"`
	NodeType       string  `json:"node_type"` // "person" | "legal_entity"
	Name           string  `json:"name"`
	PersonID       string  `json:"person_id,omitempty"`
	LegalEntityID  string  `json:"legal_entity_id,omitempty"`
	DirectStake    float64 `json:"direct_stake"`
	EffectiveStake float64 `json:"effective_stake"`
	IsUBO          bool    `json:"is_ubo"`
	// Для UBO-физлиц расширенные поля (этап 5 — паспортные данные,
	// FATCA/CRS, control basis). Используется когда NodeType == "person".
	ControlBasis      string             `json:"control_basis,omitempty"`
	FATCADeclaration  *FATCADeclaration  `json:"fatca_declaration,omitempty"`
}

// UBOEdge — ребро владения между узлами (направленное).
type UBOEdge struct {
	FromNodeID string  `json:"from_node_id"`
	ToNodeID   string  `json:"to_node_id"`
	Stake      float64 `json:"stake"`
	DocumentID string  `json:"document_id,omitempty"`
}

// UBOOwnershipChain — линейная цепочка владения от UBO-физлица к юрлицу-заявителю.
type UBOOwnershipChain struct {
	UBONodeID         string               `json:"ubo_node_id"`
	Links             []OwnershipChainLink `json:"links"`
	IsSoleBeneficiary bool                 `json:"is_sole_beneficiary"`
}

// OwnershipChainLink — одно звено цепочки владения.
type OwnershipChainLink struct {
	Level                int     `json:"level"`
	EntityName           string  `json:"entity_name"`
	EntityINNOrRegNumber string  `json:"entity_inn_or_reg_number,omitempty"`
	Country              string  `json:"country,omitempty"`
	SharePercent         float64 `json:"share_percent"`
}

// FATCADeclaration — налоговое резидентство (этап 5).
type FATCADeclaration struct {
	TaxResidencyCountries []string             `json:"tax_residency_countries,omitempty"`
	TINPerCountry         []FATCATIN           `json:"tin_per_country,omitempty"`
	USPerson              bool                 `json:"us_person"`
	FormDocID             string               `json:"form_doc_id,omitempty"`
}

// FATCATIN — TIN по конкретной стране резидентства.
type FATCATIN struct {
	Country string `json:"country"`
	TIN     string `json:"tin"`
}

// UBOGraphRepository — порт хранилища.
type UBOGraphRepository interface {
	Upsert(ctx context.Context, g *UBOGraph) error
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*UBOGraph, error)
}
