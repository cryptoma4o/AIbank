package domain

import (
	"context"
	"errors"
)

// ErrNotFound is the canonical "not found" sentinel for risk assessments.
// Mirrors the convention from services/tenant-service/internal/repository.
var ErrNotFound = errors.New("risk assessment not found")

// RiskAssessmentRepository persists and reads RiskAssessment aggregates.
// Implementations MUST honour the schema-per-tenant isolation contract from
// ADR-0002: tenantID is required on every read and write.
type RiskAssessmentRepository interface {
	Create(ctx context.Context, a *RiskAssessment) error
	GetByID(ctx context.Context, tenantID, id string) (*RiskAssessment, error)
	ListByApplication(ctx context.Context, tenantID, applicationID string) ([]*RiskAssessment, error)
}
