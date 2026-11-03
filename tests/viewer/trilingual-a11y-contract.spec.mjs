import { test, expect } from "@playwright/test";

// Accessibility and i18n contract for a disposable synthetic viewer only.
// Languages are UI controls, never authority, identity, or document URL input.
const id = c => "job-" + c.repeat(64);
const specimens = [
  { tag: "en", status: "Preview ready; this is a synthetic document.", previous: "Previous page", denied: "Preview unavailable or denied. No document pages were shown." },
  { tag: "de", status: "Vorschau bereit; dies ist ein synthetisches Dokument.", previous: "Vorherige Seite", denied: "Vorschau nicht verfügbar oder verweigert. Es wurden keine Dokumentseiten angezeigt." },
  { tag: "th", status: "ตัวอย่างพร้อมแสดง นี่คือเอกสารสังเคราะห์", previous: "หน้าก่อนหน้า", denied: "ไม่สามารถแสดงตัวอย่างหรือถูกปฏิเสธ ไม่มีการแสดงหน้าเอกสาร" }
];
for (const s of specimens) {
  test("synthetic "+s.tag+" locale labels ready state accessibly", async ({page}) => {
    const external=[];
    page.on("request",req => {if(new URL(req.url()).origin !== "http://127.0.0.1:18763")external.push(req.url());});
    await page.goto("/viewer?job_id="+id("a")+"&lang="+s.tag);
    await expect(page.locator("html")).toHaveAttribute("lang",s.tag);
    await expect(page.locator("#preview")).toHaveAttribute("data-state","ready");
    await expect(page.locator("#status")).toHaveText(s.status);
    await expect(page.getByRole("button",{name:s.previous})).toHaveCount(1);
    await expect(page.locator("#status")).toHaveAttribute("role","status");
    await expect(page.locator("#page-canvas")).toBeVisible();
    expect(external).toEqual([]);
  });
  test("synthetic "+s.tag+" denied state is spoken and shows no original", async ({page}) => {
    await page.goto("/viewer?job_id="+id("b")+"&lang="+s.tag);
    await expect(page.locator("html")).toHaveAttribute("lang",s.tag);
    await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
    await expect(page.locator("#status")).toHaveText(s.denied);
    await expect(page.locator("#status")).toHaveAttribute("aria-live","polite");
    await expect(page.locator("#page-canvas")).toBeHidden();
    await expect(page.locator("#download-original")).toBeHidden();
  });
}
test("unknown locale is safely reduced to English without HTML injection",async ({page})=>{
  await page.goto("/viewer?job_id="+id("a")+"&lang="+encodeURIComponent("<img src=x onerror=alert(1)>"));
  await expect(page.locator("html")).toHaveAttribute("lang","en");
  await expect(page.locator("#preview")).toHaveAttribute("data-state","ready");
  await expect(page.locator("#status")).toHaveText(specimens[0].status);
  expect(await page.locator("img").count()).toBe(0);
});
