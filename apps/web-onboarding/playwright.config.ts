// Playwright config for web-onboarding e2e tests.
//
// Tests run against MOCKED backends (msw + Playwright route interception).
// `webServer` boots `npm run dev` on :3000 if not already running.
// To execute: `npm run e2e` (requires `npx playwright install` first).

import { defineConfig, devices } from "@playwright/test";

const PORT = 3000;
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  retries: 0, // mocks are deterministic — no retries needed
  workers: process.env.CI ? 2 : undefined,
  reporter: "list",
  forbidOnly: !!process.env.CI,
  timeout: 30_000,
  expect: { timeout: 5_000 },

  use: {
    baseURL: BASE_URL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    actionTimeout: 10_000,
    // Tests stub backend URLs via env so MSW/route interception works.
    extraHTTPHeaders: {},
  },

  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],

  webServer: {
    command: "npm run dev",
    port: PORT,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: {
      NEXT_PUBLIC_BFF_ONBOARDING_URL: `${BASE_URL}/__mock__/graphql`,
      NEXT_PUBLIC_IDENTITY_SERVICE_URL: `${BASE_URL}/__mock__/identity`,
    },
  },
});
