package activity_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.temporal.io/sdk/testsuite"

	"aibank/onboarding-orchestrator/internal/activity"
	wf "aibank/onboarding-orchestrator/internal/workflow"
)

func newRiskServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *activity.RiskActivities) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a := activity.NewRiskActivities(activity.RiskConfig{BaseURL: srv.URL}, nil, nil)
	return srv, a
}

func TestRunRiskAssessment_Success(t *testing.T) {
	var seen map[string]any
	_, a := newRiskServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/assessments" {
			t.Fatalf("want POST /v1/assessments, got %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seen)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             "ra_42",
			"tenant_id":      "alfa",
			"application_id": "app_xyz",
			"score":          0.27,
			"category":       "LOW",
			"recommendation": "AUTO_APPROVE",
			"rules_triggered": []map[string]any{},
		})
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.RunRiskAssessment, registerOpts(wf.ActivityAssessRisk))

	val, err := env.ExecuteActivity(wf.ActivityAssessRisk, "alfa", "app_xyz")
	if err != nil {
		t.Fatalf("activity error: %v", err)
	}
	var got wf.RiskAssessmentResult
	if err := val.Get(&got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.AssessmentID != "ra_42" || got.Score != 0.27 || got.Recommendation != "AUTO_APPROVE" {
		t.Errorf("got = %+v", got)
	}
	if got.HasBlockingRule {
		t.Errorf("HasBlockingRule = true, want false")
	}
	if seen["tenant_id"] != "alfa" || seen["application_id"] != "app_xyz" {
		t.Errorf("request body = %v", seen)
	}
}

func TestRunRiskAssessment_BlockingRule(t *testing.T) {
	_, a := newRiskServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             "ra_99",
			"tenant_id":      "alfa",
			"application_id": "app_xyz",
			"score":          0.95,
			"category":       "HIGH",
			"recommendation": "DECLINE_RECOMMENDED",
			"rules_triggered": []map[string]any{
				{"rule_id": "OKVED_BLOCKED", "severity": "BLOCKING", "description": "blocked OKVED"},
			},
		})
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.RunRiskAssessment, registerOpts(wf.ActivityAssessRisk))

	val, err := env.ExecuteActivity(wf.ActivityAssessRisk, "alfa", "app_xyz")
	if err != nil {
		t.Fatalf("activity error: %v", err)
	}
	var got wf.RiskAssessmentResult
	_ = val.Get(&got)
	if !got.HasBlockingRule {
		t.Errorf("HasBlockingRule = false; want true")
	}
	if !strings.Contains(got.BlockingReason, "OKVED_BLOCKED") {
		t.Errorf("BlockingReason = %q, want contains OKVED_BLOCKED", got.BlockingReason)
	}
}

func TestRunRiskAssessment_5xx_Retryable_DeadLetters(t *testing.T) {
	var calls atomic.Int32
	_, a := newRiskServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"error":{"code":"down","message":"pipeline down"}}`, http.StatusBadGateway)
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.RunRiskAssessment, registerOpts(wf.ActivityAssessRisk))

	_, err := env.ExecuteActivity(wf.ActivityAssessRisk, "alfa", "app_xyz")
	if err == nil {
		t.Fatal("expected error on 502")
	}
	// TestActivityEnvironment executes once (no retry policy applied); the
	// important property is that the error is NOT classified as
	// non-retryable. If we got here with a non-retryable application error,
	// Temporal in production would NOT retry — that is the regression we
	// want to prevent.
	if strings.Contains(err.Error(), "NonRetryable") {
		t.Errorf("err = %v contains NonRetryable; 5xx should be retryable", err)
	}
	if calls.Load() != 1 {
		t.Errorf("server calls = %d; want 1 (TestActivityEnvironment does not retry)", calls.Load())
	}
}

func TestRunRiskAssessment_4xx_NonRetryable(t *testing.T) {
	_, a := newRiskServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"validation_failed","message":"bad input"}}`, http.StatusBadRequest)
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.RunRiskAssessment, registerOpts(wf.ActivityAssessRisk))

	_, err := env.ExecuteActivity(wf.ActivityAssessRisk, "alfa", "app_xyz")
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "RiskEngine4xx") && !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v; want classified as 4xx/RiskEngine4xx", err)
	}
}
