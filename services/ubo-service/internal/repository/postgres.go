package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aibank/ubo-service/internal/domain"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type UBORepository struct {
	db *sql.DB
}

func NewUBORepository(db *sql.DB) *UBORepository {
	return &UBORepository{db: db}
}

func (r *UBORepository) UpsertNode(ctx context.Context, n *domain.UBONode) error {
	if n.ID == "" {
		n.ID = "ubn_" + uuid.New().String()
		n.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ubo_nodes (id, tenant_id, app_id, node_type, name, inn, passport, stake, is_ubo, created_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
         ON CONFLICT (id) DO UPDATE SET name=$5, inn=$6, stake=$8, is_ubo=$9`,
		n.ID, n.TenantID, n.AppID, n.NodeType, n.Name, n.INN, n.Passport, n.Stake, n.IsUBO, n.CreatedAt,
	)
	return err
}

func (r *UBORepository) AddEdge(ctx context.Context, e *domain.UBOEdge) error {
	e.ID = "ube_" + uuid.New().String()
	e.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ubo_edges (id, tenant_id, app_id, from_node_id, to_node_id, direct_stake, created_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.ID, e.TenantID, e.AppID, e.FromNodeID, e.ToNodeID, e.DirectStake, e.CreatedAt,
	)
	return err
}

// GetGraph loads the full graph for an application.
func (r *UBORepository) GetGraph(ctx context.Context, tenantID, appID string) (*domain.UBOGraph, error) {
	graph := &domain.UBOGraph{AppID: appID, TenantID: tenantID, CreatedAt: time.Now().UTC()}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, app_id, node_type, name, inn, COALESCE(passport,''), stake, is_ubo, created_at
         FROM ubo_nodes WHERE app_id=$1 AND tenant_id=$2`, appID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ubo repo: nodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n domain.UBONode
		if err := rows.Scan(&n.ID, &n.TenantID, &n.AppID, &n.NodeType, &n.Name, &n.INN, &n.Passport, &n.Stake, &n.IsUBO, &n.CreatedAt); err != nil {
			return nil, err
		}
		graph.Nodes = append(graph.Nodes, n)
		if n.IsUBO {
			graph.UBOs = append(graph.UBOs, n)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	erows, err := r.db.QueryContext(ctx,
		`SELECT id, tenant_id, app_id, from_node_id, to_node_id, direct_stake, created_at
         FROM ubo_edges WHERE app_id=$1 AND tenant_id=$2`, appID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ubo repo: edges: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var e domain.UBOEdge
		if err := erows.Scan(&e.ID, &e.TenantID, &e.AppID, &e.FromNodeID, &e.ToNodeID, &e.DirectStake, &e.CreatedAt); err != nil {
			return nil, err
		}
		graph.Edges = append(graph.Edges, e)
	}
	return graph, erows.Err()
}

// ComputeEffectiveStakes uses a recursive CTE to compute indirect ownership.
// Returns a map of nodeID → effective stake % for a given root node (the target company).
func (r *UBORepository) ComputeEffectiveStakes(ctx context.Context, tenantID, appID, rootNodeID string) (map[string]float64, error) {
	rows, err := r.db.QueryContext(ctx, `
        WITH RECURSIVE ownership AS (
            SELECT from_node_id, to_node_id, direct_stake AS effective_stake
            FROM ubo_edges
            WHERE to_node_id = $3 AND app_id = $1 AND tenant_id = $2
            UNION ALL
            SELECT e.from_node_id, e.to_node_id, o.effective_stake * e.direct_stake / 100.0
            FROM ubo_edges e
            JOIN ownership o ON e.to_node_id = o.from_node_id
            WHERE e.app_id = $1 AND e.tenant_id = $2
        )
        SELECT from_node_id, SUM(effective_stake) AS total_stake
        FROM ownership
        GROUP BY from_node_id
    `, appID, tenantID, rootNodeID)
	if err != nil {
		return nil, fmt.Errorf("ubo repo: compute stakes: %w", err)
	}
	defer rows.Close()
	result := make(map[string]float64)
	for rows.Next() {
		var nodeID string
		var stake float64
		if err := rows.Scan(&nodeID, &stake); err != nil {
			return nil, err
		}
		result[nodeID] = stake
	}
	return result, rows.Err()
}
