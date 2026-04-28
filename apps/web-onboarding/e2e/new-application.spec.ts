import { expect, test } from "@playwright/test";
import { installMocks, NEW_APPLICATION_ID } from "./fixtures/mocks";
import { seedAuth } from "./fixtures/auth";

test.describe("/applications/new", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);
  });

  test("valid form creates application and redirects to detail page", async ({ page }) => {
    await page.goto("/applications/new");

    // Default values: legalEntityType=LLC, productCodes=[current_account_rub] — already valid.
    // Add another product to ensure productCodes serializes as array.
    await page.getByLabel(/Эквайринг/i).check();

    await page.getByRole("button", { name: /Создать заявку/i }).click();

    await page.waitForURL(`**/applications/${NEW_APPLICATION_ID}`, { timeout: 10_000 });
    await expect(page).toHaveURL(new RegExp(`/applications/${NEW_APPLICATION_ID}$`));
  });

  test("incomplete form (no products) shows validation error", async ({ page }) => {
    await page.goto("/applications/new");

    // Uncheck the default product to leave productCodes empty.
    await page.getByLabel(/Расчётный счёт \(RUB\)/i).uncheck();

    await page.getByRole("button", { name: /Создать заявку/i }).click();

    // ErrorBanner or inline validation should surface the message.
    await expect(
      page.getByText(/Выберите хотя бы один продукт/i).first()
    ).toBeVisible();

    // Still on /new — no redirect.
    await expect(page).toHaveURL(/\/applications\/new$/);
  });
});
