package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/aibank/platform/services/document-service/internal/domain"
)

// ErrNotFound — документ с указанным id отсутствует в схеме тенанта.
var ErrNotFound = errors.New("not found")

// ErrInvalidTenant — tenant_id не прошёл whitelist-валидацию.
// Имя схемы PostgreSQL формируется из tenant_id, поэтому валидация защищает
// от SQL-инъекции через идентификатор (см. ADR-0002, раздел Безопасность).
var ErrInvalidTenant = errors.New("invalid tenant id")

// validTenantID — узкий whitelist по ADR-0002.
var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// tenantSchemaName формирует безопасное имя схемы.
func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", ErrInvalidTenant
	}
	return "tnt_" + tenantID, nil
}

// PostgresDocumentRepository реализует domain.DocumentRepository поверх pg.
//
// На каждый запрос открывается транзакция с SET LOCAL search_path,
// чтобы DML-выражения резолвились в схему тенанта без префикса.
// Соединение возвращается в pool в чистом состоянии (SET LOCAL ограничен Tx).
type PostgresDocumentRepository struct {
	db *sql.DB
}

// NewPostgresDocumentRepository собирает репозиторий поверх *sql.DB.
func NewPostgresDocumentRepository(db *sql.DB) *PostgresDocumentRepository {
	return &PostgresDocumentRepository{db: db}
}

// withTenantTx запускает функцию fn внутри транзакции с выставленным search_path.
func (r *PostgresDocumentRepository) withTenantTx(
	ctx context.Context,
	tenantID string,
	fn func(*sql.Tx) error,
) error {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// schema валидирован regex'ом — безопасно интерполировать.
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`SET LOCAL search_path TO %q, public`, schema)); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// Create вставляет документ в documents.
func (r *PostgresDocumentRepository) Create(ctx context.Context, doc *domain.Document) error {
	now := time.Now().UTC()
	if doc.UploadedAt.IsZero() {
		doc.UploadedAt = now
	}
	doc.UpdatedAt = now

	var parsed any
	if len(doc.ParsedData) > 0 {
		parsed = []byte(doc.ParsedData)
	}

	return r.withTenantTx(ctx, doc.TenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO documents (
				id, application_id, type, state, file_id, filename, mime_type,
				size_bytes, sha256, storage_path, source_type, source_actor_id,
				parsed_data, uploaded_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			doc.ID, doc.ApplicationID, doc.Type, doc.State, doc.FileID,
			doc.Filename, doc.MimeType, doc.SizeBytes, doc.SHA256,
			doc.StoragePath, doc.SourceType, doc.SourceActorID,
			parsed, doc.UploadedAt, doc.UpdatedAt,
		)
		return err
	})
}

// GetByID возвращает документ по id внутри схемы тенанта.
func (r *PostgresDocumentRepository) GetByID(
	ctx context.Context, tenantID, id string,
) (*domain.Document, error) {
	var doc domain.Document
	doc.TenantID = tenantID
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
			SELECT id, application_id, type, state, file_id, filename, mime_type,
				size_bytes, sha256, storage_path, source_type,
				COALESCE(source_actor_id, ''), parsed_data, uploaded_at, updated_at
			FROM documents WHERE id = $1`, id)

		var parsed []byte
		scanErr := row.Scan(&doc.ID, &doc.ApplicationID, &doc.Type, &doc.State,
			&doc.FileID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.SHA256,
			&doc.StoragePath, &doc.SourceType, &doc.SourceActorID,
			&parsed, &doc.UploadedAt, &doc.UpdatedAt)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return ErrNotFound
		}
		if scanErr != nil {
			return scanErr
		}
		if len(parsed) > 0 {
			doc.ParsedData = json.RawMessage(parsed)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

// ListByApplication возвращает все документы заявки в порядке загрузки.
func (r *PostgresDocumentRepository) ListByApplication(
	ctx context.Context, tenantID, applicationID string,
) ([]*domain.Document, error) {
	docs := make([]*domain.Document, 0)
	err := r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
			SELECT id, application_id, type, state, file_id, filename, mime_type,
				size_bytes, sha256, storage_path, source_type,
				COALESCE(source_actor_id, ''), parsed_data, uploaded_at, updated_at
			FROM documents WHERE application_id = $1 ORDER BY uploaded_at ASC`,
			applicationID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var doc domain.Document
			doc.TenantID = tenantID
			var parsed []byte
			if err := rows.Scan(&doc.ID, &doc.ApplicationID, &doc.Type, &doc.State,
				&doc.FileID, &doc.Filename, &doc.MimeType, &doc.SizeBytes, &doc.SHA256,
				&doc.StoragePath, &doc.SourceType, &doc.SourceActorID,
				&parsed, &doc.UploadedAt, &doc.UpdatedAt); err != nil {
				return err
			}
			if len(parsed) > 0 {
				doc.ParsedData = json.RawMessage(parsed)
			}
			docs = append(docs, &doc)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// MarkParsed выставляет state=parsed и записывает результат парсинга.
func (r *PostgresDocumentRepository) MarkParsed(
	ctx context.Context, tenantID, id string, parsed json.RawMessage,
) error {
	now := time.Now().UTC()
	return r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE documents SET state=$2, parsed_data=$3, updated_at=$4 WHERE id=$1`,
			id, domain.DocumentStateParsed, []byte(parsed), now)
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

// MarkRejected выставляет state=rejected и пишет причину в parsed_data.reason.
func (r *PostgresDocumentRepository) MarkRejected(
	ctx context.Context, tenantID, id, reason string,
) error {
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	return r.withTenantTx(ctx, tenantID, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE documents SET state=$2, parsed_data=$3, updated_at=$4 WHERE id=$1`,
			id, domain.DocumentStateRejected, payload, now)
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
