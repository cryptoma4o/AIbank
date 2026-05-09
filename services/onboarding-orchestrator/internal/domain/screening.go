package domain

import (
	"context"
	"time"
)

// ScreeningResultSet — сводный результат AML-проверок по заявке (этап 7).
// Соответствует packages/domain-model/schema.json $defs.ScreeningResultSet
// (v1.1.0). Один-к-одному с Application.
type ScreeningResultSet struct {
	ID                    string             `json:"id"`
	TenantID              string             `json:"tenant_id"`
	ApplicationID         string             `json:"application_id"`
	SanctionsResults      []ScreeningResult  `json:"sanctions_results,omitempty"`
	PEPResults            []ScreeningResult  `json:"pep_results,omitempty"`
	AdverseMediaHits      []AdverseMediaHit  `json:"adverse_media_hits,omitempty"`
	OKVEDConsistencyScore *float64           `json:"okved_consistency_score,omitempty"`
	TurnoverRealismScore  *float64           `json:"turnover_realism_score,omitempty"`
	AntiFraudSignals      *AntiFraudSignals  `json:"anti_fraud_signals,omitempty"`
	PerformedAt           time.Time          `json:"performed_at"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

// ScreeningResult — отметка matching по конкретному watch list.
type ScreeningResult struct {
	ListName          string     `json:"list_name"`
	MatchLevel        string     `json:"match_level"` // "match" | "partial_match" | "no_match"
	Score             float64    `json:"score,omitempty"`
	MatchedEntityName string     `json:"matched_entity_name,omitempty"`
	CheckedAt         *time.Time `json:"checked_at,omitempty"`
}

// AdverseMediaHit — найденная негативная публикация.
type AdverseMediaHit struct {
	Category    string `json:"category"` // fraud|money_laundering|terrorism_financing|sanctions|corruption|other
	Title       string `json:"title"`
	URL         string `json:"url"`
	Source      string `json:"source,omitempty"`
	PublishedAt string `json:"published_at"`
	Summary     string `json:"summary,omitempty"`
}

// AntiFraudSignals — фрод-сигналы при подаче заявки.
type AntiFraudSignals struct {
	DeviceFingerprint string       `json:"device_fingerprint,omitempty"`
	IPAddress         string       `json:"ip_address,omitempty"`
	Geolocation       *GeoLocation `json:"geolocation,omitempty"`
}

// GeoLocation — гео-метка по IP.
type GeoLocation struct {
	Country   string  `json:"country,omitempty"`
	City      string  `json:"city,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// ScreeningResultSetRepository — порт.
type ScreeningResultSetRepository interface {
	Upsert(ctx context.Context, s *ScreeningResultSet) error
	GetByApplication(ctx context.Context, tenantID, applicationID string) (*ScreeningResultSet, error)
}
