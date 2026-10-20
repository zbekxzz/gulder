import { expect, test, type Page } from "@playwright/test";

async function login(page: Page, email: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Пароль").fill("gulder-dev-password");
  await page.getByRole("button", { name: "Войти" }).click();
}

test("исследовательский режим: выключен по умолчанию, включается и отзывается одной кнопкой (RM-01, RM-02)", async ({
  page,
}) => {
  await login(page, "giver2@gulder.local");
  await page.getByRole("link", { name: "Настройки" }).click();
  await expect(page.getByText("выключен")).toBeVisible();
  await expect(page.getByText(/Версия текста: research-consent-v1/)).toBeVisible();
  await page.getByRole("button", { name: "Согласен участвовать" }).click();
  await expect(page.getByText("включён")).toBeVisible();
  await page.getByRole("button", { name: "Отозвать согласие" }).click();
  await expect(page.getByText("выключен")).toBeVisible();
});

test("менеджер: экспертная разметка и оценка моделей", async ({ page }) => {
  await login(page, "manager@gulder.local");
  await page.getByRole("tab", { name: "Экспертная разметка" }).click();
  await expect(page.getByRole("heading", { name: /Случай r\d+-c\d/ })).toBeVisible();
  await page.getByRole("tab", { name: "Оценка моделей" }).click();
  await expect(page.getByText(/make eval|Сравнение на split/).first()).toBeVisible();
  await page.getByRole("tab", { name: "Качество данных" }).click();
  await expect(page.getByText("Токсичность не сверена с ASPCA")).toBeVisible();
});
