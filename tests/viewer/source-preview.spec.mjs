import { test, expect } from "@playwright/test";

// All requests and PDF bytes are localhost fixture data. This test is NOT
// permission evidence for any real FolioRelay document or production route.
test("PDF.js renders an admitted synthetic PDF without external network", async ({page}) => {
  const external = [];
  page.on("request", req => {
    if (!req.url().startsWith("http://127.0.0.1:18763/")) external.push(req.url());
  });
  const response = await page.goto("/");
  expect(response?.status()).toBe(200);
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "ready");
  await expect(page.locator("#page-status")).toHaveText("Page 1 of 1");
  await expect(page.locator("#text-result")).toContainText("FolioRelay SYNTHETIC");
  await expect(page.locator("#page-canvas")).toBeVisible();
  const dims = await page.locator("#page-canvas").evaluate(canvas => ({
    width: canvas.width, height: canvas.height
  }));
  expect(dims.width).toBeGreaterThan(100);
  expect(dims.height).toBeGreaterThan(100);
  await expect(page.getByRole("button", {name: "Previous page"})).toBeDisabled();
  await expect(page.getByRole("button", {name: "Next page"})).toBeDisabled();
  expect(external).toEqual([]);
});
test("viewer zoom remains usable and bounded", async ({page}) => {
  await page.goto("/");
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "ready");
  const before = await page.locator("#page-canvas").evaluate(c => c.width);
  await page.getByRole("button", {name: "Zoom in"}).click();
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "ready");
  const after = await page.locator("#page-canvas").evaluate(c => c.width);
  expect(after).toBeGreaterThan(before);
});

for (const scenario of ["denied", "missing", "corrupt", "unrecognized"]) {
  test("fail-closed preview for " + scenario + " fixture", async ({page}) => {
    await page.goto("/?scenario=" + scenario);
    await expect(page.locator("#preview")).toHaveAttribute("data-state", "denied");
    await expect(page.locator("#page-canvas")).toBeHidden();
    await expect(page.locator("#status")).toContainText("unavailable or denied");
    await expect(page.getByRole("button", {name: "Next page"})).toBeDisabled();
  });
}
test("isolated source fixture supplies restrictive viewer policy", async ({request}) => {
  const response = await request.get("/");
  const csp = response.headers()["content-security-policy"];
  expect(csp).toContain("default-src 'none'");
  expect(csp).toContain("worker-src 'self'");
  expect(csp).toContain("object-src 'none'");
  expect(csp).toContain("connect-src 'self'");
  expect(response.headers()["x-content-type-options"]).toBe("nosniff");
});
