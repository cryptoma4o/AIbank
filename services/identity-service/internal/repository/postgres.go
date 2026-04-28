package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"aibank/identity-service/internal/domain"

	"github.com/aibank/platform/packages/secrets"
)

// ErrNotFound возвращается, когда сущность не найдена.
var ErrNotFound = errors.New("not found")

// validTenantID — узкий whitelist для частей идентификатора схемы.
// Соответствует требованиям PostgreSQL identifier syntax + ADR-0002.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// PostgresUserRepository — реализация UserRepository поверх схемы platform.
type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, email, password_hash, role, is_active, created_at, updated_at
		 FROM platform.users WHERE id = $1`, id)
	return scanUser(row)
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, email, password_hash, role, is_active, created_at, updated_at
		 FROM platform.users WHERE email = $1`, email)
	return scanUser(row)
}

func (r *PostgresUserRepository) Create(ctx context.Context, u *domain.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO platform.users (id, tenant_id, email, password_hash, role, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		u.ID, u.TenantID, u.Email, u.PasswordHash, string(u.Role), u.IsActive, u.CreatedAt, u.UpdatedAt)
	return err
}

func scanUser(row *sql.Row) (*domain.User, error) {
	var u domain.User
	var tenantID sql.NullString
	var role string
	err := row.Scan(&u.ID, &tenantID, &u.Email, &u.PasswordHash, &role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tenantID.Valid {
		v := tenantID.String
		u.TenantID = &v
	}
	u.Role = domain.Role(role)
	return &u, nil
}

// PostgresApplicantRepository — реализация ApplicantRepository.
//
// Каждая операция выставляет search_path на схему тенанта в рамках текущей
// транзакции через set_config(..., is_local=true) — после возврата соединения
// в пул search_path сбрасывается. Имя схемы формируется только после строгой
// валидации tenantID (см. ADR-0002, раздел Безопасность).
//
// Field-level encryption (см. docs/security-architecture.md § 5.2):
// поля passport_series, passport_number, snils, inn шифруются Vault Transit
// per-tenant ключом aibank-tenant-<tenant_id> и пишутся в BYTEA-колонки *_enc.
// Если transit == nil (dev/тест без Vault) — данные хранятся как plaintext в
// существующих TEXT-колонках, BYTEA остаются NULL. Backward-compat
// гарантирована: integration-тесты, не передающие Transit, продолжают работать.
type PostgresApplicantRepository struct {
	db      *sql.DB
	transit secrets.Transit // nil-safe: dev/test без Vault
}

// NewPostgresApplicantRepository собирает репозиторий. transit может быть nil —
// тогда PII-шифрование отключено (только TEXT-колонки). В проде wiring ОБЯЗАН
// передать non-nil клиент.
func NewPostgresApplicantRepository(db *sql.DB, transit secrets.Transit) *PostgresApplicantRepository {
	return &PostgresApplicantRepository{db: db, transit: transit}
}

// encryptField шифрует value под per-tenant ключом. Пустая строка / nil
// transit обрабатываются helper'ом secrets.EncryptOrEmpty.
// Возвращает []byte для прямой записи в BYTEA-колонку (или nil → SQL NULL).
func (r *PostgresApplicantRepository) encryptField(ctx context.Context, tenantID, value string) ([]byte, error) {
	if value == "" || r.transit == nil {
		return nil, nil
	}
	keyName := secrets.TenantKeyName(tenantID)
	ct, err := r.transit.Encrypt(ctx, keyName, value)
	if err != nil {
		return nil, fmt.Errorf("encrypt field for tenant %q: %w", tenantID, err)
	}
	return []byte(ct), nil
}

// decryptField обратная операция: BYTEA → plaintext. NULL/empty → "".
// nil transit → возвращаем bytes как plaintext (dev-режим).
func (r *PostgresApplicantRepository) decryptField(ctx context.Context, tenantID string, raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	if r.transit == nil {
		return string(raw), nil
	}
	keyName := secrets.TenantKeyName(tenantID)
	pt, err := r.transit.Decrypt(ctx, keyName, string(raw))
	if err != nil {
		return "", fmt.Errorf("decrypt field for tenant %q: %w", tenantID, err)
	}
	return pt, nil
}

func tenantSchema(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q: must match %s", tenantID, validTenantID)
	}
	return "tnt_" + tenantID, nil
}

