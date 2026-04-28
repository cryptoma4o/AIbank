package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Features is the canonical input vector for the CatBoost-style scorer.
// Field names map 1:1 to factor identifiers in StubScorer.Score.
//
// Naming follows domain-model.md (snake_case identifiers in JSON, camelCase in
// Go). Anything money-related is in копейках (см. CLAUDE.md / domain-model.md).
type Features struct {
	CompanyAgeYears           float64 `json:"company_age_years"`
	HasBlockedOKVED           bool    `json:"has_blocked_okved"`
	RegistrationAddressIsMass bool    `json:"registration_address_is_mass"`
	UBOCount                  int     `json:"ubo_count"`
	HasForeignOwners          bool    `json:"has_foreign_owners"`
	CapitalKopecks            int64   `json:"capital_kopecks"`
	HasRosfinmonHit           bool    `json:"has_rosfinmon_hit"`
	HasFSSPHit                bool    `json:"has_fssp_hit"`
	IsInBankruptcy            bool    `json:"is_in_bankruptcy"`
	OKVED                     string  `json:"okved"`
}

// AsFacts produces the rule-engine fact map. Keys are stable strings that
// ship in default rules.yaml — changing any of them is a breaking change.
func (f Features) AsFacts() map[string]any {
	return map[string]any{
		"company_age_years":            f.CompanyAgeYears,
		"has_blocked_okved":            f.HasBlockedOKVED,
		"registration_address_is_mass": f.RegistrationAddressIsMass,
		"ubo_count":                    f.UBOCount,
		"has_foreign_owners":           f.HasForeignOwners,
		"capital_kopecks":              f.CapitalKopecks,
		"rosfinmon_hit":                f.HasRosfinmonHit,
		"fssp_hit":                     f.HasFSSPHit,
		"is_in_bankruptcy":             f.IsInBankruptcy,
		"okved":                        f.OKVED,
	}
}

// FeatureExtractor builds Features from a generic dictionary received from
// upstream services (BFF / orchestrator). Implementations should be lossy but
// never panic — missing fields fall back to zero values.
type FeatureExtractor interface {
	Extract(input map[string]any) (Features, error)
}

// MapExtractor is the default extractor used in the MVP. It accepts a flat
// dictionary keyed by snake_case strings and tolerates string/numeric mix.
type MapExtractor struct{}

// NewMapExtractor returns a ready-to-use MapExtractor.
func NewMapExtractor() *MapExtractor { return &MapExtractor{} }

// Extract is best-effort: every error is wrapped with the offending field name
// so the caller can show a useful 4xx to the client.
func (MapExtractor) Extract(input map[string]any) (Features, error) {
	if input == nil {
		return Features{}, nil
	}
	f := Features{}
	var err error
	if f.CompanyAgeYears, err = readFloat(input, "company_age_years"); err != nil {
		return f, err
	}
	if f.HasBlockedOKVED, err = readBool(input, "has_blocked_okved"); err != nil {
		return f, err
	}
	if f.RegistrationAddressIsMass, err = readBool(input, "registration_address_is_mass"); err != nil {
		return f, err
	}
	if v, err := readFloat(input, "ubo_count"); err != nil {
		return f, err
	} else {
		f.UBOCount = int(v)
	}
	if f.HasForeignOwners, err = readBool(input, "has_foreign_owners"); err != nil {
		return f, err
	}
	if v, err := readFloat(input, "capital_kopecks"); err != nil {
		return f, err
	} else {
		f.CapitalKopecks = int64(v)
	}
	if f.HasRosfinmonHit, err = readBool(input, "has_rosfinmon_hit"); err != nil {
		return f, err
	}
	if f.HasFSSPHit, err = readBool(input, "has_fssp_hit"); err != nil {
		return f, err
	}
	if f.IsInBankruptcy, err = readBool(input, "is_in_bankruptcy"); err != nil {
		return f, err
	}
	if v, ok := input["okved"]; ok {
		f.OKVED = strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return f, nil
}

func readFloat(m map[string]any, key string) (float64, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, nil
	}
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case string:
		if x == "" {
			return 0, nil
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, fmt.Errorf("field %q: not a number: %w", key, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("field %q: unsupported type %T", key, v)
	}
}

func readBool(m map[string]any, key string) (bool, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return false, nil
	}
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		if x == "" {
			return false, nil
		}
		b, err := strconv.ParseBool(x)
		if err != nil {
			return false, fmt.Errorf("field %q: not a bool: %w", key, err)
		}
		return b, nil
	case float64:
		return x != 0, nil
	case int:
		return x != 0, nil
	default:
		return false, fmt.Errorf("field %q: unsupported type %T", key, v)
	}
}
