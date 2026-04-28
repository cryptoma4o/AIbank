# audit-sdk

Tiny clients for the platform `audit-service` HTTP API. One module per
language; both wrap the same contract:

- `POST /v1/events` → append a tamper-evident event to the per-tenant
  hash chain.
- `GET /v1/events` → list events with `tenant_id` (required) plus
  optional `entity_type`, `entity_id`, `limit` filters.

The contract is captured in [`packages/openapi/audit-service.yaml`](../openapi/audit-service.yaml).
If the wire shape changes, that spec, this SDK, and `services/audit-service`
must be updated together.

## Why an SDK at all?

Per `docs/technical-structure.md` and ADR-0010, every service writes to
audit-service. Repeating the marshal/retry/auth glue in each codebase is
how subtle inconsistencies creep in (timeouts, header names, retry
budgets, payload size limits). One thin wrapper means:

- consistent retry policy (3 attempts, exp backoff, retryable on 5xx/429
  only — never on 4xx);
- consistent payload validation before the network hop (cheap fail-fast);
- consistent correlation-id propagation across services;
- a single place to evolve when ADR-0010 follow-ups land
  (e.g. transactional outbox, signing).

## Go module

`module github.com/aibank/platform/packages/audit-sdk` — stdlib only.
Use it from any service in the monorepo via a `replace` directive in
`go.mod`:

```go
require github.com/aibank/platform/packages/audit-sdk v0.0.0
replace github.com/aibank/platform/packages/audit-sdk => ../../packages/audit-sdk
```

### Direct API

```go
import audit "github.com/aibank/platform/packages/audit-sdk"

c, err := audit.NewClient(audit.ClientOptions{
    BaseURL: "http://audit-service:8081",
    Timeout: 5 * time.Second,
    APIKey:  os.Getenv("AUDIT_API_KEY"), // optional Bearer
})
if err != nil { /* fail-fast in main */ }

ev, err := c.Append(ctx, audit.RecordEventRequest{
    TenantID:   "bank_alpha",
    EntityType: "application",
    EntityID:   "app_42",
    EventType:  "application.created",
    ActorID:    "user_7",
    ActorType:  audit.ActorTypeUser,
    Payload:    json.RawMessage(`{"channel":"web"}`),
})
```

`Append` retries on 5xx/429 with exponential backoff up to
`MaxRetries` (default 3), bounded by the per-call timeout / context
deadline. 4xx errors return `*audit.APIError` immediately.

### chi middleware

Most services want "if the handler returns 2xx, write an audit event".
That's what `EmitOnSuccess` is for:

```go
r.With(authMiddleware).
  With(audit.EmitOnSuccess(auditClient, "application.created", resolveApp, log)).
  Post("/v1/applications", appHandler.Create)
```

`resolveApp` extracts `(entityType, entityID, payload)` from the
in-flight request — this varies per route, so the SDK keeps it pluggable.
The auth middleware in front is expected to call
`audit.WithAuthInfo(ctx, audit.AuthInfo{...})` so the audit middleware
can read the tenant + actor.

The audit emit happens on a detached `context.Background()` so HTTP
shutdown doesn't kill it; failures are logged but never alter the
upstream response.

### Correlation IDs

```go
ctx := audit.ContextWithCorrelation(r.Context(), audit.CorrelationFromRequest(r))
```

`audit.NewCorrelationID()` produces a `<unix-millis>-<16hex>` string —
human-greppable, dependency-free.

## Python module (`python/`)

For Python AI services. Mirrors the Go API:

```python
from aibank_audit import AsyncAuditClient, RecordEventRequest, ActorType

async with AsyncAuditClient(base_url="http://audit-service:8081") as client:
    event = await client.append(RecordEventRequest(
        tenant_id="bank_alpha",
        entity_type="application",
        entity_id="app_42",
        event_type="application.created",
        actor_id="agent_doc_intake",
        actor_type=ActorType.AI_AGENT,
        payload={"channel": "web"},
    ))
```

Install (editable, from monorepo root):

```bash
pip install -e packages/audit-sdk/python
```

## Tests

```bash
# Go
cd packages/audit-sdk && go test ./...

# Python (requires httpx + pytest + pytest-asyncio)
cd packages/audit-sdk/python && pytest
```

## Integration patterns

1. **Fail-fast in `main`**: build the client at startup and let `Validate`
   errors crash the process.
2. **Best-effort emission**: `EmitOnSuccess` does not retry on top of the
   client's retry — the client already does. If you need durable writes,
   layer a transactional outbox in front (out of scope here).
3. **Never call `Append` from a Temporal workflow** — workflows must be
   deterministic. Use it from activities only.
4. **Tenant isolation**: every event carries `tenant_id`. The SDK never
   defaults it; the call site must derive it from the auth context.
