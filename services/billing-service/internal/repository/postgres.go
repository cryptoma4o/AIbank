package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"aibank/billing-service/internal/domain"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type BillingRepository struct {
	db *sql.DB
}

func NewBillingRepository(db *sql.DB) *BillingRepository {
	return &BillingRepository{db: db}
}

func (r *BillingRepository) RecordEvent(ctx context.Context, e *domain.BillableEvent) error {
	e.ID = "bil_" + uuid.New().String()
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	if e.PriceKopecks == 0 {
		e.PriceKopecks = domain.DefaultPriceKopecks[e.Kind]
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO billable_events (id, tenant_id, kind, resource_id, price_kopecks, occurred_at)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ID, e.TenantID, e.Kind, e.ResourceID, e.PriceKopecks, e.OccurredAt,
	)
	return err
}

func (r *BillingRepository) GetUsageReport(ctx context.Context, tenantID string, from, to time.Time) (*domain.UsageReport, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT kind, COUNT(*), SUM(price_kopecks)
         FROM billable_events
         WHERE tenant_id = $1 AND occurred_at >= $2 AND occurred_at < $3
         GROUP BY kind`,
		tenantID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("billing: query report: %w", err)
	}
	defer rows.Close()

	report := &domain.UsageReport{
		TenantID:    tenantID,
		PeriodStart: from,
		PeriodEnd:   to,
		ByKind:      make(map[domain.EventKind]int64),
	}
	for rows.Next() {
		var kind domain.EventKind
		var count int
		var sum int64
		if err := rows.Scan(&kind, &count, &sum); err != nil {
			return nil, err
		}
		report.ByKind[kind] = sum
		report.TotalKopecks += sum
		report.EventCount += count
	}
	return report, rows.Err()
}
