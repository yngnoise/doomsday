import { chromium } from "@playwright/test";
import { mkdir } from "node:fs/promises";

const baseURL = process.env.SCREENSHOT_BASE_URL ?? "http://127.0.0.1:3000";
const dropID = process.env.SCREENSHOT_DROP_ID ?? "dmsdy-ss25-001";
const otpCode = process.env.E2E_OTP_CODE ?? "424242";

await mkdir("docs/images", { recursive: true });

const browser = await chromium.launch();
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  colorScheme: "dark",
  reducedMotion: "reduce",
  deviceScaleFactor: 1,
});
const page = await context.newPage();

try {
  await page.goto(`${baseURL}/drops`, { waitUntil: "domcontentloaded" });
  await page.getByRole("heading", { name: "ARCHIVE" }).waitFor();
  await page.getByText("WRAITH FIELD JACKET", { exact: true }).first().waitFor();
  await page.getByRole("button", { name: /WRAITH FIELD JACKET/ }).hover();
  await page.waitForTimeout(2_000);
  await page.screenshot({ path: "docs/images/storefront.png", fullPage: true });

  await page.goto(`${baseURL}/drops/${dropID}`, { waitUntil: "domcontentloaded" });
  await page.getByRole("button", { name: "Sign In" }).waitFor();
  await page.getByRole("button", { name: "Sign In" }).click();
  await page.getByPlaceholder("your@email.com").fill(`portfolio-${Date.now()}@example.com`);
  await page.getByRole("button", { name: /Send Code/ }).click();
  await page.locator('input[inputmode="numeric"]').first().fill(otpCode);
  await page.getByRole("button", { name: "Log out" }).waitFor();
  await page.getByRole("button", { name: /^M\b/ }).click();
  await page.getByTestId("purchase-button").click();
  await page.getByText("Item Secured").waitFor();
  await page.getByRole("button", { name: /Proceed to Checkout/ }).click();
  await page.waitForURL(/\/checkout\//);
  await page.getByPlaceholder("John Doe").fill("Portfolio Reviewer");
  await page.getByPlaceholder("Street, City, Country, Postal Code").fill("42 Test Street, Test City");
  await page.screenshot({ path: "docs/images/checkout.png", fullPage: true });
} finally {
  await browser.close();
}
