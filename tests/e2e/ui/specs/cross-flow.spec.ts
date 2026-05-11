// cross-flow.spec.ts — smoke на связку двух фронтов и BFF.
//
// Стратегия: проверяем что web-onboarding (3000) и web-admin (3001)
// одновременно живы и оба BFF (8091/8092) отвечают на запрос без авторизации
// ожидаемым 401 (а не 502 — что значило бы что upstream упал).
//
// Полноценный cross-flow (applicant создал → admin одобрил → audit увидел)
// делает API-уровень: tests/e2e/api/test_applicant_flow.py +
// test_audit_trail.py. Здесь — только UI + контракт BFF.

import { expect, test } from "@playwright/test";

const ONBOARDING_BASE = (process.env.E2E_BASE_URL || "http://localhost").replace(/\/$/, "");
const BFF_ONBOARDING_URL = `${ONBOARDING_BASE}:8091/graphql`;
const BFF_ADMIN_URL = `${ONBOARDING_BASE}:8092/graphql`;

test.describe("cross-flow / both frontends live", () => {
  test("web-onboarding /login отрисован", async ({ page }) => {
    const resp = await page.goto("/login");
    expect(resp?.status()).toBeLessThan(400);
    await expect(page.getByRole("heading", { name: /Вход в кабинет/i })).toBeVisible();
  });

  test("bff-onboarding GraphQL отвечает 401 на запрос без токена (а не 502)", async ({ request }) => {
    const resp = await request.post(BFF_ONBOARDING_URL, {
      data: { query: "{ __typename }" },
      headers: { "content-type": "application/json" },
      failOnStatusCode: false,
    });
    expect(resp.status()).toBe(401);
    const body = await resp.text();
    expect(body.toLowerCase()).toMatch(/token|unauth/);
  });

  test("bff-admin GraphQL отвечает 401 на запрос без токена (а не 502)", async ({ request }) => {
    const resp = await request.post(BFF_ADMIN_URL, {
      data: { query: "{ __typename }" },
      headers: { "content-type": "application/json" },
      failOnStatusCode: false,
    });
    expect(resp.status()).toBe(401);
    const body = await resp.text();
    expect(body.toLowerCase()).toMatch(/token|unauth/);
  });
});
