// Package domain содержит модели и интерфейсы репозиториев identity-service.
//
// Файл consent.go реализует требование 152-ФЗ ст. 9 «Согласие субъекта на
// обработку персональных данных»:
//
//   - data_processing — обязательное согласие, без которого регистрация
//     заявителя запрещена (банк не имеет права обрабатывать ПДн без него)
//   - marketing       — опциональное (рассылки, опросы)
//   - biometrics      — опциональное (ЕБС, голосовая идентификация)
//
// Запись согласия — append-only: при отзыве (Revoke) создаётся НОВАЯ строка
// с заполненным revoked_at, а не UPDATE существующей. Это обеспечивает
// неизменяемый журнал согласий, который требуется и для 152-ФЗ, и для
// внутреннего комплаенса.
package domain

import (
	"context"
	"errors"
	"time"
)

// ConsentType — каноническое имя согласия.
//
// Версии (string `version`) хранятся отдельно: один и тот же тип согласия
// переиздаётся, когда банк меняет редакцию текста (например, "v2026-04").
type ConsentType string

const (
	// ConsentDataProcessing — согласие на обработку ПДн (152-ФЗ ст. 9).
	// ОБЯЗАТЕЛЬНОЕ при регистрации applicant — без granted=true создание
	// заявителя запрещено бизнес-правилом.
	ConsentDataProcessing ConsentType = "data_processing"
	// ConsentMarketing — согласие на маркетинговые коммуникации (опциональное).
	ConsentMarketing ConsentType = "marketing"
	// ConsentBiometrics — согласие на обработку биометрии (опциональное; ЕБС).
	ConsentBiometrics ConsentType = "biometrics"
)

// IsValid возвращает true для одного из определённых типов согласий.
func (t ConsentType) IsValid() bool {
	switch t {
	case ConsentDataProcessing, ConsentMarketing, ConsentBiometrics:
		return true
	}
	return false
}

// ErrDataProcessingConsentRequired возвращается, когда при регистрации
// applicant'а в массиве consents отсутствует data_processing с granted=true.
// Это нарушение 152-ФЗ ст. 9 — без явного согласия обработка ПДн запрещена.
var ErrDataProcessingConsentRequired = errors.New(
	"согласие на обработку персональных данных (data_processing) обязательно при регистрации",
)

// ErrConsentTypeInvalid — при попытке записать согласие неизвестного типа.
var ErrConsentTypeInvalid = errors.New("неизвестный тип согласия")

// Consent — единичная запись согласия субъекта ПДн.
//
// Хранится в схеме тенанта tnt_<id> в таблице consents. Append-only:
// UPDATE/DELETE запрещены триггером БД. Активное согласие — то, у которого
// revoked_at IS NULL; уникальность активной записи поддержана частичным
// уникальным индексом UNIQUE(applicant_id, consent_type) WHERE revoked_at IS NULL.
type Consent struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	ApplicantID string      `json:"applicant_id"`
	ConsentType ConsentType `json:"consent_type"`
	// Granted — true означает «согласие дано», false — «отказ зафиксирован».
	// Отказ от опциональных согласий (marketing/biometrics) допустим и не
	// блокирует регистрацию; отказ от data_processing — нет (см. ErrDataProcessingConsentRequired).
	Granted bool `json:"granted"`
	// Version — текстовый идентификатор редакции соглашения, например "v2026-04".
	// Управляется конфигурацией тенанта (см. configs/tenants/*/consent_versions.yaml — TODO).
	Version string `json:"version"`
	// IPAddress — адрес, с которого прислана регистрация (X-Forwarded-For или RemoteAddr).
	IPAddress string `json:"ip_address,omitempty"`
	// UserAgent — клиентский User-Agent на момент дачи согласия.
	UserAgent string `json:"user_agent,omitempty"`
	// Signature — необязательный hash УКЭП-подписи (если согласие подписано
	// клиентом по 63-ФЗ). Формат hex(SHA-256(signature_bytes)).
	Signature string `json:"signature,omitempty"`
	// RecordedAt — время записи в БД (UTC).
	RecordedAt time.Time `json:"recorded_at"`
	// RevokedAt — UTC-время отзыва согласия. Active consent → nil.
	// Для отзыва записывается НОВАЯ строка с заполненным revoked_at.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// IsActive возвращает true, если согласие действует (не отозвано).
func (c *Consent) IsActive() bool {
	return c != nil && c.RevokedAt == nil
}

// ConsentRepository — операции над consents в схеме тенанта.
//
// Append-only по контракту: Update метод не предоставляется. Revoke создаёт
// новую строку (revoke-row) с RevokedAt=NOW и Granted=false; уникальный
// индекс по (applicant_id, consent_type) WHERE revoked_at IS NULL гарантирует,
// что в любой момент существует не более одного активного согласия каждого типа.
type ConsentRepository interface {
	// Record записывает согласие в БД. Применяется и при первоначальной
	// регистрации, и при повторной выдаче после отзыва.
	Record(ctx context.Context, c *Consent) error

	// ListByApplicant возвращает АКТИВНЫЕ согласия заявителя
	// (revoked_at IS NULL), отсортированные по типу.
	ListByApplicant(ctx context.Context, tenantID, applicantID string) ([]*Consent, error)

	// Revoke помечает активное согласие данного типа как отозванное.
	// Реализация: вставляет новую строку с revoked_at=NOW и предварительно
	// «закрывает» старую активную запись через UPDATE revoked_at = NOW
	// (это единственное доменно-разрешённое UPDATE для consents — на уровне
	// триггера разрешается ТОЛЬКО переход NULL → not-NULL для revoked_at;
	// все прочие UPDATE/DELETE запрещены).
	Revoke(ctx context.Context, tenantID, applicantID string, t ConsentType) error
}

// ValidateConsentsForRegistration проверяет бизнес-правило 152-ФЗ:
// в массиве согласий ОБЯЗАНО присутствовать data_processing с granted=true,
// иначе регистрация applicant'а запрещена. Все типы согласий должны быть
// валидными (см. ConsentType.IsValid).
//
// Не имеет побочных эффектов и пригодна для использования в handler/validator.
func ValidateConsentsForRegistration(consents []Consent) error {
	hasDataProcessing := false
	for i := range consents {
		if !consents[i].ConsentType.IsValid() {
			return ErrConsentTypeInvalid
		}
		if consents[i].ConsentType == ConsentDataProcessing && consents[i].Granted {
			hasDataProcessing = true
		}
	}
	if !hasDataProcessing {
		return ErrDataProcessingConsentRequired
	}
	return nil
}
