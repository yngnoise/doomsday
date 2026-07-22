import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

function fixture(name: "E2E_LIVE_DROP_ID" | "E2E_LIVE_DROP_NAME") {
  const value = process.env[name];
  if (!value) throw new Error(`${name} was not initialized by global setup`);
  return value;
}

async function expectNoSeriousAxeViolations(page: Page, include?: string) {
  await page.waitForTimeout(750);
  let builder = new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]);
  if (include) builder = builder.include(include);
  const results = await builder.analyze();
  const blocking = results.violations.filter(({ impact }) => impact === "serious" || impact === "critical");
  expect(blocking, blocking.map(({ id, help, nodes }) => `${id}: ${help} (${nodes.length})`).join("\n")).toEqual([]);
}

test.describe("responsive accessibility", () => {
  test.beforeEach(async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
  });

  test("drop archive is keyboard operable and has no horizontal overflow", async ({ page }) => {
    await page.goto("/drops");
    const drop = page.getByRole("link", { name: new RegExp(fixture("E2E_LIVE_DROP_NAME"), "i") });
    await expect(drop).toBeVisible();
    await expect(page.getByTestId("custom-cursor-ring")).toHaveCount(0);
    expect(await drop.evaluate((element) => getComputedStyle(element).cursor)).not.toBe("none");
    await drop.focus();
    await expect(drop).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(new RegExp(`/drops/${fixture("E2E_LIVE_DROP_ID")}$`));
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(1);
  });

  test("archive and product page pass automated accessibility checks", async ({ page }) => {
    await page.goto("/drops");
    await expectNoSeriousAxeViolations(page);

    await page.goto(`/drops/${fixture("E2E_LIVE_DROP_ID")}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expectNoSeriousAxeViolations(page);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(1);
  });

  test("sign-in dialog traps focus and exposes errors accessibly", async ({ page }) => {
    await page.goto(`/drops/${fixture("E2E_LIVE_DROP_ID")}`);
    const trigger = page.getByRole("button", { name: "Sign In" });
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "Request Access" });
    await expect(dialog).toBeVisible();
    await expect(page.getByLabel("Email address")).toBeFocused();
    await expectNoSeriousAxeViolations(page, '[role="dialog"]');

    await page.keyboard.press("Shift+Tab");
    await expect(page.getByRole("button", { name: "Close sign-in dialog" })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(trigger).toBeFocused();
  });

  test("checkout and confirmation remain accessible without horizontal overflow", async ({ page }) => {
    await page.goto(`/drops/${fixture("E2E_LIVE_DROP_ID")}`);
    await page.getByRole("button", { name: "Sign In" }).click();
    await page.getByLabel("Email address").fill(`a11y-${test.info().project.name}-${Date.now()}@example.com`);
    await page.getByRole("button", { name: /Send Code/ }).click();
    await page.locator('input[inputmode="numeric"]').first().fill("424242");
    await expect(page.getByRole("button", { name: "Log out" })).toBeVisible();

    await page.getByRole("button", { name: /^XL,/ }).click();
    await page.getByTestId("purchase-button").click();
    await page.getByRole("button", { name: /Proceed to Checkout/ }).click();
    await expect(page).toHaveURL(/\/checkout\//);
    await page.getByLabel("Full Name").fill("Accessible Customer");
    await page.getByLabel("Full Address").fill("42 Test Street, Test City");
    await expectNoSeriousAxeViolations(page);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1);

    await page.getByRole("button", { name: /Simulate Payment/ }).click();
    await expect(page).toHaveURL(/\/confirmation\?/);
    await expectNoSeriousAxeViolations(page);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1);
  });
});
