// applicant-register-login.spec.ts — UI smoke для публичных страниц
// web-onboarding (3000): /login и /register. Проверяет, что страницы
// рендерятся без 500/build-error и ключевые поля формы доступны.
//
// Реальную регистрацию против identity-service (с записью в БД) делает
// API-уровень: scripts/seed-staging-companies.py + tests/e2e/api/.

import { expect, test } from "@playwright/test";

test.describe("web-onboarding / public pages", () => {
  test("/login: форма входа отрисована", async ({ page }) => {
    const resp = await page.goto("/login");
    expect(resp?.status()).toBeLessThan(400);

    await expect(page.getByRole("heading", { name: /Вход в кабинет/i })).toBeVisible();
    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator("#tenant_id")).toBeVisible();
    await expect(page.getByRole("button", { name: /Войти/i })).toBeEnabled();
  });

  test("/register: форма регистрации отрисована", async ({ page }) => {
    const resp = await page.goto("/register");
    expect(resp?.status()).toBeLessThan(400);

    // react-hook-form биндит по name=. id может отсутствовать.
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('input[name="inn"]')).toBeVisible();
    await expect(page.locator('input[name="phone"]')).toBeVisible();
    await expect(page.locator('input[name="full_name"]')).toBeVisible();
    // Должна быть какая-то кнопка submit (точное название может меняться).
    await expect(page.getByRole("button").first()).toBeVisible();
  });

  test("/login → пустая submission показывает валидацию", async ({ page }) => {
    await page.goto("/login");
    await page.getByRole("button", { name: /Войти/i }).click();
    const errorText = page.getByText(/Введите email|Введите пароль|Идентификатор/i);
    await expect(errorText.first()).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
  });

  test("/ → 307 redirect to /login (unauthenticated)", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/login$/);
  });
});
