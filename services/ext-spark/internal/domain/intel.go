package domain

import "errors"

// SparkIntel — корпоративная интеллигенция от СПАРК / Контур.Фокус.
//
// Дополняет ЕГРЮЛ маркетинговыми/риск-сигналами: «здоровье» компании, новостной
// дайджест, флаг санкций, налоговая нагрузка.
type SparkIntel struct {
	INN                 string   `json:"inn"`
	FinancialHealth     string   `json:"financial_health"` // "green" | "yellow" | "red"
	NewsSummary         []string `json:"news_summary"`
	Sanctions           bool     `json:"sanctions"`
	TaxesPaidKopecksYear int64   `json:"taxes_paid_kopecks_year"`
	EmployeesCount      int      `json:"employees_count"`
	RevenueKopecksYear  int64    `json:"revenue_kopecks_year"`
	YearOfData          int      `json:"year_of_data"`
}

// ErrInvalidINN — неподходящий ИНН.
var ErrInvalidINN = errors.New("invalid INN format: expected 10 (legal entity) or 12 (individual) digits")

// ValidateINN — 10 или 12 цифр.
func ValidateINN(inn string) error {
	if len(inn) != 10 && len(inn) != 12 {
		return ErrInvalidINN
	}
	for _, c := range inn {
		if c < '0' || c > '9' {
			return ErrInvalidINN
		}
	}
	return nil
}
