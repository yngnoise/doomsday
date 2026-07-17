import { defineConfig, devices } from "@playwright/test";

const e2eAdminPassword = process.env.ADMIN_PASSWORD ?? "e2e-admin-password-with-32-characters";

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
    baseURL: "http://127.0.0.1:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: "go run .",
      url: "http://127.0.0.1:8080/api/drops",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        APP_ENV: "test",
        PORT: "8080",
        SITE_URL: "http://127.0.0.1:3000",
        CORS_ORIGINS: "http://127.0.0.1:3000",
        JWT_SECRET: process.env.JWT_SECRET ?? "e2e-jwt-secret-with-at-least-32-characters",
        PAYMENT_WEBHOOK_SECRET: process.env.PAYMENT_WEBHOOK_SECRET ?? "e2e-payment-secret-with-at-least-32-characters",
        ADMIN_PASSWORD: e2eAdminPassword,
        E2E_OTP_CODE: "424242",
        ...(process.env.DATABASE_URL ? { DATABASE_URL: process.env.DATABASE_URL } : {}),
        ...(process.env.REDIS_URL ? { REDIS_URL: process.env.REDIS_URL } : {}),
      },
    },
    {
      command: "npm run dev -- --hostname 127.0.0.1 --port 3000",
      url: "http://127.0.0.1:3000/drops",
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: {
        API_ORIGIN: "http://127.0.0.1:8080",
      },
    },
  ],
});
