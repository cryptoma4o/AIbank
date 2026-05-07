package domain

import "time"

// Role identifies what a User can do in the platform.
//
// Платформенные роли (нет привязки к тенанту) и банковские роли (всегда с tenant_id).
type Role string

const (
	RolePlatformAdmin         Role = "platform.admin"
	RoleBankOperator          Role = "bank.operator"
	RoleBankComplianceOfficer Role = "bank.compliance_officer"
	RoleBankAdmin             Role = "bank.admin"
	// RoleApplicant — клиент банка, проходящий онбординг (этап 1 формы).
	// Появилась в migrations/002_allow_applicant_role.sql.
	RoleApplicant Role = "applicant"
)

// IsValid возвращает true, если роль — одна из определённых выше.
func (r Role) IsValid() bool {
	switch r {
	case RolePlatformAdmin, RoleBankOperator, RoleBankComplianceOfficer, RoleBankAdmin, RoleApplicant:
		return true
	}
	return false
}

// IsPlatform возвращает true для ролей, не привязанных к конкретному банку.
func (r Role) IsPlatform() bool {
	return r == RolePlatformAdmin
}

// User — банковский оператор/администратор/комплаенс или платформенный админ.
// Хранится в общей служебной схеме platform.users (см. ADR-0002).
type User struct {
	ID           string    `json:"id"`
	TenantID     *string   `json:"tenant_id,omitempty"` // nil для платформенных ролей
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
