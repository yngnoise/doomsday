import { expect, test } from "@playwright/test";

function fixture(name: "E2E_LIVE_DROP_ID" | "E2E_LIVE_DROP_NAME") {
  const value = process.env[name];
  if (!value) throw new Error(`${name} was not initialized by global setup`);
  return value;
}

test("archive and product content are present before hydration", async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false });
  const page = await context.newPage();

  try {
    await page.goto("/drops", { waitUntil: "domcontentloaded" });
    await expect(page.getByText(fixture("E2E_LIVE_DROP_NAME"), { exact: true })).toBeVisible();
    await expect(page.getByText("Loading drops…")).toHaveCount(0);

    await page.goto(`/drops/${fixture("E2E_LIVE_DROP_ID")}`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("heading", { level: 1, name: fixture("E2E_LIVE_DROP_NAME") })).toBeVisible();
    await expect(page.getByText("Loading drop…")).toHaveCount(0);
  } finally {
    await context.close();
  }
});
