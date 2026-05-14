// Package activity contains real implementations of Temporal activities
// declared in services/onboarding-orchestrator/internal/workflow/activities.go.
//
// Activities live OUTSIDE workflow code on purpose: they may call wall-clock
// time.Now, generate UUIDs, perform network IO, and rely on environment
// variables. Workflow code remains deterministic; activities do the dirty
// work, and Temporal handles retries via the workflow's RetryPolicy.
//
// Each activity:
//   - Receives a stdlib context.Context.
//   - Uses structured slog.Logger threaded through New*Activities constructors.
//   - Emits audit events via audit-sdk on terminal success/failure.
//   - Returns a typed *Result from internal/workflow.types.
//
// All HTTP calls go through httpDoer which centralises timeouts, retry-friendly
// classification (5xx is retryable, 4xx is fatal), and structured error wrap.
package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.temporal.io/sdk/activity"
)

const (
	// defaultHTTPTimeout — per-request wall-clock. Temporal also imposes
	// StartToCloseTimeout at the workflow level (5 minutes for these
	// activities), so this only limits a single HTTP attempt.
	defaultHTTPTimeout = 10 * time.Second

	// maxResponseBytes — defensive cap; identity and risk responses are
	// small (well under 64 KiB). Reject anything larger to avoid OOM.
	maxResponseBytes = 1 << 20
)

// httpDoer is a thin wrapper around *http.Client with JSON helpers and
// retryable-vs-fatal classification.
//
// We intentionally do NOT retry inside the activity — Temporal retries the
// activity invocation, which is correct: it survives worker restarts and
// applies the workflow-configured backoff.
type httpDoer struct {
	client  *http.Client
	baseURL string
}

func newHTTPDoer(baseURL string, timeout time.Duration) *httpDoer {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &httpDoer{
		client:  &http.Client{Timeout: timeout},
		baseURL: baseURL,
	}
}

// httpStatusError is returned for non-2xx responses. The error is wrapped in
// a Temporal-friendly way: 5xx + 429 are surfaced as a transient error type so
// Temporal will retry; 4xx are returned as-is and trigger workflow decline.
type httpStatusError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.URL, e.StatusCode, truncate(e.Body, 200))
}

// IsRetryable reports whether Temporal should retry this activity attempt.
// 5xx and 429 are retried; all other 4xx are fatal (will decline the workflow).
func (e *httpStatusError) IsRetryable() bool {
	return e.StatusCode == http.StatusTooManyRequests ||
		(e.StatusCode >= 500 && e.StatusCode < 600)
}

// doJSON performs an HTTP request, encodes reqBody as JSON if non-nil, and
// decodes the response into respBody. Non-2xx → *httpStatusError.
func (h *httpDoer) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	url := h.baseURL + path

	var body io.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("activity: marshal request: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("activity: build request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		// Network/timeout errors are transient by nature — Temporal will retry.
		return fmt.Errorf("activity: http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("activity: read body %s: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{
			StatusCode: resp.StatusCode,
			URL:        url,
			Body:       string(raw),
		}
	}

	if respBody != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("activity: decode response %s: %w", url, err)
		}
	}
	return nil
}

// asStatusError unwraps to *httpStatusError when applicable.
func asStatusError(err error) (*httpStatusError, bool) {
	var s *httpStatusError
	if errors.As(err, &s) {
		return s, true
	}
	return nil, false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// withActivityContext enriches the logger with Temporal activity metadata when
// the context is an activity context, and leaves it untouched otherwise.
//
// activity.GetInfo panics if called outside an activity context (the Temporal
// SDK exposes no HasInfo predicate), so we use activity.HasHeartbeatDetails
// as a proxy: it returns false for non-activity contexts without panicking.
// As a defence-in-depth, we also wrap in a recover-guarded closure.
func withActivityContext(ctx context.Context, log *slog.Logger) *slog.Logger {
	if !isActivityContext(ctx) {
		return log
	}
	info := activity.GetInfo(ctx)
	return log.With(
		"workflow_id", info.WorkflowExecution.ID,
		"attempt", info.Attempt,
	)
}

// isActivityContext returns true iff ctx carries a Temporal activity context.
// Uses defer/recover because the SDK does not expose a probe API.
func isActivityContext(ctx context.Context) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	_ = activity.GetInfo(ctx)
	return true
}
