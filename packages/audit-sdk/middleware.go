package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// AuthInfo describes the caller as derived from JWT (or other auth)
// middleware running before EmitOnSuccess. Services typically populate
// this in their auth middleware via WithAuthInfo.
type AuthInfo struct {
	TenantID  string
	ActorID   string
	ActorType ActorType
}

type authInfoKey struct{}

// WithAuthInfo stores AuthInfo on ctx so EmitOnSuccess can pick it up.
func WithAuthInfo(ctx context.Context, info AuthInfo) context.Context {
	return context.WithValue(ctx, authInfoKey{}, info)
}

// AuthInfoFromContext returns the AuthInfo previously stored, or zero
// value with ok=false if none was attached.
func AuthInfoFromContext(ctx context.Context) (AuthInfo, bool) {
	v, ok := ctx.Value(authInfoKey{}).(AuthInfo)
	return v, ok
}

// EntityResolver lets callers compute (entityType, entityID, payload)
// from the in-flight request, since those vary per-route. Returning
// ok=false skips the audit emission for that request without an error.
type EntityResolver func(r *http.Request) (entityType, entityID string, payload json.RawMessage, ok bool)

// entityRef is the mutable holder middleware places in the request context
// before invoking the handler. The handler may call SetEntity to fill it
// in after a successful domain mutation. If unset after handler return,
// EmitOnSuccess falls back to the configured EntityResolver.
type entityRef struct {
	Type     string
	ID       string
	TenantID string
	Payload  json.RawMessage
}

type entityRefKey struct{}

// SetEntity assigns the audit entity from inside a handler. Use this once
// you have a freshly-created/updated domain object's id available — e.g.
// `auditsdk.SetEntity(r.Context(), "tenant", t.ID, payloadJSON)`.
//
// No-op if EmitOnSuccess middleware is not in the chain.
func SetEntity(ctx context.Context, entityType, entityID string, payload json.RawMessage) {
	if ref, ok := ctx.Value(entityRefKey{}).(*entityRef); ok {
		ref.Type = entityType
		ref.ID = entityID
		ref.Payload = payload
	}
}

// SetTenantID overrides the tenant_id used for the audit event. Handlers
// that derive tenant from request body (e.g. POST /tenants) call this
// after parsing. Services with JWT-derived AuthInfo can skip it.
//
// No-op if EmitOnSuccess middleware is not in the chain.
func SetTenantID(ctx context.Context, tenantID string) {
	if ref, ok := ctx.Value(entityRefKey{}).(*entityRef); ok {
		ref.TenantID = tenantID
	}
}

// EmitOnSuccess returns chi-compatible middleware that calls
// client.Append after the wrapped handler responds with a 2xx status.
//
// Failures inside Append are logged via log but never block or alter
// the HTTP response — audit is best-effort for the request path; durable
// guarantees are the caller's job (e.g. transactional outbox).
//
// resolve must be non-nil. eventType is the constant slug used as
// AuditEvent.event_type (e.g. "tenant.created").
func EmitOnSuccess(client *Client, eventType string, resolve EntityResolver, log *slog.Logger) func(http.Handler) http.Handler {
	if client == nil {
		panic("audit: EmitOnSuccess requires a non-nil client")
	}
	if resolve == nil {
		panic("audit: EmitOnSuccess requires a non-nil resolver")
	}
	if log == nil {
		log = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
			ref := &entityRef{}
			reqCtx := context.WithValue(r.Context(), entityRefKey{}, ref)
			next.ServeHTTP(rw, r.WithContext(reqCtx))

			if rw.status < 200 || rw.status >= 300 {
				return
			}

			info, ok := AuthInfoFromContext(r.Context())
			tenantID := info.TenantID
			if ref.TenantID != "" {
				tenantID = ref.TenantID
			}
			if !ok || tenantID == "" || info.ActorID == "" {
				log.Warn("audit: skipping emit, missing auth info",
					"path", r.URL.Path, "status", rw.status)
				return
			}
			actorType := info.ActorType
			if !actorType.IsValid() {
				actorType = ActorTypeSystem
			}

			// Prefer entity set by handler via SetEntity; fall back to resolver.
			var entityType, entityID string
			var payload json.RawMessage
			if ref.ID != "" {
				entityType, entityID, payload, ok = ref.Type, ref.ID, ref.Payload, true
			} else {
				entityType, entityID, payload, ok = resolve(r)
			}
			if !ok {
				return
			}

			ev := RecordEventRequest{
				TenantID:   tenantID,
				EntityType: entityType,
				EntityID:   entityID,
				EventType:  eventType,
				ActorID:    info.ActorID,
				ActorType:  actorType,
				Payload:    payload,
			}

			// Detach from request lifecycle: emit on a background ctx so
			// shutdowns don't kill the audit write. Keep correlation id.
			ctx := context.Background()
			if cid := CorrelationFromContext(r.Context()); cid != "" {
				ctx = ContextWithCorrelation(ctx, cid)
			}
			if _, err := client.Append(ctx, ev); err != nil {
				log.Error("audit: append failed",
					"event_type", eventType,
					"tenant", info.TenantID,
					"actor", info.ActorID,
					"err", err)
			}
		})
	}
}

// captureWriter records the response status code so the middleware can
// branch on success vs failure without buffering the body.
type captureWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (c *captureWriter) WriteHeader(code int) {
	if c.wroteHeader {
		return
	}
	c.status = code
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.ResponseWriter.Write(b)
}
