// Package graph — резолвер complianceDashboard и связанные интерфейсы.
//
// Вынесено отдельно от resolver.go, чтобы:
//  1. удержать resolver.go под лимитом 500 строк;
//  2. изолировать узкие интерфейсы upstream-клиентов для unit-тестов
//     (см. resolver_test.go: TestComplianceDashboard_*).
//
// Дизайн агрегации:
//   - 4 параллельных запроса в upstream (apps / ubo / audit / forgotten)
//     через sync.WaitGroup; каждый коллектор пишет в свою ячейку без
//     общих мутируемых структур (race-free).
//   - Частичные сбои не валят весь дашборд: фейл одного коллектора
//     логируется и соответствующее поле остаётся zero-значением.
//   - Математика (automation_rate, avg time-to-decision) считается
//     в резолвере по сырым apps[] — не доверяем upstream-агрегациям.
package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/graphql-go/graphql"

	"aibank/bff-admin/internal/auth"
	"aibank/bff-admin/internal/clients"
)

// ── Узкие интерфейсы upstream-клиентов ───────────────────────────────
//
// Объявлены в графе (а не в clients/), чтобы:
//   - удовлетворить Dependency Inversion: владелец абстракции — потребитель;
//   - дать тестам возможность подменять только нужные методы без полного
//     mock-а HTTP-клиента.

// applicationsLister — ListByDateRange + Get; реализуется *clients.OrchestratorClient.
type applicationsLister interface {
	ListByDateRange(ctx context.Context, tenantID string, from, to time.Time) ([]clients.Application, error)
}

// uboCounter — count UBO-screenings с отметкой match.  Soft endpoint:
// если ubo-graph не реализован — возвращает (0, 0, nil) без ошибки.
type uboCounter interface {
	CountScreenings(ctx context.Context, tenantID string, from, to time.Time) (total int, withMatch int, err error)
}

// auditCounter — count audit-событий за период.  Используется как
// sanity-check (за сегодня) и для forgotten-метрики.
type auditCounter interface {
	CountEvents(ctx context.Context, tenantID string, from, to time.Time, action string) (int, error)
}

// ── Реализация по умолчанию для production ────────────────────────────

// orchestratorAdapter — *clients.OrchestratorClient уже соответствует.
// Этот тип нужен только чтобы методы можно было удобно собрать в Resolver.

// uboHTTPClient — простой клиент GET /v1/ubo-graphs?tenant_id=…&from=…&to=…
// При HTTP 404 (endpoint не задеплоен) — graceful 0,0.
type uboHTTPClient struct {
	baseURL string
}

// NewUBOClient — для cmd/server (если потребуется отдельный URL).
func NewUBOClient(baseURL string) *uboHTTPClient { //nolint:revive // factory for main.go
	return &uboHTTPClient{baseURL: baseURL}
}

// CountScreenings — count UBO graphs.  Если эндпоинт отсутствует — (0,0,nil).
func (c *uboHTTPClient) CountScreenings(ctx context.Context, tenantID string, from, to time.Time) (int, int, error) {
	if c == nil || c.baseURL == "" {
		return 0, 0, nil
	}
	q := url.Values{}
	q.Set("tenant_id", tenantID)
	if !from.IsZero() {
		q.Set("from", from.UTC().Format(time.RFC3339))
	}
	if !to.IsZero() {
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	u := fmt.Sprintf("%s/v1/ubo-graphs?%s", c.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return 0, 0, nil // soft: endpoint not deployed yet
	}
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("ubo: status %d", resp.StatusCode)
	}
	// Пытаемся достать total и matched из заголовков (cheap).
	total := parseHeaderInt(resp.Header.Get("X-Total-Count"))
	withMatch := parseHeaderInt(resp.Header.Get("X-Matched-Count"))
	return total, withMatch, nil
}

