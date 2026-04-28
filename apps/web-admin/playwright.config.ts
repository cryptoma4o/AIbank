// Playwright config for web-admin smoke tests.
//
// Admin app is an SSR Next.js app (port 3001) calling bff-admin via REST/GraphQL.
// We mock all backend traffic via Playwright route interception.

import { defineConfig, devices } from "@playwright/test";

const PORT = 3001;
const BASE_URL = `http://localhost:${PORT}`;

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  retries: 0,
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
      NEXT_PUBLIC_BFF_ADMIN_URL: `${BASE_URL}/__mock__/graphql`,
      NEXT_PUBLIC_IDENTITY_SERVICE_URL: `${BASE_URL}/__mock__/identity`,
    },
  },
});
