package audit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// HeaderCorrelationID is the canonical HTTP header carrying a correlation
// id between services. Picked to align with common ingress conventions.
const HeaderCorrelationID = "X-Correlation-ID"

type correlationKey struct{}

// NewCorrelationID returns a 128-bit random hex string prefixed with the
// epoch milliseconds. Deterministic enough to grep for, random enough to
// not collide. No external uuid dep so the SDK stays stdlib-only.
//
// Example: "1714125600123-3a7c4f1e9b2d6a8c"
func NewCorrelationID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	ts := time.Now().UTC().UnixMilli()
	return formatTS(ts) + "-" + hex.EncodeToString(b[:])
}

// ContextWithCorrelation returns a child context carrying id. If id is
// empty a fresh one is generated.
func ContextWithCorrelation(ctx context.Context, id string) context.Context {
	if id == "" {
		id = NewCorrelationID()
	}
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationFromContext returns the correlation id from ctx, or "" if
// none is set.
func CorrelationFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(correlationKey{}).(string)
	return v
}

// CorrelationFromRequest extracts the correlation id from the request
// header (HeaderCorrelationID). Empty if missing.
func CorrelationFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.Header.Get(HeaderCorrelationID)
}

// formatTS formats a positive int64 as a base-10 string without
// strconv (kept dep-free for parity with the rest of the SDK).
func formatTS(n int64) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
