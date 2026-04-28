import { expect, test } from "@playwright/test";
import { installMocks } from "./fixtures/mocks";
import { readStoredToken } from "./fixtures/auth";

test.describe("/login", () => {
  test.beforeEach(async ({ page }) => {
    await installMocks(page);
  });

  test("renders form with email, password, tenant_id fields", async ({ page }) => {
    await page.goto("/login");

    await expect(page.getByRole("heading", { name: /Вход в кабинет/i })).toBeVisible();
    await expect(page.locator("#email")).toBeVisible();
    await expect(page.locator("#password")).toBeVisible();
    await expect(page.locator("#tenant_id")).toBeVisible();
    await expect(page.getByRole("button", { name: /Войти/i })).toBeEnabled();
  });

  test("empty submission shows validation errors", async ({ page }) => {
    await page.goto("/login");

    await page.getByRole("button", { name: /Войти/i }).click();

    // react-hook-form + zodResolver-style: page renders inline errors next to fields.
    // login/page.tsx uses safeParse() → setSubmitError on first failure → ErrorBanner.
    // Either path is acceptable; assert that *some* validation feedback is shown.
    const banner = page.getByText(/Введите email|Введите пароль|Идентификатор банка/i);
    await expect(banner.first()).toBeVisible();

    // We must NOT have been redirected.
    await expect(page).toHaveURL(/\/login$/);
  });

  test("valid creds redirect to /applications and store JWT", async ({ page }) => {
    await page.goto("/login");

    await page.locator("#email").fill("applicant@example.com");
    await page.locator("#password").fill("Sup3rSecret!");
    await page.locator("#tenant_id").fill("bank_alpha");

    await page.getByRole("button", { name: /Войти/i }).click();

    await page.waitForURL("**/applications", { timeout: 10_000 });
    await expect(page).toHaveURL(/\/applications$/);

    const token = await readStoredToken(page);
    expect(token).toBeTruthy();
    expect(token!.split(".")).toHaveLength(3);
  });
});
