import { defineConfig, devices } from "@playwright/test";

// E2E_BASE_URL задан → тесты идут по уже поднятому стеку (CI: docker compose).
// Иначе Playwright сам поднимает API (миграции + сид) и Vite на E2E_DATABASE_URL.
const external = process.env.E2E_BASE_URL;
const databaseURL =
  process.env.E2E_DATABASE_URL ??
  process.env.DATABASE_URL ??
  "postgres://gulder:gulder@localhost:5433/gulder?sslmode=disable";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: external ?? "http://localhost:5173",
    locale: "ru-RU",
    timezoneId: "Asia/Almaty",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: external
    ? undefined
    : [
        {
          command:
            "cd ../backend && go run ./cmd/migrate up && go run ./cmd/seed -data ../data && go run ./cmd/api",
          url: "http://localhost:8080/readyz",
          env: { DATABASE_URL: databaseURL, APP_ENV: "test", LOG_LEVEL: "warn" },
          reuseExistingServer: !process.env.CI,
          timeout: 180_000,
        },
        {
          command: "npm run dev",
          url: "http://localhost:5173",
          reuseExistingServer: !process.env.CI,
          timeout: 60_000,
        },
      ],
});
