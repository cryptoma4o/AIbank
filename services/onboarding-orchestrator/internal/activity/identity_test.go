package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"aibank/onboarding-orchestrator/internal/activity"
	wf "aibank/onboarding-orchestrator/internal/workflow"
)

// newIdentityServer spins up a fake identity-service.
func newIdentityServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *activity.IdentityActivities) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a := activity.NewIdentityActivities(activity.IdentityConfig{BaseURL: srv.URL}, nil, nil)
	return srv, a
}

func TestVerifyIdentity_Verified(t *testing.T) {
	_, a := newIdentityServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/applicants/per_alice"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("tenant_id"); got != "alfa" {
			t.Fatalf("tenant_id = %q, want %q", got, "alfa")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "per_alice",
			"tenant_id":     "alfa",
			"esia_verified": true,
			"esia_subject":  "esia_subj_42",
		})
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.VerifyIdentity, registerOpts(wf.ActivityVerifyIdentity))

	val, err := env.ExecuteActivity(wf.ActivityVerifyIdentity, "alfa", "per_alice")
	if err != nil {
		t.Fatalf("activity error: %v", err)
	}
	var got wf.IdentityVerificationResult
	if err := val.Get(&got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !got.Verified {
		t.Errorf("Verified = false, want true")
	}
	if got.Method != "ESIA" {
		t.Errorf("Method = %q, want ESIA", got.Method)
	}
}

func TestVerifyIdentity_NotFound_DeclinesNonRetryably(t *testing.T) {
	calls := 0
	_, a := newIdentityServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"error":{"code":"not_found","message":"applicant missing"}}`, http.StatusNotFound)
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.VerifyIdentity, registerOpts(wf.ActivityVerifyIdentity))

	val, err := env.ExecuteActivity(wf.ActivityVerifyIdentity, "alfa", "missing")
	if err != nil {
		t.Fatalf("activity error: %v", err)
	}
	var got wf.IdentityVerificationResult
	if err := val.Get(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Verified {
		t.Fatalf("Verified = true on 404, want false")
	}
	if got.Reason != "applicant_not_found" {
		t.Errorf("Reason = %q, want applicant_not_found", got.Reason)
	}
	if calls != 1 {
		t.Errorf("server calls = %d, want exactly 1 (404 must NOT be retried)", calls)
	}
}

func TestVerifyIdentity_5xx_IsRetryable(t *testing.T) {
	_, a := newIdentityServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"down","message":"db unavailable"}}`, http.StatusInternalServerError)
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.VerifyIdentity, registerOpts(wf.ActivityVerifyIdentity))

	_, err := env.ExecuteActivity(wf.ActivityVerifyIdentity, "alfa", "per_alice")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	// Should be NOT non-retryable.
	if temporal.IsApplicationError(err) {
		var appErr *temporal.ApplicationError
		// In Temporal SDK 1.27 errors.As is the correct way to unwrap.
		if _, ok := err.(*temporal.ApplicationError); ok {
			appErr = err.(*temporal.ApplicationError)
		}
		if appErr != nil && appErr.NonRetryable() {
			t.Errorf("5xx classified as NonRetryable; want retryable")
		}
	}
}

func TestVerifyIdentity_TenantMismatch(t *testing.T) {
	_, a := newIdentityServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "per_alice",
			"tenant_id":     "other_bank",
			"esia_verified": true,
		})
	})

	env := (&testsuite.WorkflowTestSuite{}).NewTestActivityEnvironment()
	env.RegisterActivityWithOptions(a.VerifyIdentity, registerOpts(wf.ActivityVerifyIdentity))

	_, err := env.ExecuteActivity(wf.ActivityVerifyIdentity, "alfa", "per_alice")
	if err == nil {
		t.Fatal("expected error on tenant mismatch")
	}
	if !strings.Contains(err.Error(), "TenantMismatch") && !strings.Contains(err.Error(), "tenant_id mismatch") {
		t.Errorf("err = %v, want tenant mismatch", err)
	}
}

func TestVerifyIdentity_EmptyArgs_InvalidInput(t *testing.T) {
	a := activity.NewIdentityActivities(activity.IdentityConfig{BaseURL: "http://unused"}, nil, nil)

	_, err := a.VerifyIdentity(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty ids")
	}
}
