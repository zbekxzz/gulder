import { expect, test, type Page } from "@playwright/test";

// Сквозной сценарий из docs/walkthrough.md (DoD §19): два получателя с одним бюджетом → разные Top-5 →
// уточняющий вопрос → наборы ассистента → исключение позиции → «Рядом» на карте →
// флорист пытается поставить лилии кошатнику → блок.

const PASSWORD = "gulder-dev-password";
const stamp = Date.now().toString(36);

async function login(page: Page, email: string) {
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Пароль").fill(PASSWORD);
  await page.getByRole("button", { name: "Войти" }).click();
}

async function logout(page: Page) {
  await page.getByRole("button", { name: "Выйти" }).click();
  await expect(page).toHaveURL(/\/login$/);
}

async function createRecipient(page: Page, name: string) {
  await page.getByRole("link", { name: "Получатели" }).click();
  await page.getByLabel("Имя получателя").fill(name);
  await page.getByRole("button", { name: "Добавить получателя" }).click();
  await expect(page.getByRole("heading", { name })).toBeVisible();
}

/** Ставит «Знаю» и выбирает варианты в поле профиля. */
async function know(page: Page, field: string, chips: string[] = []) {
  const group = page.getByRole("group", { name: field, exact: true });
  await group.getByRole("radio", { name: "Знаю", exact: true }).click();
  for (const c of chips) await group.getByRole("button", { name: c, exact: true }).click();
}

async function recommend(page: Page): Promise<string[]> {
  await page.getByLabel("Бюджет до, ₸").fill("40000");
  await page.getByRole("button", { name: "Подобрать букеты" }).click();
  await expect(page.getByRole("heading", { name: "Подборка букетов" })).toBeVisible();
  const names = page.getByTestId("card-name");
  await expect(names.first()).toBeVisible();
  return names.allInnerTexts();
}

/** Скриншоты шагов для ревью интерфейса: E2E_SCREENSHOTS=<каталог>. */
async function shot(page: Page, name: string) {
  const dir = process.env.E2E_SCREENSHOTS;
  if (dir) await page.screenshot({ path: `${dir}/${name}.png`, fullPage: true });
}

test.use({ geolocation: { latitude: 43.2531, longitude: 76.9031 }, permissions: ["geolocation"] });

test("сквозной сценарий: от двух получателей до блокировки опасной замены", async ({ page }) => {
  await login(page, "giver@gulder.local");
  await expect(page.getByRole("heading", { name: "Получатели" })).toBeVisible();

  // 1. Первый получатель: любит лилии, ограничений нет
  await createRecipient(page, `Сестра ${stamp}`);
  await know(page, "Аллергии");
  await know(page, "Домашние животные");
  await know(page, "Любимые цветы", ["Лилии"]);
  await page.getByRole("button", { name: "Сохранить профиль" }).click();
  await expect(page.getByText("Профиль сохранён")).toBeVisible();
  const topFree = await recommend(page);

  // 2. Второй получатель: тот же вкус, но дома кошка; остальное неизвестно
  await createRecipient(page, `Кошатница ${stamp}`);
  await know(page, "Домашние животные", ["Кошка"]);
  await know(page, "Любимые цветы", ["Лилии"]);
  await page.getByRole("button", { name: "Сохранить профиль" }).click();
  await expect(page.getByText("Профиль сохранён")).toBeVisible();
  const topCat = await recommend(page);
  expect(topCat).not.toEqual(topFree); // AC-1: один бюджет — разные Top-5
  const occasionId = (await page.getByTestId("occasion-id").innerText()).replace(/\D/g, "");
  await shot(page, "01-top5-cat-owner");

  // 3. Уточняющий вопрос: ответ обновляет профиль и пересчитывает подборку
  const question = page.getByTestId("clarifying-question");
  await expect(question).toBeVisible();
  const before = page.url();
  await page.locator("section").filter({ has: question }).getByRole("button").first().click();
  await expect(page).not.toHaveURL(before);
  await expect(page.getByTestId("card-name").first()).toBeVisible();

  // 4. Набор «букет + подарки» от ассистента и исключение позиции
  await page.getByRole("button", { name: "Собрать наборы" }).click();
  const sets = page.getByTestId("gift-set");
  await expect(sets).toHaveCount(3);
  const firstSet = sets.first();
  const removable = firstSet.getByRole("button", { name: /^Убрать (?!💐)/ });
  const removedName = ((await removable.first().getAttribute("aria-label")) ?? "").replace(
    /^Убрать /,
    "",
  );
  await removable.first().click();
  await expect(firstSet.getByText(removedName, { exact: true })).toHaveCount(0);
  await shot(page, "02-gift-sets");

  // 5. «Рядом»: геолокация по клику, расстояние на карточках, карта магазинов
  await page.getByRole("button", { name: "Рядом" }).click();
  await expect(page.getByTestId("shops-map")).toBeVisible();
  await expect(page.getByText(/км ·/).first()).toBeVisible();
  await expect(page.getByTestId("card-name").first()).toBeVisible();
  await shot(page, "03-nearby");

  await logout(page);

  // 6. Флорист пытается поставить лилии кошатнице → блок (AC-3)
  await login(page, "florist@gulder.local");
  await page.getByRole("tab", { name: "Замены" }).click();
  await page
    .getByRole("combobox", { name: "Букет" })
    .selectOption({ label: "AB-001 — Классика: 25 красных роз" });
  await page.getByLabel("Повод").fill(occasionId);
  await page
    .getByRole("combobox", { name: "Заменить цветок" })
    .selectOption({ label: "Роза Red Naomi × 25" });
  await page
    .getByRole("combobox", { name: "На цветок" })
    .selectOption({ label: "Лилия восточная Siberia" });
  await expect(page.getByTestId("substitution-result")).toContainText("Замена запрещена");
  await expect(page.getByRole("button", { name: "Применить замену" })).toBeDisabled();
  await shot(page, "04-substitution-blocked");

  // API всё равно проверяет: прямой запрос получает 409
  const token = await page.evaluate(
    () => JSON.parse(localStorage.getItem("gulder.session") ?? "{}").access_token as string,
  );
  const ids = await page.evaluate(async (t) => {
    const res = await fetch("/api/v1/catalog/bouquets", {
      headers: { Authorization: `Bearer ${t}` },
    });
    const body = (await res.json()) as {
      items: { sku: string; id: number; items: { flower_id: number }[] }[];
    };
    const b = body.items.find((x) => x.sku === "AB-001")!;
    const flowers = (await (
      await fetch("/api/v1/catalog/flowers", { headers: { Authorization: `Bearer ${t}` } })
    ).json()) as { items: { code: string; id: number }[] };
    const lily = flowers.items.find((f) => f.code === "lily_white")!;
    return { bouquet: b.id, from: b.items[0]!.flower_id, to: lily.id };
  }, token);
  const res = await page.request.post(`/api/v1/bouquets/${ids.bouquet}/substitutions/validate`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { from_flower_id: ids.from, to_flower_id: ids.to, occasion_id: Number(occasionId) },
  });
  expect(res.status()).toBe(409);
});
