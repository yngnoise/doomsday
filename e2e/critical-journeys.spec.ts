import { expect, test, type Page } from "@playwright/test";

const otpCode = "424242";
const adminPassword = process.env.ADMIN_PASSWORD ?? "e2e-admin-password-with-32-characters";

function fixture(name: "E2E_LIVE_DROP_ID" | "E2E_LIVE_DROP_NAME" | "E2E_SOLD_OUT_DROP_ID") {
  const value = process.env[name];
  if (!value) throw new Error(`${name} was not initialized by global setup`);
  return value;
}

async function signIn(page: Page, email: string) {
  await page.getByRole("button", { name: "Sign In" }).click();
  await page.getByPlaceholder("your@email.com").fill(email);
  await page.getByRole("button", { name: /Send Code/ }).click();
  await expect(page.getByText("Enter Access Code")).toBeVisible();
  await page.locator('input[inputmode="numeric"]').first().fill(otpCode);
  await expect(page.getByRole("button", { name: "Log out" })).toBeVisible();
}

async function reserveAndOpenCheckout(page: Page, email: string, size: string) {
  const liveDropID = fixture("E2E_LIVE_DROP_ID");
  await page.goto(`/drops/${liveDropID}`);
  await signIn(page, email);
  await page.getByRole("button", { name: new RegExp(`^${size}\\b`) }).click();
  await page.getByTestId("purchase-button").click();
  await expect(page.getByText("Item Secured")).toBeVisible();
  await page.getByRole("button", { name: /Proceed to Checkout/ }).click();
  await page.waitForURL(/\/checkout\//, { timeout: 20_000 });
  await expect(page.getByText("Payment Gateway Simulator")).toBeVisible();
  await expect(page.getByPlaceholder("your@email.com")).toHaveValue(email);
}

async function fillCheckout(page: Page) {
  await page.getByPlaceholder("John Doe").fill("E2E Customer");
  await page.getByPlaceholder("Street, City, Country, Postal Code").fill("42 Test Street, Test City");
}

test.describe.serial("critical customer journeys", () => {
  test.beforeEach(async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
  });

  test("OTP, reservation, approved payment, confirmation, and admin refund", async ({ page }) => {
    await reserveAndOpenCheckout(page, `approved-${Date.now()}@example.com`, "S");
    await fillCheckout(page);
    await page.getByRole("button", { name: "Approved" }).click();
    await page.getByRole("button", { name: /Simulate Payment/ }).click();
    await expect(page).toHaveURL(/\/confirmation\?/);
    await expect(page.getByRole("heading", { name: "SECURED" })).toBeVisible();

    await page.goto("/admin");
    await page.getByPlaceholder("Enter password").fill(adminPassword);
    await page.getByRole("button", { name: /Enter/ }).click();
    await page.getByRole("button", { name: "orders" }).click();
    const row = page.getByRole("row").filter({ hasText: fixture("E2E_LIVE_DROP_NAME") }).filter({ hasText: "paid" }).first();
    await expect(row).toBeVisible();
    page.once("dialog", dialog => dialog.accept());
    await row.getByRole("button", { name: "Refund" }).click();
    const refundedRow = page.getByRole("row").filter({ hasText: fixture("E2E_LIVE_DROP_NAME") }).first();
    await expect(refundedRow).toContainText("refunded");
  });

  test("declined payment can be retried successfully", async ({ page }) => {
    await reserveAndOpenCheckout(page, `retry-${Date.now()}@example.com`, "M");
    await fillCheckout(page);
    await page.getByRole("button", { name: "Declined" }).click();
    await page.getByRole("button", { name: /Simulate Payment/ }).click();
    await expect(page.getByText(/simulated card was declined/i)).toBeVisible();
    await page.getByRole("button", { name: "Approved" }).click();
    await page.getByRole("button", { name: /Simulate Payment/ }).click();
    await expect(page).toHaveURL(/\/confirmation\?/);
  });

  test("expired reservation is blocked before payment", async ({ page, request }) => {
    await reserveAndOpenCheckout(page, `expired-${Date.now()}@example.com`, "L");
    const reservationID = new URL(page.url()).pathname.split("/").pop()!;

    const login = await request.post("http://127.0.0.1:8080/api/admin/login", { data: { password: adminPassword } });
    expect(login.ok()).toBeTruthy();
    const { token } = await login.json() as { token: string };
    const expire = await request.post(`http://127.0.0.1:8080/api/admin/test/reservations/${reservationID}/expire`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(expire.ok()).toBeTruthy();
    const { expires_at } = await expire.json() as { expires_at: string };

    await page.goto(`/checkout/${reservationID}?drop=${fixture("E2E_LIVE_DROP_ID")}&expires=${encodeURIComponent(expires_at)}`);
    await expect(page.getByText("EXPIRED", { exact: true })).toBeVisible();
  });

  test("verified user can join a sold-out waitlist", async ({ page }) => {
    await page.goto(`/drops/${fixture("E2E_SOLD_OUT_DROP_ID")}`);
    await signIn(page, `waitlist-${Date.now()}@example.com`);
    await page.getByRole("button", { name: "Join Waitlist" }).click();
    await expect(page.getByText("Waitlist Position")).toBeVisible();
    await expect(page.getByText("#1", { exact: true })).toBeVisible();
  });
});
