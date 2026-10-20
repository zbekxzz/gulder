import { expect, test } from "@playwright/test";

test("главная страница открывается", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Gulder" })).toBeVisible();
});

test("API отвечает /readyz", async ({ request }) => {
  const res = await request.get("/healthz");
  expect(res.ok()).toBeTruthy();
});
