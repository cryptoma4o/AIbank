import { expect, test } from "@playwright/test";
import { installMocks, NEW_APPLICATION_ID } from "./fixtures/mocks";
import { seedAuth } from "./fixtures/auth";

// Этап 3 — AML-сведения о деятельности.  Дёргает mutation
// submitApplicationActivity и редиректит на этап 4.

test.describe("/applications/[id]/wizard/3", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);
  });

  test("happy path: заполняет поля и сохраняет, переход к этапу 4", async ({
    page,
  }) => {
    await page.goto(`/applications/${NEW_APPLICATION_ID}/wizard/3`);

    // Wizard progress видим, текущий шаг — 3.
    await expect(page.getByTestId("wizard-progress")).toBeVisible();
    await expect(page.getByTestId("stage-3-form")).toBeVisible();

    // Описание бизнеса — минимум 20 символов.
    await page
      .getByLabel(/Описание деятельности/i)
      .fill("Оптовая торговля канцелярскими товарами B2B по РФ.");

    // Категория ОКВЭД — по умолчанию low.  Выбираем medium.
    await page.getByLabel(/Средний риск/i).check();

    // Операционная модель.
    await page.getByLabel(/Месячный оборот \(план\)/i).fill("5000000");
    await page.getByLabel(/Валюта мес\. оборота/i).fill("RUB");
    await page.getByLabel(/Годовой оборот \(план\)/i).fill("60000000");
    await page.getByLabel(/Валюта год\. оборота/i).fill("RUB");
    await page.getByLabel(/Доля наличных, %/i).fill("5");
    await page
      .getByLabel(/География операций \(ISO 3166, через запятую\)/i)
      .fill("RU, KZ");

    // Источник средств.
    await page.getByLabel(/^Выручка$/).check();
    await page
      .getByLabel(/^Обоснование$/)
      .fill("Поступления от оптовых продаж по договорам поставки.");

    await page
      .getByRole("button", { name: /Сохранить и перейти к этапу 4/i })
      .click();

    await page.waitForURL(
      `**/applications/${NEW_APPLICATION_ID}/wizard/4`,
      { timeout: 10_000 }
    );
    await expect(page).toHaveURL(
      new RegExp(`/applications/${NEW_APPLICATION_ID}/wizard/4$`)
    );
  });

  test("validation: пустое описание блокирует сохранение", async ({ page }) => {
    await page.goto(`/applications/${NEW_APPLICATION_ID}/wizard/3`);
    await expect(page.getByTestId("stage-3-form")).toBeVisible();

    // Никаких полей не заполняем, жмём submit.
    await page
      .getByRole("button", { name: /Сохранить и перейти к этапу 4/i })
      .click();

    // Сообщение об ошибке описания бизнеса.
    await expect(
      page.getByText(/Опишите бизнес не короче 20 символов/i).first()
    ).toBeVisible();

    // URL не изменился.
    await expect(page).toHaveURL(
      new RegExp(`/applications/${NEW_APPLICATION_ID}/wizard/3$`)
    );
  });
});
