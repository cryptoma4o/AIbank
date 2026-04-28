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

  test("upload modal opens on click and shows drag-and-drop area", async ({ page }) => {
    await page.goto(`/applications/${SAMPLE_APPLICATION_DETAIL.id}`);

    await page.getByTestId("open-upload").click();

    // Модалка появилась
    await expect(page.getByRole("dialog", { name: /Загрузка документов/i })).toBeVisible();
    await expect(page.getByTestId("upload-dropzone")).toBeVisible();

    // Подсказка про допустимые форматы
    await expect(
      page.getByText(/PDF, PNG, JPEG или TIFF/i)
    ).toBeVisible();

    // Закрытие крестиком
    await page.getByLabel("Закрыть").click();
    await expect(page.getByRole("dialog")).not.toBeVisible();
  });

  test("upload modal validates file type client-side", async ({ page }) => {
    await page.goto(`/applications/${SAMPLE_APPLICATION_DETAIL.id}`);
    await page.getByTestId("open-upload").click();

    // Подсовываем .txt — должно отвергаться client-side validation.
    await page.getByTestId("upload-input").setInputFiles({
      name: "rejected.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("hello"),
    });

    await expect(page.getByText(/Неподдерживаемый формат/i)).toBeVisible();
  });
});
