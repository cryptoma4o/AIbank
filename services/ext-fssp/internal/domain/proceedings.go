package domain

import (
	"errors"
	"strings"
	"time"
)

// ProceedingsQuery — параметры запроса.
//
// Для ЮЛ: только INN. Для ФЛ: FullName + BirthDate (INN опционально).
type ProceedingsQuery struct {
	INN       string `json:"inn,omitempty"`
	FullName  string `json:"full_name,omitempty"`
	BirthDate string `json:"birth_date,omitempty"` // YYYY-MM-DD
}

// Proceeding — одно исполнительное производство.
type Proceeding struct {
	ID            string `json:"id"`
	Debtor        string `json:"debtor"`
	SumKopecks    int64  `json:"sum_kopecks"`
	Currency      string `json:"currency"` // RUB
	StartedAt     string `json:"started_at"`
	DocumentType  string `json:"document_type"` // "Исполнительный лист" | "Судебный приказ" | ...
	Court         string `json:"court"`
	Status        string `json:"status"` // "active" | "closed"
	BailiffOffice string `json:"bailiff_office,omitempty"`
}

// ProceedingsResult — ответ.
type ProceedingsResult struct {
	Subject         string       `json:"subject"` // "INN:7707..." или "PERSON:Имя|Дата"
	Count           int          `json:"count"`
	TotalDebtKopecks int64       `json:"total_debt_kopecks"`
	Items           []Proceeding `json:"items"`
}

// ErrInvalidParams — общая sentinel-ошибка валидации.
var ErrInvalidParams = errors.New("invalid params")

// ValidateINN — 10 (ЮЛ) или 12 (ИП) цифр.
func ValidateINN(inn string) error {
	if len(inn) != 10 && len(inn) != 12 {
		return errors.New("invalid INN format: expected 10 or 12 digits")
	}
	for _, c := range inn {
		if c < '0' || c > '9' {
			return errors.New("invalid INN format: only digits allowed")
		}
	}
	return nil
}

// ValidatePersonQuery — для эндпоинта /by-person.
//
// birth_date должен быть валидной календарной датой в формате YYYY-MM-DD.
func ValidatePersonQuery(fullName, birthDate string) error {
	if strings.TrimSpace(fullName) == "" {
		return errors.New("full_name is required")
	}
	if _, err := time.Parse("2006-01-02", birthDate); err != nil {
		return errors.New("birth_date must be valid date YYYY-MM-DD")
	}
	return nil
}
