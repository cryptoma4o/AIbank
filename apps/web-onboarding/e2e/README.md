# web-onboarding · End-to-end tests

Playwright tests against the Next.js dev server with **fully mocked
backends** — no identity-service / bff-onboarding required.

## Running

```bash
# 1. Install deps (once)
npm install

# 2. Install Playwright browsers (~500 MB, once)
npx playwright install
# CI: npx playwright install --with-deps

# 3. Run all e2e tests
npm run e2e

# Headed mode (visible browser, slower)
npm run e2e:headed

# Run a specific file
npx playwright test e2e/login.spec.ts

# Debug a single test
npx playwright test --debug e2e/applications.spec.ts
```

## How mocks work

- **Backend URLs** are rewritten by `playwright.config.ts → webServer.env`
  to `http://localhost:3000/__mock__/...`.
- **Network interception** happens in
  [`fixtures/mocks.ts`](./fixtures/mocks.ts) via Playwright `page.route`,
  matching by GraphQL `operationName` (`MyApplications`, `Application`,
  `SubmitApplication`, `Me`) and REST path (`/v1/auth/login`, `/v1/me`).
- **Auth** is seeded via [`fixtures/auth.ts`](./fixtures/auth.ts) — a
  JWT-shaped string is written into `localStorage` *before* navigation
  using `page.addInitScript`, so `AuthGuard` sees it on first render.

`msw` is listed as a devDependency for future jsdom component tests; the
e2e suite uses Playwright's native interception, which is the
recommended pattern when tests live outside the browser bundle.

## Coverage

| File | Scenarios |
|---|---|
| `login.spec.ts` | form fields render · empty submit shows validation · valid creds redirect + store JWT |
| `applications.spec.ts` | unauthenticated → /login · 2 mock rows render · "Новая заявка" navigates |
| `new-application.spec.ts` | valid submit → detail redirect · no products → validation |
| `application-detail.spec.ts` | timeline+applicant+documents render · `collecting_documents` shows upload button |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `Executable doesn't exist at /…/chrome-headless-shell` | `npx playwright install` |
| Hangs on "waiting for webServer" | Free port 3000 (`lsof -i :3000`) or kill stale `next dev` |
| Tests pass locally, fail in CI | Run with `--with-deps`: `npx playwright install --with-deps` |
| AuthGuard always redirects to /login | Token shape changed — update `makeFakeJwt()` in `fixtures/auth.ts` |
| Redirect doesn't happen on login | Check `installMocks()` was called *before* `page.goto()` |

## TODOs

- Visual-regression snapshots (deferred to CI infra ticket).
- Component tests with `msw` + `@testing-library` for fast inner-loop
  coverage of forms in isolation.
- Mobile viewport project (currently chromium-desktop only).
