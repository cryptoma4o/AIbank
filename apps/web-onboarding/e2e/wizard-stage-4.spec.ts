import { expect, test } from "@playwright/test";
import { installMocks, NEW_APPLICATION_ID } from "./fixtures/mocks";
import { seedAuth } from "./fixtures/auth";

// Этап 4 — ЕИО и представители.  Дёргает mutation upsertRepresentative.

test.describe("/applications/[id]/wizard/4", () => {
  test.beforeEach(async ({ page, baseURL }) => {
    await seedAuth(page, { baseURL: baseURL! });
    await installMocks(page);
  });

  test("happy path: добавляет ЕИО и форма сбрасывается", async ({ page }) => {
    await page.goto(`/applications/${NEW_APPLICATION_ID}/wizard/4`);

    await expect(page.getByTestId("wizard-progress")).toBeVisible();
    await expect(page.getByTestId("stage-4-form")).toBeVisible();

    // Персональные данные.
    await page.getByLabel(/^Фамилия$/).fill("Иванов");
    await page.getByLabel(/^Имя$/).fill("Иван");
    await page.getByLabel(/^Отчество$/).fill("Иванович");
    await page.getByLabel(/^Дата рождения$/).fill("1980-05-10");
    await page.getByLabel(/Место рождения/i).fill("г. Москва");
    // Гражданство по умолчанию RU — оставляем.

    // Документ — паспорт РФ по умолчанию.
    await page.getByLabel(/^Серия$/).fill("4502");
    await page.getByLabel(/^Номер$/).fill("123456");
    await page.getByLabel(/Код подразделения/i).fill("770-001");
    await page.getByLabel(/^Дата выдачи$/).fill("2005-08-15");
    await page.getByLabel(/^Кем выдан$/).fill("ОВД района Тверской г. Москвы");

    // Полномочия — должность и основание по умолчанию (Генеральный директор / Устав).

    // ПДЛ выключен по умолчанию.

    await page
      .getByRole("button", { name: /Сохранить представителя/i })
      .click();

    // После сохранения форма сбрасывается (фамилия снова пустая).
    await expect(page.getByLabel(/^Фамилия$/)).toHaveValue("");
  });

  test("validation: пустая фамилия блокирует сохранение", async ({ page }) => {
    await page.goto(`/applications/${NEW_APPLICATION_ID}/wizard/4`);
    await expect(page.getByTestId("stage-4-form")).toBeVisible();

    // Заполняем только номер документа (минимум, чтобы пройти его проверку),
    // дату рождения и дату выдачи, остальное оставляем пустым.
    await page.getByLabel(/^Дата рождения$/).fill("1980-05-10");
    await page.getByLabel(/^Серия$/).fill("4502");
    await page.getByLabel(/^Номер$/).fill("123456");
    await page.getByLabel(/^Дата выдачи$/).fill("2005-08-15");
    await page.getByLabel(/^Кем выдан$/).fill("ОВД района Тверской");

    await page
      .getByRole("button", { name: /Сохранить представителя/i })
      .click();

    await expect(
      page.getByText(/Укажите фамилию/i).first()
    ).toBeVisible();

    // URL не изменился — мы остались на этапе 4.
    await expect(page).toHaveURL(
      new RegExp(`/applications/${NEW_APPLICATION_ID}/wizard/4$`)
    );
  });
});
