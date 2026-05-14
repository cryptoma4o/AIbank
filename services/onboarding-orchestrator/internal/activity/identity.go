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

// identityApplicant mirrors the identity-service Applicant DTO
// (packages/openapi/identity-service.yaml#components.schemas.Applicant).
// Only the fields we actually consume are decoded; unknown fields are ignored.
type identityApplicant struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	INN           string `json:"inn"`
	Phone         string `json:"phone"`
	FullName      string `json:"full_name"`
	ESIAVerified  bool   `json:"esia_verified"`
	ESIASubject   string `json:"esia_subject,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// IdentityActivities is the Temporal activity holder for identity-service.
//
// Conforms to internal/workflow.IdentityActivity (the workflow uses the string
// name ActivityVerifyIdentity to dispatch, so the concrete method signature
// MUST stay compatible with what workflow.OnboardingWorkflow passes:
// `(ctx, tenantID, applicantID)`).
type IdentityActivities struct {
	http  *httpDoer
	audit *auditsdk.Client
	log   *slog.Logger
}

// IdentityConfig wires the activity at server boot.
type IdentityConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewIdentityActivities returns a ready-to-register IdentityActivities.
// audit may be nil — audit calls become no-ops in that case (test setups).
func NewIdentityActivities(cfg IdentityConfig, audit *auditsdk.Client, log *slog.Logger) *IdentityActivities {
	if log == nil {
		log = slog.Default()
	}
	return &IdentityActivities{
		http:  newHTTPDoer(cfg.BaseURL, cfg.Timeout),
		audit: audit,
		log:   log,
	}
}

// VerifyIdentity is the real implementation behind workflow.ActivityVerifyIdentity.
//
// State-machine contract (see workflow.OnboardingWorkflow §1):
//   - Verified=true   → workflow advances draft → identifying → collecting_documents.
//   - Verified=false  → workflow ends in StateDeclined with Reason filled.
//   - error           → Temporal retries (per RetryPolicy MaximumAttempts=3);
//                       on final failure workflow ends in StateDeclined.
//
// HTTP contract: GET {base}/v1/applicants/{id}?tenant_id={tid}
//   - 200 → Applicant payload; we treat esia_verified as the canonical signal.
//   - 404 → applicant unknown; Verified=false, Reason="applicant_not_found".
//           NON-retryable: we explicitly return a Temporal application error
//           with non-retryable type so workflow gets the negative result fast.
//   - 5xx/429/network → returned as-is; Temporal retries.
//
// IMPORTANT contract gap (2026-05-12): the published OpenAPI
// (packages/openapi/identity-service.yaml) does NOT yet document GET
// /v1/applicants/{id}. The endpoint is the natural shape (mirrors the repo
// method PostgresApplicantRepository.GetByID) and is required by the
// orchestrator state machine. Until identity-service ships it, the activity
// will fall back to 404 → declined, which is safe but pessimistic.
func (a *IdentityActivities) VerifyIdentity(ctx context.Context, tenantID, applicantID string) (*wf.IdentityVerificationResult, error) {
	log := a.log.With(
		"activity", wf.ActivityVerifyIdentity,
		"tenant_id", tenantID,
		"applicant_id", applicantID,
	)
	log = withActivityContext(ctx, log)

	if tenantID == "" || applicantID == "" {
		log.Warn("verify_identity called with empty ids — workflow input invariant violated")
		return nil, temporal.NewNonRetryableApplicationError(
			"tenant_id and applicant_id are required",
			"InvalidInput", nil)
	}

	path := "/v1/applicants/" + applicantID + "?tenant_id=" + tenantID
	var dto identityApplicant
	err := a.http.doJSON(ctx, http.MethodGet, path, nil, &dto)

	if err != nil {
		if s, ok := asStatusError(err); ok && s.StatusCode == http.StatusNotFound {
			log.Info("applicant_not_found", "status", s.StatusCode)
			result := &wf.IdentityVerificationResult{
				Verified:  false,
				Method:    "ESIA",
				CheckedAt: time.Now().UTC(),
				Reason:    "applicant_not_found",
			}
			a.emitAudit(ctx, tenantID, applicantID, "identity.verify.declined", map[string]any{
				"reason": "applicant_not_found",
				"http_status": s.StatusCode,
			})
			return result, nil
		}
		// 5xx / 429 / network: surface to Temporal so it retries.
		log.Warn("verify_identity http_error", "err", err)
		return nil, err
	}

	// Sanity-check tenant binding — defence in depth against cross-tenant leak.
	if dto.TenantID != "" && dto.TenantID != tenantID {
		log.Error("tenant_mismatch", "got", dto.TenantID, "want", tenantID)
		return nil, temporal.NewNonRetryableApplicationError(
			"tenant_id mismatch from identity-service",
			"TenantMismatch", nil)
	}

	method := "MANUAL"
	if dto.ESIAVerified {
		method = "ESIA"
	}

	result := &wf.IdentityVerificationResult{
		Verified:  dto.ESIAVerified,
		Method:    method,
		CheckedAt: time.Now().UTC(),
	}
	if !dto.ESIAVerified {
		result.Reason = "esia_not_verified"
	}

	eventType := "identity.verify.passed"
	if !dto.ESIAVerified {
		eventType = "identity.verify.declined"
	}
	a.emitAudit(ctx, tenantID, applicantID, eventType, map[string]any{
		"esia_verified": dto.ESIAVerified,
		"esia_subject":  dto.ESIASubject,
		"method":        method,
	})

	log.Info("verify_identity_done",
		"verified", result.Verified,
		"method", result.Method,
	)
	return result, nil
}

func (a *IdentityActivities) emitAudit(ctx context.Context, tenantID, applicantID, eventType string, payload map[string]any) {
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
		EntityType: "applicant",
		EntityID:   applicantID,
		EventType:  eventType,
		ActorID:    "onboarding-orchestrator",
		ActorType:  auditsdk.ActorTypeSystem,
		Payload:    body,
	}); err != nil {
		// Audit is best-effort: log and continue. Never fail the activity for it.
		var apiErr *auditsdk.APIError
		if errors.As(err, &apiErr) {
			a.log.Warn("audit emit failed", "code", apiErr.Code, "status", apiErr.StatusCode)
		} else {
			a.log.Warn("audit emit failed", "err", err)
		}
	}
}
