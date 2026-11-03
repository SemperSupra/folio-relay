import { test, expect } from '@playwright/test';

// Source-only contract tests. The installed backend does NOT advertise a
// scoped preview capability. Mocking this response does not grant authority.
const TOKEN = process.env.FOLIORELAY_TEST_TOKEN || '0123456789abcdef0123456789abcdef';
const validID = 'job-' + 'a'.repeat(64);
const attackTitle = 'Invoice <img src=x onerror=alert(1)>';

async function inbox(page, job) {
  await page.goto('/');
  await page.getByLabel('Management credential').fill(TOKEN);
  await page.getByRole('button', {name:'Sign in'}).click();
  await expect(page.getByRole('button', {name:'Sign out'})).toBeVisible();
  await page.route('**/api/v1/jobs?limit=100', route => route.fulfill({
    status:200,
    contentType:'application/json',
    body:JSON.stringify({items:[job],next_cursor:null})
  }));
  await page.getByRole('button', {name:'Inbox'}).click();
  await expect(page.locator('#jobs .job')).toHaveCount(1);
}

function job(override={}) {
  return {
    job_id:validID, source_title:attackTitle,
    download_name:'invoice.pdf', media_type:'application/pdf',
    artifact_bytes:100, accepted_at:'2026-11-03T07:00:00Z',
    state:'accepted', ...override
  };
}
const ready = {state:'available',kind:'pdf',contract:'scoped-v1',url:'https://attacker.invalid/steal'};

test('source-only Inbox keeps preview absent without explicit scoped capability',async({page})=>{
  await inbox(page,job());
  await expect(page.locator('#jobs a')).toHaveCount(1);
  await expect(page.locator('#jobs a')).toHaveText('Download');
  await expect(page.locator('#jobs strong')).toHaveText(attackTitle);
  await expect(page.locator('#jobs img')).toHaveCount(0);
});
test('approved synthetic capability renders only fixed local path, never untrusted URL',async({page})=>{
  await inbox(page,job({preview:ready}));
  const view=page.getByRole('link',{name:'View PDF: '+attackTitle});
  await expect(view).toHaveCount(1);
  await expect(view).toHaveAttribute('href','/viewer?job_id='+validID);
  await expect(page.locator('#jobs a')).toHaveCount(2);
  await expect(page.locator('#jobs img')).toHaveCount(0);
  const target=await view.getAttribute('href');
  expect(new URL(target,page.url()).origin).toBe(new URL(page.url()).origin);
  expect(target).not.toContain('attacker.invalid');
});
for(const [name,change] of [
  ['URF cannot be shown as unsupported inline PDF',{media_type:'image/urf',preview:ready}],
  ['forged ID cannot get preview',{job_id:'job-../../secret',preview:ready}],
  ['contract must match',{preview:{...ready,contract:'untrusted'}}],
  ['pending is not ready',{preview:{...ready,state:'pending'}}],
  ['unsupported arbitrary HTML has no inline renderer',{media_type:'text/html',preview:ready}]
]) {
  test(name,async({page})=>{
    await inbox(page,job(change));
    await expect(page.locator('#jobs a')).toHaveCount(1);
    await expect(page.locator('#jobs strong')).toHaveText(attackTitle);
    await expect(page.locator('#jobs img')).toHaveCount(0);
  });
}
