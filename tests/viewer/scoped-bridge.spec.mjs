import { test, expect } from "@playwright/test";

const id = char => "job-" + char.repeat(64);

// Entire fixture is a disposable localhost-only fake document renderer.
// It is NOT FolioRelay's real, actor-authorized API, and cannot prove AAA.
test("fixed scoped preview URL renders the right synthetic PDF in five browsers", async ({page}) => {
  const requested = [];
  page.on("request", req => requested.push(req.url()));
  await page.goto("/viewer?job_id=" + id("a") + "&url=https://steal.invalid/example.pdf");
  await expect(page.locator("#preview")).toHaveAttribute("data-state","ready");
  await expect(page.locator("#text-result")).toContainText("FolioRelay SYNTHETIC");
  await expect(page.locator("#download-original")).toHaveAttribute(
    "href","/api/v1/jobs/" + id("a") + "/artifact"
  );
  await expect(page.locator("#download-original")).toBeVisible();
  expect(requested.some(x => x.includes("/api/v1/jobs/" + id("a") + "/preview"))).toBe(true);
  expect(requested.every(x => x.startsWith("http://127.0.0.1:18763/"))).toBe(true);
  expect(requested.some(x => x.includes("steal.invalid"))).toBe(false);
});

for (const [character,reason] of [["b","denied"],["c","missing"],["d","corrupt"]]) {
  test("scoped " + reason + " document never shows a page or original download", async ({page}) => {
    await page.goto("/viewer?job_id=" + id(character));
    await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
    await expect(page.locator("#page-canvas")).toBeHidden();
    await expect(page.locator("#download-original")).toBeHidden();
  });
}

for (const query of [
  "",
  "?job_id=../../other",
  "?job_id=" + encodeURIComponent("job-" + "a".repeat(63) + String.fromCharCode(10)),
  "?job_id=" + id("A"),
  "?url=https://steal.invalid/example.pdf"
]) {
  test("malformed/absent ID denies without preview fetch: " + query.slice(0,40), async ({page}) => {
    const requests = [];
    page.on("request",req => requests.push(req.url()));
    await page.goto("/viewer" + query);
    await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
    await expect(page.locator("#download-original")).toBeHidden();
    expect(requests.some(x => x.includes("/api/v1/jobs/"))).toBe(false);
    expect(requests.some(x => x.includes("steal.invalid"))).toBe(false);
  });
}
