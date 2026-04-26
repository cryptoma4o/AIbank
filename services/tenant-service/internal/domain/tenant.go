package domain

import "time"

type TenantStatus string

const (
	TenantStatusTrial      TenantStatus = "trial"
	TenantStatusActive     TenantStatus = "active"
	TenantStatusSuspended  TenantStatus = "suspended"
	TenantStatusTerminated TenantStatus = "terminated"
)

type DeploymentMode string

const (
	DeploymentModeSaaS   DeploymentMode = "saas"
	DeploymentModeOnPrem DeploymentMode = "on_prem"
	DeploymentModeHybrid DeploymentMode = "hybrid"
)

type Tenant struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	BIK            string         `json:"bik"`
	INN            string         `json:"inn"`
	Status         TenantStatus   `json:"status"`
	DeploymentMode DeploymentMode `json:"deployment_mode"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type TenantConfig struct {
	TenantID      string    `json:"tenant_id"`
	RawConfig     []byte    `json:"raw_config"`
	SchemaVersion string    `json:"schema_version"`
	UpdatedAt     time.Time `json:"updated_at"`
}
