package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"aibank/onboarding-orchestrator/internal/domain"
)

type PostgresUBOGraphRepository struct {
	db *sql.DB
}

func NewPostgresUBOGraphRepository(db *sql.DB) *PostgresUBOGraphRepository {
	return &PostgresUBOGraphRepository{db: db}
}

func (r *PostgresUBOGraphRepository) withTenantTx(ctx context.Context, tenantID string) (*sql.Tx, error) {
	schema, err := tenantSchemaName(tenantID)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	stmt := fmt.Sprintf("SET LOCAL search_path TO %s, public", pq.QuoteIdentifier(schema))
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("set search_path: %w", err)
	}
	return tx, nil
}

func (r *PostgresUBOGraphRepository) Upsert(ctx context.Context, g *domain.UBOGraph) error {
	tx, err := r.withTenantTx(ctx, g.TenantID)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now

	nodesJSON, _ := json.Marshal(orEmpty(g.Nodes))
	edgesJSON, _ := json.Marshal(orEmpty(g.Edges))
	chainsJSON, _ := json.Marshal(orEmpty(g.OwnershipChains))

	var computedAt sql.NullTime
	if g.ComputedAt != nil {
		computedAt = sql.NullTime{Time: *g.ComputedAt, Valid: true}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ubo_graphs (
			id, tenant_id, application_id, legal_entity_id,
			nodes, edges, ownership_chains,
			no_ubo_reason, eio_as_ubo_confirmation, diagram_doc_id,
			computed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,NULLIF($10,''),$11,$12,$13)
		ON CONFLICT (application_id) DO UPDATE SET
			legal_entity_id          = EXCLUDED.legal_entity_id,
			nodes                    = EXCLUDED.nodes,
			edges                    = EXCLUDED.edges,
			ownership_chains         = EXCLUDED.ownership_chains,
			no_ubo_reason            = EXCLUDED.no_ubo_reason,
			eio_as_ubo_confirmation  = EXCLUDED.eio_as_ubo_confirmation,
			diagram_doc_id           = EXCLUDED.diagram_doc_id,
			computed_at              = EXCLUDED.computed_at,
			updated_at               = EXCLUDED.updated_at`,
		g.ID, g.TenantID, g.ApplicationID, g.LegalEntityID,
		nodesJSON, edgesJSON, chainsJSON,
		g.NoUBOReason, g.EIOAsUBOConfirmation, g.DiagramDocID,
		computedAt, g.CreatedAt, g.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert ubo_graph: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresUBOGraphRepository) GetByApplication(ctx context.Context, tenantID, applicationID string) (*domain.UBOGraph, error) {
	tx, err := r.withTenantTx(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, application_id, legal_entity_id,
		       nodes, edges, ownership_chains,
		       COALESCE(no_ubo_reason,''), eio_as_ubo_confirmation, COALESCE(diagram_doc_id,''),
		       computed_at, created_at, updated_at
		FROM ubo_graphs
		WHERE application_id = $1`, applicationID)

	var (
		g          domain.UBOGraph
		nodesJSON  []byte
		edgesJSON  []byte
		chainsJSON []byte
		computedAt sql.NullTime
	)
	if err := row.Scan(
		&g.ID, &g.TenantID, &g.ApplicationID, &g.LegalEntityID,
		&nodesJSON, &edgesJSON, &chainsJSON,
		&g.NoUBOReason, &g.EIOAsUBOConfirmation, &g.DiagramDocID,
		&computedAt, &g.CreatedAt, &g.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan ubo_graph: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if computedAt.Valid {
		t := computedAt.Time
		g.ComputedAt = &t
	}
	if len(nodesJSON) > 0 {
		if err := json.Unmarshal(nodesJSON, &g.Nodes); err != nil {
			return nil, fmt.Errorf("decode nodes: %w", err)
		}
	}
	if len(edgesJSON) > 0 {
		if err := json.Unmarshal(edgesJSON, &g.Edges); err != nil {
			return nil, fmt.Errorf("decode edges: %w", err)
		}
	}
	if len(chainsJSON) > 0 {
		if err := json.Unmarshal(chainsJSON, &g.OwnershipChains); err != nil {
			return nil, fmt.Errorf("decode ownership_chains: %w", err)
		}
	}
	return &g, nil
}
