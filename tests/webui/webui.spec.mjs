import { test, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';
import {
  assertKeyboardFocusPath,
  assertPrimaryTargets,
} from '../../.ci/static-hci-playwright.mjs';

const TOKEN = process.env.FOLIORELAY_TEST_TOKEN || '0123456789abcdef0123456789abcdef';

function runtimeErrors(page) {
  const errors = [];
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`));
  page.on('console', message => {
    if (message.type() === 'error') errors.push(`console: ${message.text()}`);
  });
  return errors;
}

async function assertVisibleStructure(page, label) {
  const title = (await page.title()).trim();
  expect(title.length, `${label} needs a descriptive title`).toBeGreaterThanOrEqual(4);

  const visibleH1 = page.locator('h1:visible');
  await expect(visibleH1, `${label} should expose one visible page-purpose H1`).toHaveCount(1);
  expect((await visibleH1.textContent())?.trim().length || 0).toBeGreaterThanOrEqual(3);

  const headings = await page.locator('h1:visible,h2:visible,h3:visible,h4:visible,h5:visible,h6:visible')
    .evaluateAll(nodes => nodes.map(node => Number(node.tagName.slice(1))));
  for (let index = 1; index < headings.length; index += 1) {
    expect(
      headings[index] - headings[index - 1],
      `${label} visible heading levels should not skip downward`,
    ).toBeLessThanOrEqual(1);
  }
}

async function assertAxe(page, label) {
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations, `${label} automated accessibility violations`).toEqual([]);
}

async function assertNoPageOverflow(page, label) {
  const geometry = await page.evaluate(() => ({
    innerWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(
    geometry.scrollWidth,
    `${label} must not create page-level horizontal overflow`,
  ).toBeLessThanOrEqual(geometry.innerWidth + 1);
}

async function login(page) {
  await page.goto('/');
  await page.getByLabel('Management credential').fill(TOKEN);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await expect(page.locator('#overall-status')).not.toHaveText('Checking…');
}

async function selectView(page, name) {
  await page.getByRole('button', { name }).click();
  await expect(page.locator(`#${name.toLowerCase()}`)).toBeVisible();
}

test('unauthenticated login is usable and accessible', async ({ page }) => {
  const errors = runtimeErrors(page);
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'Connect to FolioRelay' })).toBeVisible();
  await expect(page.getByLabel('Management credential')).toBeVisible();
  await assertVisibleStructure(page, 'login');
  await assertPrimaryTargets({
    page,
    expect,
    selector: '#login:not([hidden]) input, #login:not([hidden]) button',
    label: 'login',
  });
  await assertNoPageOverflow(page, 'login');
  await assertAxe(page, 'login');
  expect(errors).toEqual([]);
});

test('wrong credential fails closed without entering the app', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Management credential').fill('definitely-wrong-browser-test-token');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page.locator('#login-error')).toContainText('not accepted');
  await expect(page.locator('#app')).toBeHidden();
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeHidden();
});

test('authenticated Overview, Inbox, Printer, and Diagnostics journeys work', async ({ page }) => {
  const errors = runtimeErrors(page);
  await login(page);

  await expect(page.locator('#overall-status')).toHaveText('Ready');
  await expect(page.locator('#printer-uri')).toContainText('/printers/FolioRelay');
  await assertVisibleStructure(page, 'Overview');
  await assertAxe(page, 'Overview');

  await selectView(page, 'Inbox');
  await expect(page.getByRole('heading', { name: 'Inbox' })).toBeVisible();
  await assertVisibleStructure(page, 'Inbox');
  await assertAxe(page, 'Inbox');

  await selectView(page, 'Printer');
  await expect(page.getByRole('heading', { name: 'Printer' })).toBeVisible();
  await expect(page.locator('#printer-details')).toContainText('FolioRelay');
  await assertVisibleStructure(page, 'Printer');
  await assertAxe(page, 'Printer');

  await selectView(page, 'Diagnostics');
  await expect(page.getByRole('heading', { name: 'Diagnostics' })).toBeVisible();
  await expect(page.locator('#checks .check').first()).toBeVisible();
  await assertVisibleStructure(page, 'Diagnostics');
  await assertAxe(page, 'Diagnostics');

  const browserStorage = await page.evaluate(() => ({
    cookie: document.cookie,
    localStorage: Object.values(localStorage),
    sessionStorage: Object.values(sessionStorage),
    html: document.documentElement.outerHTML,
  }));
  expect(browserStorage.cookie).not.toContain(TOKEN);
  expect(browserStorage.localStorage.join('\n')).not.toContain(TOKEN);
  expect(browserStorage.sessionStorage.join('\n')).not.toContain(TOKEN);
  expect(browserStorage.html).not.toContain(TOKEN);

  await page.getByRole('button', { name: 'Sign out' }).click();
  await expect(page.getByRole('heading', { name: 'Connect to FolioRelay' })).toBeVisible();
  expect(errors).toEqual([]);
});

test('standalone controls meet the 44 CSS-pixel project target', async ({ page }) => {
  await page.goto('/');
  await assertPrimaryTargets({
    page,
    expect,
    selector: '#login:not([hidden]) input, #login:not([hidden]) button',
    label: 'login',
    minimumCssPixels: 44,
  });

  await login(page);
  for (const view of ['Overview', 'Inbox', 'Printer', 'Diagnostics']) {
    await selectView(page, view);
    await assertPrimaryTargets({
      page,
      expect,
      selector: '#logout:not([hidden]), #app:not([hidden]) .tabs button, #app:not([hidden]) .view:not([hidden]) button',
      label: view,
      minimumCssPixels: 44,
    });
  }
});

test('desktop keyboard traversal reaches every critical control without obscuration', async ({ page }, testInfo) => {
  test.skip(!testInfo.project.name.startsWith('desktop-'));

  await page.setViewportSize({ width: 320, height: 800 });
  await login(page);

  for (const view of ['Overview', 'Inbox', 'Printer', 'Diagnostics']) {
    await selectView(page, view);
    await assertKeyboardFocusPath({
      page,
      expect,
      selector: '#logout:not([hidden]), #app:not([hidden]) .tabs button, #app:not([hidden]) .view:not([hidden]) button',
      label: `${testInfo.project.name} ${view} at 320px`,
    });
  }
});

test('mobile contexts and 320px desktop reflow have no page-level overflow', async ({ page }, testInfo) => {
  if (testInfo.project.name.startsWith('desktop-')) {
    await page.setViewportSize({ width: 320, height: 800 });
  }

  await login(page);
  for (const view of ['Overview', 'Inbox', 'Printer', 'Diagnostics']) {
    await selectView(page, view);
    await assertNoPageOverflow(page, `${testInfo.project.name} ${view}`);
  }
});

test('system light and dark preferences both render distinct usable themes', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' });
  await login(page);
  const light = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);

  await page.emulateMedia({ colorScheme: 'dark' });
  const dark = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);

  expect(light).not.toBe(dark);
  await assertAxe(page, 'authenticated dark theme');
});
