# scripts/

Operational scripts for the AIbank stack.

## smoke.sh

End-to-end happy-path smoke test. Walks through the core onboarding
pipeline and asserts every step returns the expected HTTP status.

### What it does

1. Verifies `docker compose` has at least 5 services running.
2. Polls `/health` (Go services) and `/healthz` (Python services) on
   tenant-service, audit-service, identity-service, llm-gateway and
   agent-conversational until healthy (60s timeout).
3. Runs platform migrations via `db-migrator-platform` (one-shot job).
4. Creates tenant `demo` (`POST /v1/tenants`).
5. Verifies tenant retrieval (`GET /v1/tenants/demo`).
6. Writes an audit event (`POST /v1/events`).
7. Calls llm-gateway in mock mode and asserts `aibank_gateway` metadata
   is present in the response.
8. Calls agent-conversational `/v1/chat`.
9. Fetches `/v1/usage?tenant_id=demo` and asserts a `data` array is
   present.

### How to run

```bash
make up                # start full stack
make migrate-platform  # apply tenant-service platform migrations
make smoke             # run end-to-end happy path
```

### Expected duration

Roughly **30 seconds** once the stack is already up. First run after
`make up` may add another 30–60s while services finish warming up.

### How to clean up

```bash
make smoke-clean       # docker compose down -v + rm /tmp/aibank-smoke-state
```

This removes Docker volumes (Postgres, MinIO, Qdrant data) and the
smoke state directory.

### Output and artifacts

The script writes per-step artifacts to `/tmp/aibank-smoke-state/`:

- `last-step` — last step number/description/status (also updated on
  failure).
- `migrate.log` — db-migrator output.
- `tenant-create.json`, `tenant-get.json`, `audit-create.json`,
  `llm-chat.json`, `agent-chat.json`, `usage.json` — raw HTTP response
  bodies for each step.

On success the script prints `✓ smoke OK`. On failure it prints
`✗ smoke FAILED at step N: <description>` and exits with code 1.

### Troubleshooting

- **Health check timeout** — service didn't come up in 60s.
  Check logs: `docker compose logs <service>` (e.g. `tenant-service`,
  `llm-gateway`).
- **Step 4 returns 409** — tenant already exists from a previous run.
  The script treats 409 as idempotent success. Use `make smoke-clean`
  to start from a fresh DB.
- **Step 7 missing `aibank_gateway` metadata** — llm-gateway is not in
  mock mode. Verify `LLM_GATEWAY_FORCE_MOCK=1` is set in
  `docker-compose.yml`.
- **Migration step hangs** — Postgres may not be healthy yet.
  Run `docker compose ps postgres` and check the `STATUS` column for
  `(healthy)`.
- **Inspect last failure** — `cat /tmp/aibank-smoke-state/last-step`
  shows the step number, description, status and exit code of the
  most recent run.
