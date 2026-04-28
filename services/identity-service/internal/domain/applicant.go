package domain

import "time"

// Applicant — клиент банка (физлицо, ИП или представитель юрлица), подающий
// заявку на открытие счёта/кредит. Хранится в схеме тенанта tnt_<id>.
//
// Поля PassportSeries / PassportNumber / SNILS дополнительно шифруются на
// уровне приложения через Vault Transit (см. docs/security-architecture.md
// § 5.2). В БД — BYTEA-колонки *_enc; в Go — обычные plaintext-строки, которые
// repository-слой шифрует/расшифровывает прозрачно для handler'ов.
type Applicant struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	INN            string     `json:"inn"`
	Phone          string     `json:"phone"`
	FullName       string     `json:"full_name"`
	PassportSeries string     `json:"passport_series,omitempty"`
	PassportNumber string     `json:"passport_number,omitempty"`
	SNILS          string     `json:"snils,omitempty"`
	ESIAVerified   bool       `json:"esia_verified"`
	ESIASubject    string     `json:"esia_subject,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	// ForgottenAt — заполняется при right-to-be-forgotten (152-ФЗ ст. 14).
	// PII-поля (FullName/Phone/INN/Passport/SNILS) скрабятся, но строка
	// остаётся для FK-целостности audit-логов (5 лет по 115-ФЗ).
	ForgottenAt *time.Time `json:"forgotten_at,omitempty"`
	ForgottenBy string     `json:"forgotten_by,omitempty"`
}
