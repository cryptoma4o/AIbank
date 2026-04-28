// Package repository — PostgreSQL-реализация UBOGraphRepository.
//
// Изоляция тенантов — schema-per-tenant (ADR-0002). search_path выставляется
// LOCAL'но в транзакции, имя схемы валидируется regex'ом, идентичным
// tenant-service.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"aibank/ubo-service/internal/domain"
)

// ErrNotFound — единая sentinel-ошибка из репозитория.
var ErrNotFound = errors.New("not found")

var validTenantID = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func tenantSchemaName(tenantID string) (string, error) {
	if !validTenantID.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id %q", tenantID)
	}
	return "tnt_" + tenantID, nil
}

func withTenantTx(ctx context.Context, db *sql.DB, tenantID string, fn func(*sql.Tx) error) error {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`SET LOCAL search_path TO %q, public`, schema)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// PostgresUBOGraphRepository реализует domain.UBOGraphRepository.
type PostgresUBOGraphRepository struct {
	db *sql.DB
}

// NewPostgresUBOGraphRepository — конструктор.
func NewPostgresUBOGraphRepository(db *sql.DB) *PostgresUBOGraphRepository {
	return &PostgresUBOGraphRepository{db: db}
}

// Create вычисляет version = MAX(version)+1 для пары (legal_entity_id) внутри
// схемы тенанта и вставляет новую строку в одной транзакции. Сериализуемая
// версия через UNIQUE-индекс защитит от двух одновременных вставок.
func (r *PostgresUBOGraphRepository) Create(ctx context.Context, g *domain.UBOGraph) error {
	if g.ComputedAt.IsZero() {
		g.ComputedAt = time.Now().UTC()
	}
	return withTenantTx(ctx, r.db, g.TenantID, func(tx *sql.Tx) error {
		var maxVer sql.NullInt64
		if err := tx.QueryRowContext(ctx,
			`SELECT MAX(version) FROM ubo_graphs WHERE legal_entity_id = $1`,
			g.LegalEntityID).Scan(&maxVer); err != nil {
			return err
		}
		g.Version = int(maxVer.Int64) + 1
		_, err := tx.ExecContext(ctx,
			`INSERT INTO ubo_graphs
			   (id, legal_entity_id, version, nodes, edges, ubos,
			    confidence, unresolved_branches, computed_at, computed_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			g.ID, g.LegalEntityID, g.Version,
			[]byte(g.Nodes), []byte(g.Edges), []byte(g.UBOs),
			g.Confidence, []byte(g.UnresolvedBranches),
			g.ComputedAt, g.ComputedBy)
		return err
	})
}

// GetByID — точечный запрос по PK; tenant_id используется только для
// выбора схемы (search_path), в самой таблице его нет.
func (r *PostgresUBOGraphRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.UBOGraph, error) {
	g := &domain.UBOGraph{TenantID: tenantID}
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT id, legal_entity_id, version, nodes, edges, ubos,
			        confidence, unresolved_branches, computed_at, computed_by
			 FROM ubo_graphs WHERE id = $1`, id)
		return scanGraph(row, g)
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

// GetLatestByLegalEntity возвращает запись с наибольшим version.
// Использует idx_ubo_graphs_le_ver для O(log n) лукапа.
func (r *PostgresUBOGraphRepository) GetLatestByLegalEntity(ctx context.Context, tenantID, legalEntityID string) (*domain.UBOGraph, error) {
	g := &domain.UBOGraph{TenantID: tenantID}
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx,
			`SELECT id, legal_entity_id, version, nodes, edges, ubos,
			        confidence, unresolved_branches, computed_at, computed_by
			 FROM ubo_graphs WHERE legal_entity_id = $1
			 ORDER BY version DESC LIMIT 1`, legalEntityID)
		return scanGraph(row, g)
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

// ListByLegalEntity — история версий по убыванию.
func (r *PostgresUBOGraphRepository) ListByLegalEntity(ctx context.Context, tenantID, legalEntityID string, limit, offset int) ([]*domain.UBOGraph, error) {
	out := make([]*domain.UBOGraph, 0)
	err := withTenantTx(ctx, r.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, legal_entity_id, version, nodes, edges, ubos,
			        confidence, unresolved_branches, computed_at, computed_by
			 FROM ubo_graphs WHERE legal_entity_id = $1
			 ORDER BY version DESC LIMIT $2 OFFSET $3`,
			legalEntityID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			g := &domain.UBOGraph{TenantID: tenantID}
			var nodes, edges, ubos, unresolved []byte
			if err := rows.Scan(&g.ID, &g.LegalEntityID, &g.Version,
				&nodes, &edges, &ubos,
				&g.Confidence, &unresolved,
				&g.ComputedAt, &g.ComputedBy); err != nil {
				return err
			}
			g.Nodes = nodes
			g.Edges = edges
			g.UBOs = ubos
			g.UnresolvedBranches = unresolved
			out = append(out, g)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanGraph — общая логика чтения единичного графа.
type singleScanner interface {
	Scan(dest ...any) error
}

func scanGraph(row singleScanner, g *domain.UBOGraph) error {
	var nodes, edges, ubos, unresolved []byte
	err := row.Scan(&g.ID, &g.LegalEntityID, &g.Version,
		&nodes, &edges, &ubos,
		&g.Confidence, &unresolved,
		&g.ComputedAt, &g.ComputedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	g.Nodes = nodes
	g.Edges = edges
	g.UBOs = ubos
	g.UnresolvedBranches = unresolved
	return nil
}
