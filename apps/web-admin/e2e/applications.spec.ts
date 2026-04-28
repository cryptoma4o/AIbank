import { expect, test } from "@playwright/test";
import { seedAdminAuth } from "./fixtures/auth";
import { installAdminMocks } from "./fixtures/mocks";

test.describe("/applications (admin)", () => {
  test("renders applications page heading and status badges", async ({ page, baseURL }) => {
    await seedAdminAuth(page, { baseURL: baseURL!, role: "bank.admin" });
    await installAdminMocks(page);

    await page.goto("/applications");

    await expect(page.getByRole("heading", { name: /Заявки/i })).toBeVisible();

    // Static page renders status filter badges (draft, validating, etc.).
    // We assert at least 3 of the well-known statuses are present.
    for (const status of ["draft", "auto_approved", "rejected"]) {
      await expect(page.getByText(status, { exact: false }).first()).toBeVisible();
    }
  });
});
