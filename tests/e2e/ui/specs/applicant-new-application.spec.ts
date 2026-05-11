// applicant-new-application.spec.ts — UI smoke для авторизованной зоны
// web-onboarding: /applications и /applications/new.
//
// Использует seedFakeAuth (JWT-shape в localStorage) — UI рендерится, но
// API-запросы (например, MyApplications query через bff-onboarding) могут
// упасть с 401 (фейковая подпись). Это smoke на UI-структуру, не на бэкенд.

import { expect, test } from "@playwright/test";

import { seedFakeAuth } from "../fixtures/auth";

test.describe("web-onboarding / authenticated zone", () => {
  test.beforeEach(async ({ page }) => {
    await seedFakeAuth(page, { tenantId: "demo", role: "bank.applicant" });
  });

  test("/applications: страница рендерится для authenticated user", async ({ page }) => {
    const resp = await page.goto("/applications");
    expect(resp?.status()).toBeLessThan(500);

    // AuthGuard видит токены → не редиректит на /login.
    await expect(page).not.toHaveURL(/\/login/);
    // Страница содержит хоть какой-то контент (heading/main).
    await expect(page.locator("main, [role=main]")).toBeVisible();
  });

  test("/applications/new: форма prequalify доступна", async ({ page }) => {
    const resp = await page.goto("/applications/new");
    expect(resp?.status()).toBeLessThan(500);
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page.locator("main, [role=main]")).toBeVisible();
  });
});