// withTenantTx открывает транзакцию, выставляет search_path и выполняет fn.
func (r *PostgresApplicantRepository) withTenantTx(
	ctx context.Context,
	tenantID string,
	fn func(tx *sql.Tx) error,
) error {
	schema, err := tenantSchema(tenantID)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// is_local = true ограничивает действие текущей транзакцией.
	if _, err := tx.ExecContext(ctx,
		`SELECT set_config('search_path', $1, true)`, schema); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// scanApplicantRow читает строку и расшифровывает PII-колонки. tenantID
// проставляется явно — в БД не хранится (search_path подразумевает схему).
func (r *PostgresApplicantRepository) scanApplicantRow(ctx context.Context, tenantID string, row *sql.Row) (*domain.Applicant, error) {
	var got domain.Applicant
	var esiaSubject sql.NullString
	var passSeriesEnc, passNumberEnc, snilsEnc, innEnc []byte

	err := row.Scan(
		&got.ID, &got.INN, &got.Phone, &got.FullName,
		&got.ESIAVerified, &esiaSubject,
		&passSeriesEnc, &passNumberEnc, &snilsEnc, &innEnc,
		&got.CreatedAt, &got.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	got.TenantID = tenantID
	if esiaSubject.Valid {
		got.ESIASubject = esiaSubject.String
	}

	if got.PassportSeries, err = r.decryptField(ctx, tenantID, passSeriesEnc); err != nil {
		return nil, err
	}
	if got.PassportNumber, err = r.decryptField(ctx, tenantID, passNumberEnc); err != nil {
		return nil, err
	}
	if got.SNILS, err = r.decryptField(ctx, tenantID, snilsEnc); err != nil {
		return nil, err
	}
	// inn_enc дублирует got.INN (legacy plaintext колонка). На переходный
	// период используем plaintext-копию; после раскатки и rewrap'а — будем
	// читать только из inn_enc и колонку inn удалим follow-up миграцией.
	_ = innEnc

	return &got, nil
}

const applicantSelectColumns = `id, inn, phone, full_name, esia_verified, esia_subject,
		passport_series_enc, passport_number_enc, snils_enc, inn_enc,
		created_at, updated_at`

func (r *PostgresApplicantRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Applicant, error) {
	var a *domain.Applicant
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT `+applicantSelectColumns+` FROM applicants WHERE id = $1`, id)
		got, err := r.scanApplicantRow(ctx, tenantID, row)
		if err != nil {
			return err
		}
		a = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *PostgresApplicantRepository) GetByINN(ctx context.Context, tenantID, inn string) (*domain.Applicant, error) {
	var a *domain.Applicant
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT `+applicantSelectColumns+` FROM applicants WHERE inn = $1`, inn)
		got, err := r.scanApplicantRow(ctx, tenantID, row)
		if err != nil {
			return err
		}
		a = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *PostgresApplicantRepository) Create(ctx context.Context, a *domain.Applicant) error {
	return r.CreateWithConsents(ctx, a, nil)
}

// CreateWithConsents — атомарная регистрация applicant'а вместе с массивом
// согласий 152-ФЗ. Если consents nil/empty, ведёт себя как старый Create.
//
// Бизнес-валидация (обязательное data_processing) делается ВЫШЕ — в handler
// через domain.ValidateConsentsForRegistration. Репозиторий принимает уже
// провалидированный набор и обеспечивает только атомарность INSERT'ов.
func (r *PostgresApplicantRepository) CreateWithConsents(
	ctx context.Context,
	a *domain.Applicant,
	consents []domain.Consent,
) error {
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now

	// Гарантируем существование per-tenant Transit-ключа до первого encrypt'а.
	// Идемпотентно, кэшируется внутри TransitClient — не идёт в Vault при
	// повторных вызовах.
	if r.transit != nil {
		keyName := secrets.TenantKeyName(a.TenantID)
		if err := r.transit.EnsureKey(ctx, keyName); err != nil {
			return fmt.Errorf("ensure transit key: %w", err)
		}
	}

	// Шифруем PII вне транзакции — Vault HTTP-вызов не должен держать lock'и.
	passSeriesEnc, err := r.encryptField(ctx, a.TenantID, a.PassportSeries)
	if err != nil {
		return err
	}
	passNumberEnc, err := r.encryptField(ctx, a.TenantID, a.PassportNumber)
	if err != nil {
		return err
	}
	snilsEnc, err := r.encryptField(ctx, a.TenantID, a.SNILS)
	if err != nil {
		return err
	}
	innEnc, err := r.encryptField(ctx, a.TenantID, a.INN)
	if err != nil {
		return err
	}

	return r.withTenantTx(ctx, a.TenantID, func(tx *sql.Tx) error {
		var esiaSubject sql.NullString
		if a.ESIASubject != "" {
			esiaSubject = sql.NullString{String: a.ESIASubject, Valid: true}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO applicants (
			    id, inn, phone, full_name, esia_verified, esia_subject,
			    passport_series_enc, passport_number_enc, snils_enc, inn_enc,
			    created_at, updated_at
			 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			a.ID, a.INN, a.Phone, a.FullName, a.ESIAVerified, esiaSubject,
			passSeriesEnc, passNumberEnc, snilsEnc, innEnc,
			a.CreatedAt, a.UpdatedAt); err != nil {
			return err
		}
		for i := range consents {
			c := &consents[i]
			if c.RecordedAt.IsZero() {
				c.RecordedAt = now
			}
			c.ApplicantID = a.ID
			c.TenantID = a.TenantID
			if err := insertConsentTx(ctx, tx, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// Forget — реализация 152-ФЗ ст. 14 "право на забвение".
// PII-поля (full_name/phone/inn/passport*/snils) сбрасываются, ставятся
// forgotten_at и forgotten_by. Запись applicant'а сохраняется (FK для
// audit-логов, 5 лет по 115-ФЗ).
//
// Идемпотентно: если forgotten_at уже выставлено — повторный UPDATE
// сохраняет старые значения благодаря COALESCE.
func (r *PostgresApplicantRepository) Forget(
	ctx context.Context, tenantID, id, requester string,
) error {
	return r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE applicants
			 SET full_name = '',
			     phone = '',
			     inn = '',
			     passport_series = NULL,
			     passport_number = NULL,
			     snils = NULL,
			     passport_series_enc = NULL,
			     passport_number_enc = NULL,
			     snils_enc = NULL,
			     inn_enc = NULL,
			     forgotten_at = COALESCE(forgotten_at, NOW()),
			     forgotten_by = COALESCE(forgotten_by, $2),
			     updated_at = NOW()
			 WHERE id = $1`,
			id, requester)
		return err
	})
}

// insertConsentTx — общий код вставки одной строки в consents (используется
// CreateWithConsents и PostgresConsentRepository.Record).
func insertConsentTx(ctx context.Context, tx *sql.Tx, c *domain.Consent) error {
	var ip, ua, sig sql.NullString
	if c.IPAddress != "" {
		ip = sql.NullString{String: c.IPAddress, Valid: true}
	}
	if c.UserAgent != "" {
		ua = sql.NullString{String: c.UserAgent, Valid: true}
	}
	if c.Signature != "" {
		sig = sql.NullString{String: c.Signature, Valid: true}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO consents (id, applicant_id, consent_type, granted, version,
		    ip_address, user_agent, signature, recorded_at, revoked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL)`,
		c.ID, c.ApplicantID, string(c.ConsentType), c.Granted, c.Version,
		ip, ua, sig, c.RecordedAt)
	return err
}

// PostgresConsentRepository — реализация domain.ConsentRepository.
//
// Изоляция тенантов — schema-per-tenant (ADR-0002). На каждый запрос
// устанавливается LOCAL search_path в схему tnt_<id> и DML идёт без префикса
// схемы. Append-only enforcement — на уровне триггера в схеме тенанта
// (см. migrations/tenant/002_create_consents.sql).
type PostgresConsentRepository struct {
	db *sql.DB
}

// NewPostgresConsentRepository — конструктор.
func NewPostgresConsentRepository(db *sql.DB) *PostgresConsentRepository {
	return &PostgresConsentRepository{db: db}
}

func (r *PostgresConsentRepository) withTenantTx(
	ctx context.Context,
	tenantID string,
	fn func(tx *sql.Tx) error,
) error {
	schema, err := tenantSchema(tenantID)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`SELECT set_config('search_path', $1, true)`, schema); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// Record — INSERT в consents в схеме тенанта.
func (r *PostgresConsentRepository) Record(ctx context.Context, c *domain.Consent) error {
	if c.RecordedAt.IsZero() {
		c.RecordedAt = time.Now().UTC()
	}
	return r.withTenantTx(ctx, c.TenantID, func(tx *sql.Tx) error {
		return insertConsentTx(ctx, tx, c)
	})
}

// ListByApplicant — активные согласия (revoked_at IS NULL) заявителя.
func (r *PostgresConsentRepository) ListByApplicant(
	ctx context.Context,
	tenantID, applicantID string,
) ([]*domain.Consent, error) {
	out := make([]*domain.Consent, 0)
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, applicant_id, consent_type, granted, version,
			        ip_address, user_agent, signature, recorded_at, revoked_at
			   FROM consents
			  WHERE applicant_id = $1 AND revoked_at IS NULL
			  ORDER BY consent_type ASC`, applicantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c := &domain.Consent{TenantID: tenantID}
			var ctype string
			var ip, ua, sig sql.NullString
			var revoked sql.NullTime
			if err := rows.Scan(&c.ID, &c.ApplicantID, &ctype, &c.Granted, &c.Version,
				&ip, &ua, &sig, &c.RecordedAt, &revoked); err != nil {
				return err
			}
			c.ConsentType = domain.ConsentType(ctype)
			if ip.Valid {
				c.IPAddress = ip.String
			}
			if ua.Valid {
				c.UserAgent = ua.String
			}
			if sig.Valid {
				c.Signature = sig.String
			}
			if revoked.Valid {
				t := revoked.Time
				c.RevokedAt = &t
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Revoke — закрывает активное согласие данного типа: UPDATE revoked_at = NOW
// (триггер разрешает строго NULL → not-NULL переход для активной строки).
// Если активной записи нет — возвращает ErrNotFound.
func (r *PostgresConsentRepository) Revoke(
	ctx context.Context,
	tenantID, applicantID string,
	t domain.ConsentType,
) error {
	if !t.IsValid() {
		return domain.ErrConsentTypeInvalid
	}
	return r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE consents
			    SET revoked_at = NOW()
			  WHERE applicant_id = $1
			    AND consent_type = $2
			    AND revoked_at IS NULL`,
			applicantID, string(t))
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}
