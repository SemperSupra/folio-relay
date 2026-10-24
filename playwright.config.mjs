import { defineConfig, devices } from '@playwright/test';

const externalBaseURL = process.env.FOLIORELAY_WEBUI_BASE_URL;
const baseURL = externalBaseURL || 'http://127.0.0.1:18080/';

export default defineConfig({
  testDir: './tests/webui',
  outputDir: 'build/playwright-results',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [['line'], ['html', { outputFolder: 'build/playwright-report', open: 'never' }]]
    : 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: externalBaseURL
    ? undefined
    : {
        command: 'bash scripts/webui/start-browser-test-server.sh',
        url: baseURL,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
      },
  projects: [
    { name: 'desktop-chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'desktop-firefox', use: { ...devices['Desktop Firefox'] } },
    { name: 'desktop-webkit', use: { ...devices['Desktop Safari'] } },
    { name: 'android-chromium', use: { ...devices['Pixel 5'] } },
    { name: 'iphone-webkit', use: { ...devices['iPhone 13'] } },
  ],
});
