import { test, expect } from "@playwright/test";

// Browser source-only memory-budget oracles: no real backend, user or artifact.
const id = c => "job-" + c.repeat(64);

test("oversize announced length denies without showing PDF or download", async({page})=>{
  await page.goto("/viewer?job_id="+id("e"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
});

test("missing length with over-limit chunked body is capped and denied",async({page})=>{
  const requests=[];
  page.on("request",req=>requests.push(req.url()));
  await page.goto("/viewer?job_id="+id("8"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
  expect(requests.every(u=>new URL(u).origin==="http://127.0.0.1:18763")).toBe(true);
});

test("normal bounded synthetic PDF remains readable",async({page})=>{
  await page.goto("/viewer?job_id="+id("a"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","ready");
  await expect(page.locator("#download-original")).toBeVisible();
});
