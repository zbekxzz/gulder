import { expect, test } from "@playwright/test";

test("без входа открывается страница входа", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("heading", { name: "Вход" })).toBeVisible();
});

test("API отвечает /healthz", async ({ request }) => {
  const res = await request.get("/healthz");
  expect(res.ok()).toBeTruthy();
});
