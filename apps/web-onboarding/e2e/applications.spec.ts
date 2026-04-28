import { expect, test } from "@playwright/test";
import { installMocks, SAMPLE_APPLICATIONS } from "./fixtures/mocks";
import { seedAuth } from "./fixtures/auth";

test.describe("/applications", () => {
  test("unauthenticated visit redirects to /login", async ({ page, baseURL }) => {
    await installMocks(page);
    // No seedAuth → AuthGuard should bounce to /login.
    await page.goto("/applications");

    await page.waitForURL("**/login", { timeout: 10_000 });
    await expect(page).toHaveURL(/\/login$/);
    expect(baseURL).toBeTruthy();
  });

  test("authenticated user sees 2 applications from mock", async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);

    await page.goto("/applications");

    await expect(page.getByRole("heading", { name: /Мои заявки/i })).toBeVisible();

    // Wait for table rows (one per mock application).
    const rows = page.locator("tbody tr");
    await expect(rows).toHaveCount(SAMPLE_APPLICATIONS.length);

    // First row should reference an id starting with the first mock UUID prefix.
    await expect(rows.first()).toContainText(SAMPLE_APPLICATIONS[0].id.slice(0, 8));
  });

  test("'Новая заявка' button navigates to /applications/new", async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);

    await page.goto("/applications");
    await page.getByRole("link", { name: /Новая заявка/i }).first().click();

    await page.waitForURL("**/applications/new");
    await expect(page).toHaveURL(/\/applications\/new$/);
    await expect(page.getByRole("heading", { name: /Новая заявка/i })).toBeVisible();
  });
});
