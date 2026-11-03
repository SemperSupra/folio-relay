import { defineConfig, devices } from "@playwright/test";

const baseURL = "http://127.0.0.1:18763";
export default defineConfig({
  testDir: "../../tests/viewer",
  outputDir: "../../build/viewer-results",
  timeout: 45_000,
  expect: { timeout: 15_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [["line"], ["html", { outputFolder: "../../build/viewer-report", open: "never" }]]
    : "list",
  use: {
    baseURL, trace: "retain-on-failure", screenshot: "only-on-failure",
    video: "retain-on-failure", serviceWorkers: "block"
  },
  webServer: {
    command: "node experiments/viewer/server.mjs",
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 20_000
  },
  projects: [
    {name: "desktop-chromium", use: {...devices["Desktop Chrome"]}},
    {name: "desktop-firefox", use: {...devices["Desktop Firefox"]}},
    {name: "desktop-webkit", use: {...devices["Desktop Safari"]}},
    {name: "android-chromium", use: {...devices["Pixel 5"]}},
    {name: "iphone-webkit", use: {...devices["iPhone 13"]}}
  ]
});
