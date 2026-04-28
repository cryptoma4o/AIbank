package domain

import "context"

// UserRepository работает с таблицей platform.users (общая служебная схема,
// одна на весь кластер; см. ADR-0002).
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, u *User) error
}

// ApplicantRepository работает с таблицей applicants внутри схемы тенанта
// tnt_<tenantID>. Имя схемы выставляется через SET search_path при каждом
// вызове, без префиксов в SQL (см. ADR-0002).
type ApplicantRepository interface {
	GetByID(ctx context.Context, tenantID, id string) (*Applicant, error)
	GetByINN(ctx context.Context, tenantID, inn string) (*Applicant, error)
	Create(ctx context.Context, a *Applicant) error
	// CreateWithConsents выполняет атомарную регистрацию: вставляет applicant
	// и одновременно фиксирует его согласия (152-ФЗ ст. 9) в одной транзакции.
	// Если consents nil/empty — поведение совпадает с Create. Бизнес-валидация
	// (обязательное data_processing) — ответственность handler-слоя через
	// ValidateConsentsForRegistration; репозиторий лишь обеспечивает атомарность.
	CreateWithConsents(ctx context.Context, a *Applicant, consents []Consent) error
	// Forget реализует "право на забвение" по 152-ФЗ ст. 14: PII-поля
	// (FullName/Phone/Passport*/SNILS/INN) сбрасываются в пустую строку или
	// NULL, ставится forgotten_at/forgotten_by. Запись applicant'а сохраняется
	// для FK-целостности audit-логов (5 лет по 115-ФЗ — это другое требование).
	// Идемпотентно: повторный вызов на forgotten-applicant возвращает nil.
	Forget(ctx context.Context, tenantID, id, requester string) error
}
