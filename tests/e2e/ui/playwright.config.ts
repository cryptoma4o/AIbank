// Playwright config для E2E UI-тестов AIbank против live-стека.
//
// В отличие от apps/web-onboarding/playwright.config.ts (моки + webServer),
// здесь стек уже работает (docker-compose up), а тесты — smoke против реальных
// 3000/3001/8082/8091/8092.
//
// Окружение:
//   E2E_BASE_URL          — http://localhost (default) или http://206.204.106.28
//   E2E_ONBOARDING_PORT   — 3000 (default)
//   E2E_ADMIN_PORT        — 3001 (default)

import { defineConfig, devices } from "@playwright/test";

const baseHost = (process.env.E2E_BASE_URL || "http://localhost").replace(/\/$/, "");
const onboardingPort = process.env.E2E_ONBOARDING_PORT || "3000";
const adminPort = process.env.E2E_ADMIN_PORT || "3001";

export default defineConfig({
  testDir: "./specs",
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.PLAYWRIGHT_JSON_OUTPUT_NAME ? "json" : "list",
  forbidOnly: !!process.env.CI,
  timeout: 30_000,
  expect: { timeout: 5_000 },

  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    actionTimeout: 10_000,
    ignoreHTTPSErrors: true,
  },

  projects: [
    {
      name: "onboarding",
      testMatch: /applicant-.*\.spec\.ts|cross-flow\.spec\.ts/,
      use: {
        baseURL: `${baseHost}:${onboardingPort}`,
        ...devices["Desktop Chrome"],
      },
    },
    {
      name: "admin",
      testMatch: /admin-.*\.spec\.ts/,
      use: {
        baseURL: `${baseHost}:${adminPort}`,
        ...devices["Desktop Chrome"],
      },
    },
  ],
});
