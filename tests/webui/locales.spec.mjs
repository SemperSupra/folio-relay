import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

const TOKEN = process.env.FOLIORELAY_TEST_TOKEN || '0123456789abcdef0123456789abcdef';

test('public locale catalog is bounded, complete and language neutral', async ({ request }) => {
  const entries = {};
  for (const lang of ['en', 'de', 'th']) {
    const response = await request.get('/locales/' + lang);
    expect(response.status()).toBe(200);
    entries[lang] = await response.json();
    expect(Object.keys(entries[lang]).length).toBeGreaterThanOrEqual(65);
    expect(entries[lang]['login.title']).toBeTruthy();
    expect(entries[lang]['diagnostics.next_action']).toContain('{action}');
  }
  const sourceKeys = Object.keys(entries.en).sort();
  expect(Object.keys(entries.de).sort()).toEqual(sourceKeys);
  expect(Object.keys(entries.th).sort()).toEqual(sourceKeys);
  for (const attempt of ['fr', '..', 'th-TH', 'config', 'admin']) {
    expect((await request.get('/locales/' + encodeURIComponent(attempt))).status()).toBe(404);
  }
});

test('German and Thai login and incorrect credentials stay accessible', async ({ page }) => {
  await page.goto('/');
  const selector = page.locator('#language');
  await expect(selector).toBeVisible();
  await expect(selector).toHaveAttribute('aria-label', /Language|Sprache|ภาษา/);
  await selector.selectOption('de');
  await expect(page.locator('html')).toHaveAttribute('lang', 'de');
  await expect(page.getByRole('heading', { name: 'Mit FolioRelay verbinden' })).toBeVisible();
  await expect(page.getByLabel('Verwaltungszugangsdaten')).toBeVisible();
  await page.locator('#credential').fill('bad-disposable-synthetic-token');
  await page.locator('#login-form button[type=submit]').click();
  await expect(page.locator('#login-error')).toContainText('nicht akzeptiert');
  await expect(page.locator('#app')).toBeHidden();
  await expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);

  await selector.selectOption('th');
  await expect(page.locator('html')).toHaveAttribute('lang', 'th');
  await expect(page.getByRole('heading', { name: 'เชื่อมต่อกับ FolioRelay' })).toBeVisible();
  await expect(page.getByLabel('ข้อมูลรับรองสำหรับการจัดการ')).toBeVisible();
  await page.locator('#credential').fill('bad-disposable-synthetic-token');
  await page.locator('#login-form button[type=submit]').click();
  await expect(page.locator('#login-error')).toContainText('ไม่ถูกต้อง');
  await expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('locale changes preserve auth and do not rerun diagnostics', async ({ page }) => {
  let diagnosticPosts = 0;
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().includes('/api/v1/self-test')) {
      diagnosticPosts += 1;
    }
  });
  await page.goto('/');
  await page.locator('#language').selectOption('th');
  await page.locator('#credential').fill(TOKEN);
  await page.locator('#login-form button[type=submit]').click();
  await expect(page.locator('#overall-status')).toHaveText('พร้อมใช้งาน');
  await expect(page.locator('#status-detail')).toContainText('FolioRelay');
  const before = await page.locator('#printer-uri').textContent();

  await page.locator('button[data-view=inbox]').click();
  await expect(page.locator('#inbox h1')).toHaveText('กล่องขาเข้า');
  await expect(page.locator('#jobs')).toContainText('ยังไม่มีงานพิมพ์ที่รับแล้ว');

  await page.locator('button[data-view=printer]').click();
  await expect(page.locator('#printer h1')).toHaveText('เครื่องพิมพ์');
  await expect(page.locator('#printer-details')).toContainText('ตำแหน่ง');

  await page.locator('button[data-view=diagnostics]').click();
  await expect(page.locator('#diagnostics h1')).toHaveText('การวินิจฉัย');
  await expect(page.locator('#checks .check').first()).toContainText('ผ่าน');
  expect(diagnosticPosts).toBe(1);

  await page.locator('#language').selectOption('de');
  await expect(page.locator('html')).toHaveAttribute('lang', 'de');
  await expect(page.locator('#diagnostics h1')).toHaveText('Diagnose');
  await expect(page.locator('#checks .check').first()).toContainText('Bestanden');
  expect(diagnosticPosts).toBe(1);
  await expect(page.locator('#printer-uri')).toHaveText(before || '');
  await expect(page.locator('#logout')).toBeVisible();
  await expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});

test('language chooser is keyboard-sized and does not create horizontal overflow', async ({ page }) => {
  await page.goto('/');
  for (const lang of ['en','de','th']) {
    await page.locator('#language').selectOption(lang);
    const bounds = await page.locator('#language').boundingBox();
    expect(bounds?.height || 0).toBeGreaterThanOrEqual(44);
    const viewport = await page.evaluate(() => ({
      width: window.innerWidth, scrollWidth: document.documentElement.scrollWidth,
    }));
    expect(viewport.scrollWidth).toBeLessThanOrEqual(viewport.width + 1);
  }
});
