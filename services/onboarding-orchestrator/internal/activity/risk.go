package activity

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	auditsdk "github.com/aibank/platform/packages/audit-sdk"
	"go.temporal.io/sdk/temporal"

	wf "aibank/onboarding-orchestrator/internal/workflow"
)

// riskAssessmentDTO mirrors the risk-engine response from
// POST /v1/assessments (services/risk-engine/internal/domain.RiskAssessment).
// We only decode the fields that drive workflow decisions.
type riskAssessmentDTO struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	ApplicationID  string            `json:"application_id"`
	Score          float64           `json:"score"`
	Category       string            `json:"category"`
	RulesTriggered []ruleTriggeredDTO `json:"rules_triggered"`
	Recommendation string            `json:"recommendation"`
	CreatedAt      time.Time         `json:"created_at"`
}

type ruleTriggeredDTO struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// RiskActivities is the Temporal activity holder for risk-engine.
type RiskActivities struct {
	http  *httpDoer
	audit *auditsdk.Client
	log   *slog.Logger
}

// RiskConfig wires the activity at server boot.
type RiskConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewRiskActivities returns a ready-to-register RiskActivities.
// audit may be nil — audit emission becomes a no-op.
func NewRiskActivities(cfg RiskConfig, audit *auditsdk.Client, log *slog.Logger) *RiskActivities {
	if log == nil {
		log = slog.Default()
	}
	return &RiskActivities{
		http:  newHTTPDoer(cfg.BaseURL, cfg.Timeout),
		audit: audit,
		log:   log,
	}
}

// RunRiskAssessment is the real implementation behind workflow.ActivityAssessRisk.
//
// HTTP contract: POST {base}/v1/assessments
//
//	{
//	  "tenant_id":      "...",
//	  "application_id": "...",
//	  "input_data":     {}    // free-form; orchestrator passes empty —
//	                          // risk-engine derives features from its
//	                          // own tenant-scoped data (see ADR-0015).
//	}
//
//   - 201 + body → success; mapped into RiskAssessmentResult.
//   - 4xx (other) → non-retryable; workflow will decline.
//   - 5xx/429/network → returned as-is; Temporal retries via workflow RetryPolicy.
//
// Returns a Temporal NON-retryable application error on 4xx so the
// workflow's MaximumAttempts is not wasted on permanent failures.
func (a *RiskActivities) RunRiskAssessment(ctx context.Context, tenantID, applicationID string) (*wf.RiskAssessmentResult, error) {
	log := a.log.With(
		"activity", wf.ActivityAssessRisk,
		"tenant_id", tenantID,
		"application_id", applicationID,
	)
	log = withActivityContext(ctx, log)

	if tenantID == "" || applicationID == "" {
		return nil, temporal.NewNonRetryableApplicationError(
			"tenant_id and application_id are required",
			"InvalidInput", nil)
	}

	reqBody := map[string]any{
		"tenant_id":      tenantID,
		"application_id": applicationID,
		"input_data":     map[string]any{},
	}

	var dto riskAssessmentDTO
	err := a.http.doJSON(ctx, http.MethodPost, "/v1/assessments", reqBody, &dto)
	if err != nil {
		if s, ok := asStatusError(err); ok {
			if s.IsRetryable() {
				log.Warn("risk_engine_transient", "status", s.StatusCode)
				return nil, err
			}
			log.Error("risk_engine_4xx", "status", s.StatusCode, "body", truncate(s.Body, 200))
			a.emitAudit(ctx, tenantID, applicationID, "risk.assess.failed", map[string]any{
				"http_status": s.StatusCode,
			})
			return nil, temporal.NewNonRetryableApplicationError(
				s.Error(), "RiskEngine4xx", err)
		}
		// Network / decode / context errors — treat as transient.
		log.Warn("risk_engine_io", "err", err)
		return nil, err
	}

	hasBlocking := false
	blockingReason := ""
	for _, rt := range dto.RulesTriggered {
		if rt.Severity == "BLOCKING" {
			hasBlocking = true
			if blockingReason == "" {
				blockingReason = rt.RuleID
				if rt.Description != "" {
					blockingReason = rt.RuleID + ": " + rt.Description
				}
			}
		}
	}

	result := &wf.RiskAssessmentResult{
		AssessmentID:    dto.ID,
		Score:           dto.Score,
		Category:        dto.Category,
		HasBlockingRule: hasBlocking,
		BlockingReason:  blockingReason,
		Recommendation:  dto.Recommendation,
		ComputedAt:      time.Now().UTC(),
	}

	a.emitAudit(ctx, tenantID, applicationID, "risk.assess.completed", map[string]any{
		"assessment_id":    dto.ID,
		"score":            dto.Score,
		"category":         dto.Category,
		"recommendation":   dto.Recommendation,
		"has_blocking":     hasBlocking,
		"blocking_reason":  blockingReason,
	})

	log.Info("risk_assess_done",
		"assessment_id", dto.ID,
		"score", dto.Score,
		"recommendation", dto.Recommendation,
		"has_blocking", hasBlocking,
	)
	return result, nil
}

func (a *RiskActivities) emitAudit(ctx context.Context, tenantID, applicationID, eventType string, payload map[string]any) {
	if a.audit == nil || tenantID == "" {
		return
	}
	body, mErr := json.Marshal(payload)
	if mErr != nil {
		a.log.Warn("audit payload marshal failed", "err", mErr)
		return
	}
	if _, err := a.audit.Append(ctx, auditsdk.RecordEventRequest{
		TenantID:   tenantID,
		EntityType: "application",
		EntityID:   applicationID,
		EventType:  eventType,
		ActorID:    "onboarding-orchestrator",
		ActorType:  auditsdk.ActorTypeSystem,
		Payload:    body,
	}); err != nil {
		var apiErr *auditsdk.APIError
		if errors.As(err, &apiErr) {
			a.log.Warn("audit emit failed", "code", apiErr.Code, "status", apiErr.StatusCode)
		} else {
			a.log.Warn("audit emit failed", "err", err)
		}
	}
}