func parseHeaderInt(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// auditAdapter — обёртка вокруг *clients.AuditClient, реализующая auditCounter.
type auditAdapter struct{ inner *clients.AuditClient }

// NewAuditCounter — для cmd/server.  inner может быть nil → (0, nil).
func NewAuditCounter(inner *clients.AuditClient) auditCounter { return &auditAdapter{inner: inner} }

func (a *auditAdapter) CountEvents(ctx context.Context, tenantID string, from, to time.Time, action string) (int, error) {
	if a == nil || a.inner == nil {
		return 0, nil
	}
	// audit-service сегодня без count-эндпоинта → fetch + len.
	// TODO upstream: HEAD /v1/audit-events с X-Total-Count.
	f := clients.AuditFilter{Action: action, Limit: 1000}
	events, err := a.inner.List(ctx, tenantID, f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range events {
		if !from.IsZero() && e.OccurredAt.Before(from) {
			continue
		}
		if !to.IsZero() && e.OccurredAt.After(to) {
			continue
		}
		n++
	}
	return n, nil
}

// ── Конфигурация резолвера ────────────────────────────────────────────

// Dashboard — совокупность зависимостей для complianceDashboard.
// Заполняется в NewResolver / тестах.
type Dashboard struct {
	Apps    applicationsLister
	UBO     uboCounter
	Audit   auditCounter
	Logger  *slog.Logger
}

// ── Resolver ──────────────────────────────────────────────────────────

// ErrCrossTenantForbidden — auth.tenant ≠ запрошенный tenantId, и роль не
// platform.admin.
var ErrCrossTenantForbidden = errors.New("forbidden: cross-tenant access denied")

// ErrComplianceRoleRequired — роль ниже bank.compliance_officer.
var ErrComplianceRoleRequired = errors.New("forbidden: bank.compliance_officer or above required")

// resolveComplianceDashboard — основной резолвер.
//
// 1) Проверяет auth: роль ≥ bank.compliance_officer и tenant=auth.tenant
//    (или platform.admin override).
// 2) Параллельно собирает метрики через WaitGroup.
// 3) Вычисляет производные KPI (automation_rate, avg time-to-decision).
func (r *Resolver) resolveComplianceDashboard(p graphql.ResolveParams) (interface{}, error) {
	ac, err := authFrom(p.Context)
	if err != nil {
		return nil, err
	}
	// Любая admin-роль уже прошла middleware.  Здесь — узкая проверка:
	// bank.operator не должен попадать в дашборд (хотя middleware его
	// и не пускает в bff-admin сейчас, мы оставляем guard на будущее).
	if !isComplianceCapable(ac.Role) {
		return nil, ErrComplianceRoleRequired
	}

	tenantArg, _ := p.Args["tenantId"].(string)
	if tenantArg == "" {
		return nil, fmt.Errorf("tenantId is required")
	}
	if tenantArg != ac.TenantID && !auth.IsPlatformAdmin(ac.Role) {
		return nil, ErrCrossTenantForbidden
	}

	from, to := parseDateRange(p.Args)

	if r.dashboard == nil {
		return nil, fmt.Errorf("dashboard dependencies not configured")
	}
	logger := r.dashboard.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Параллельный сбор: 3 коллектора пишут в свои ячейки, чтобы
	// не было гонки.
	var (
		wg                                       sync.WaitGroup
		apps                                     []clients.Application
		appsErr                                  error
		uboTotal, uboMatched                     int
		uboErr                                   error
		auditToday                               int
		auditErr                                 error
	)

	// Используем тайм-аут 8 секунд: medium-latency upstream + 1 retry в transport.
	ctx, cancel := context.WithTimeout(p.Context, 8*time.Second)
	defer cancel()

	wg.Add(3)
	go func() {
		defer wg.Done()
		if r.dashboard.Apps == nil {
			return
		}
		apps, appsErr = r.dashboard.Apps.ListByDateRange(ctx, tenantArg, from, to)
	}()
	go func() {
		defer wg.Done()
		if r.dashboard.UBO == nil {
			return
		}
		uboTotal, uboMatched, uboErr = r.dashboard.UBO.CountScreenings(ctx, tenantArg, from, to)
	}()
	go func() {
		defer wg.Done()
		if r.dashboard.Audit == nil {
			return
		}
		// Sanity check audit chain: события за сегодня (UTC).
		startOfDay := time.Now().UTC().Truncate(24 * time.Hour)
		auditToday, auditErr = r.dashboard.Audit.CountEvents(ctx, tenantArg, startOfDay, time.Now().UTC(), "")
	}()
	wg.Wait()

	// Частичные сбои: лог + zeroed поле, дашборд возвращается всё равно.
	if appsErr != nil {
		logger.Warn("complianceDashboard: orchestrator failed", "tenant", tenantArg, "err", appsErr)
		apps = nil
	}
	if uboErr != nil {
		logger.Warn("complianceDashboard: ubo failed", "tenant", tenantArg, "err", uboErr)
		uboTotal, uboMatched = 0, 0
	}
	if auditErr != nil {
		logger.Warn("complianceDashboard: audit failed", "tenant", tenantArg, "err", auditErr)
		auditToday = 0
	}

	// TODO: forgotten_applicants_count — identity-service пока не
	// экспортирует count удалённых субъектов (152-ФЗ § 14).  Возвращаем 0
	// до появления GET /v1/me/forgotten-count в identity-service.
	forgotten := 0

	agg := aggregate(apps)

	return map[string]interface{}{
		"tenantId":                 tenantArg,
		"from":                     from,
		"to":                       to,
		"applicationsTotal":        agg.total,
		"applicationsByState":      agg.byState,
		"decisionsAutoApproved":    agg.autoApproved,
		"decisionsManualReview":    agg.manualReview,
		"decisionsDeclined":        agg.declined,
		"decisionsWithEDD":         agg.withEDD,
		"automationRate":           agg.automationRate(),
		"averageTimeToDecisionSec": agg.avgTimeToDecisionSec(),
		"uboScreeningsTotal":       uboTotal,
		"screeningsWithMatch":      uboMatched,
		"auditEventsToday":         auditToday,
		"forgottenApplicantsCount": forgotten,
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────

func isComplianceCapable(role string) bool {
	return role == auth.RoleBankComplianceOfficer ||
		role == auth.RoleBankAdmin ||
		role == auth.RolePlatformAdmin
}

// parseDateRange — извлекает from/to из аргументов.  Default: [00:00 UTC, now].
func parseDateRange(args map[string]interface{}) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.Truncate(24 * time.Hour)
	to := now
	if v, ok := args["from"].(time.Time); ok && !v.IsZero() {
		from = v
	}
	if v, ok := args["to"].(time.Time); ok && !v.IsZero() {
		to = v
	}
	return from, to
}

// appsAggregate — промежуточный результат подсчёта.
type appsAggregate struct {
	total           int
	byState         map[string]int
	autoApproved    int
	manualReview    int
	declined        int
	withEDD         int
	totalDecisionMS int64 // для avg
	decisionsCount  int
}

func (a *appsAggregate) automationRate() float64 {
	if a.total == 0 {
		return 0
	}
	return float64(a.autoApproved) / float64(a.total)
}

func (a *appsAggregate) avgTimeToDecisionSec() float64 {
	if a.decisionsCount == 0 {
		return 0
	}
	return float64(a.totalDecisionMS) / float64(a.decisionsCount) / 1000.0
}

// aggregate — чистая функция: out зависит только от apps.
//
// Маппинг state→bucket (зеркалит ApplicationState enum):
//   - auto_approved             → autoApproved
//   - manual_review             → manualReview
//   - declined                  → declined
//   - approved_with_edd         → withEDD (и одновременно засчитан как approved)
//
// Время до решения: updated_at - created_at для terminal states (auto_approved,
// approved, approved_with_edd, declined, account_opened).  Это допущение
// без отдельной decision_at колонки в Application; точная метрика — после
// добавления Decision.DecidedAt в payload (TODO).
func aggregate(apps []clients.Application) appsAggregate {
	a := appsAggregate{byState: map[string]int{}}
	for _, app := range apps {
		a.total++
		a.byState[app.State]++
		switch app.State {
		case "auto_approved":
			a.autoApproved++
		case "manual_review":
			a.manualReview++
		case "declined":
			a.declined++
		case "approved_with_edd":
			a.withEDD++
		}
		if isTerminal(app.State) && !app.UpdatedAt.IsZero() && !app.CreatedAt.IsZero() {
			delta := app.UpdatedAt.Sub(app.CreatedAt)
			if delta > 0 {
				a.totalDecisionMS += delta.Milliseconds()
				a.decisionsCount++
			}
		}
	}
	return a
}

func isTerminal(state string) bool {
	switch state {
	case "auto_approved", "approved", "approved_with_edd", "declined", "account_opened":
		return true
	}
	return false
}
