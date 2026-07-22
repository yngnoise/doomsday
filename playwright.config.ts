import { defineConfig, devices } from "@playwright/test";

const e2eAdminPassword = process.env.ADMIN_PASSWORD ?? "e2e-admin-password-with-32-characters";
const e2eAPIOrigin = `http://127.0.0.1:${process.env.E2E_API_PORT ?? "8080"}`;
const e2eWebOrigin = `http://127.0.0.1:${process.env.E2E_WEB_PORT ?? "3000"}`;

export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [["line"], ["html", { open: "never" }]]
    : [["list"], ["html", { open: "never" }]],
  globalSetup: "./e2e/global-setup.ts",
  use: {
    baseURL: e2eWebOrigin,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "mobile-accessibility",
      testMatch: /accessibility\.spec\.ts/,
      use: { ...devices["Pixel 5"] },
    },
  ],
  webServer: [
    {
      command: "go run .",
      url: `${e2eAPIOrigin}/api/drops`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        APP_ENV: "test",
        PORT: process.env.E2E_API_PORT ?? "8080",
        SITE_URL: e2eWebOrigin,
        CORS_ORIGINS: e2eWebOrigin,
        JWT_SECRET: process.env.JWT_SECRET ?? "e2e-jwt-secret-with-at-least-32-characters",
        PAYMENT_WEBHOOK_SECRET: process.env.PAYMENT_WEBHOOK_SECRET ?? "e2e-payment-secret-with-at-least-32-characters",
        ADMIN_PASSWORD: e2eAdminPassword,
        E2E_OTP_CODE: "424242",
        ...(process.env.DATABASE_URL ? { DATABASE_URL: process.env.DATABASE_URL } : {}),
        ...(process.env.REDIS_URL ? { REDIS_URL: process.env.REDIS_URL } : {}),
      },
    },
    {
      command: `npm run dev -- --hostname 127.0.0.1 --port ${process.env.E2E_WEB_PORT ?? "3000"}`,
      url: `${e2eWebOrigin}/drops`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        API_ORIGIN: e2eAPIOrigin,
      },
    },
  ],
});
