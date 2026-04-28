import { expect, test } from "@playwright/test";
import { installMocks, SAMPLE_APPLICATION_DETAIL } from "./fixtures/mocks";
import { seedAuth } from "./fixtures/auth";

test.describe("/applications/[id]", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);
  });

  test("renders state timeline, applicant block, and documents section", async ({ page }) => {
    await page.goto(`/applications/${SAMPLE_APPLICATION_DETAIL.id}`);

    // Heading uses first 8 chars of the id.
    await expect(
      page.getByRole("heading", {
        name: new RegExp(`Заявка ${SAMPLE_APPLICATION_DETAIL.id.slice(0, 8)}`),
      })
    ).toBeVisible();

    // Sections — h2 headings.
    await expect(page.getByRole("heading", { name: /Этапы прохождения/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /Заявитель/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /Документы/i })).toBeVisible();

    // Applicant fields — render full name from mock.
    await expect(page.getByText(SAMPLE_APPLICATION_DETAIL.applicant.fullName)).toBeVisible();
  });

  test("status=collecting_documents shows 'Загрузить документ' button", async ({ page }) => {
    // SAMPLE_APPLICATION_DETAIL.state === "collecting_documents" (see mocks.ts).
    await page.goto(`/applications/${SAMPLE_APPLICATION_DETAIL.id}`);

    await expect(
      page.getByRole("button", { name: /Загрузить документ/i })
    ).toBeVisible();
  });
});
