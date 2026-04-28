package domain

import (
	"errors"
	"strings"
	"time"
)

// SubjectType — кого скринем.
type SubjectType string

const (
	SubjectPerson       SubjectType = "person"
	SubjectLegalEntity  SubjectType = "legal_entity"
)

// Identifiers — идентификаторы субъекта проверки.
//
// Для ФЛ обязателен FullName (опционально INN/BirthDate).
// Для ЮЛ обязателен INN или FullName.
type Identifiers struct {
	INN       string `json:"inn,omitempty"`
	FullName  string `json:"full_name,omitempty"`
	BirthDate string `json:"birth_date,omitempty"` // YYYY-MM-DD, только для person
}

// ScreeningRequest — запрос на скрининг по перечню 115-ФЗ.
type ScreeningRequest struct {
	SubjectType SubjectType `json:"subject_type"`
	Identifiers Identifiers `json:"identifiers"`
}

// ScreeningResult — результат проверки.
type ScreeningResult struct {
	RequestID       string                 `json:"request_id"`
	Matched         bool                   `json:"matched"`
	ListName        string                 `json:"list_name,omitempty"`        // "115-FZ:terrorist" | "115-FZ:extremist" | ""
	MatchConfidence float64                `json:"match_confidence"`           // 0.0..1.0
	SourceRecord    map[string]interface{} `json:"source_record,omitempty"`    // запись из перечня (для аудита)
	CheckedAt       time.Time              `json:"checked_at"`
}

// ListSnapshot — метаданные текущего перечня.
type ListSnapshot struct {
	LastUpdated time.Time `json:"last_updated"`
	ListCount   int       `json:"list_count"`
	Source      string    `json:"source"`  // "stub:115-FZ"
	Version     string    `json:"version"` // ISO date of snapshot
}

// SanctionedEntry — запись из синтетического перечня 115-ФЗ.
type SanctionedEntry struct {
	ID        string `json:"id"`
	INN       string `json:"inn,omitempty"`
	FullName  string `json:"full_name"`
	BirthDate string `json:"birth_date,omitempty"`
	ListType  string `json:"list_type"` // terrorist | extremist | proliferation
	AddedAt   string `json:"added_at"`
}

// Validate проверяет минимальную полноту запроса.
func (r ScreeningRequest) Validate() error {
	if r.SubjectType != SubjectPerson && r.SubjectType != SubjectLegalEntity {
		return errors.New("subject_type must be 'person' or 'legal_entity'")
	}
	hasINN := strings.TrimSpace(r.Identifiers.INN) != ""
	hasName := strings.TrimSpace(r.Identifiers.FullName) != ""
	if !hasINN && !hasName {
		return errors.New("at least one of identifiers.inn or identifiers.full_name is required")
	}
	if r.SubjectType == SubjectLegalEntity {
		if hasINN && len(r.Identifiers.INN) != 10 {
			return errors.New("legal_entity INN must be 10 digits")
		}
	}
	if r.SubjectType == SubjectPerson {
		if hasINN && len(r.Identifiers.INN) != 12 {
			return errors.New("person INN must be 12 digits")
		}
	}
	return nil
}
