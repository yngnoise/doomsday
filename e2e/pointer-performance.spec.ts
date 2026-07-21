import { expect, test } from "@playwright/test";

function liveDropID() {
  const value = process.env.E2E_LIVE_DROP_ID;
  if (!value) throw new Error("E2E_LIVE_DROP_ID was not initialized by global setup");
  return value;
}

test("custom pointer and product zoom update through compositor-friendly values", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto(`/drops/${liveDropID()}`);

  const photoArea = page.getByTestId("product-photo-area").filter({ visible: true });
  await expect(photoArea).toBeVisible();
  const bounds = await photoArea.boundingBox();
  expect(bounds).not.toBeNull();
  if (!bounds) return;

  await page.mouse.move(
    bounds.x + bounds.width * 0.75,
    bounds.y + bounds.height * 0.25,
  );

  const cursor = page.getByTestId("custom-cursor-crosshair");
  await expect(cursor).toBeVisible();
  await expect.poll(() => photoArea.evaluate((element) =>
    element.style.getPropertyValue("--zoom-origin"),
  )).toMatch(/^7\d(?:\.\d+)?% 2\d(?:\.\d+)?%$/);
});
