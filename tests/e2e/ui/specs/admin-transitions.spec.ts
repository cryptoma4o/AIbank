// admin-transitions.spec.ts — UI smoke для web-admin (3001).
// Покрывает /login (публичная) и /applications (authenticated).
//
// Реальные state-machine transitions (через bff-admin GraphQL) делаются
// API-тестами: tests/e2e/api/test_admin_flow.py.

import { expect, test } from "@playwright/test";

import { seedFakeAuth } from "../fixtures/auth";

test.describe("web-admin / public", () => {
  test("/login: форма входа отрисована", async ({ page }) => {
    const resp = await page.goto("/login");
    expect(resp?.status()).toBeLessThan(400);

    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    // tenant_id опциональный (для platform.admin) — но поле в форме есть.
    await expect(page.locator('input[name="tenant_id"]')).toBeVisible();
    await expect(page.getByRole("button").first()).toBeVisible();
  });

  test("/login: пустая submission не редиректит", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button").first().click();
    // Любая валидационная реакция (banner/inline) — но НЕ редирект.
    await expect(page).toHaveURL(/\/login$/);
  });
});

test.describe("web-admin / authenticated", () => {
  test.beforeEach(async ({ page }) => {
    await seedFakeAuth(page, { tenantId: "demo", role: "bank.operator" });
  });

  test("/applications: список доступен оператору", async ({ page }) => {
    const resp = await page.goto("/applications");
    expect(resp?.status()).toBeLessThan(500);
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page.locator("main, [role=main]")).toBeVisible();
  });

  test("/ → redirect to /applications", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/applications/);
  });
});
