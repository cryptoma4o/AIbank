# web-admin · End-to-end tests

Smoke tests for the bank-side admin console.

## Running

```bash
npm install
npx playwright install        # one-off, ~500 MB
npm run e2e
```

## Coverage

| File | Scenarios |
|---|---|
| `login.spec.ts` | role-check: `bank.admin` allowed, `bank.applicant` denied |
| `applications.spec.ts` | `/applications` renders with status filter badges |
| `audit.spec.ts` | `/audit` renders 5 mock events (or graceful empty state) |

## Mocks

Network mocks via `page.route` in [`fixtures/mocks.ts`](./fixtures/mocks.ts).
Mocked endpoints:

- `POST /v1/auth/login` — operator login, role taken from `loginRole` opt
- `GET /v1/me` — operator profile
- `POST /graphql` (operationName=`GetApplications`) — application list
- `POST /graphql` (operationName=`GetAuditEvents`) — audit events

JWT payloads come from [`fixtures/auth.ts`](./fixtures/auth.ts) — same
shape as identity-service `JWTClaims`, signature is a stub.

## TODOs

- Visual regression (deferred).
- CI pipeline: `npx playwright install --with-deps && npm run e2e`.
- Add `/dashboard` test once the page lands.
