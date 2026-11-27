import { defineConfig, devices } from "@playwright/test";

// Against a deployment: PLAYWRIGHT_BASE_URL=https://... npx playwright test
// Locally: starts `next dev` (needs DATABASE_URL and a core on CORE_API_URL; see tests/e2e/README.md).
const baseURL = process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000";
const local = !process.env.PLAYWRIGHT_BASE_URL;

export default defineConfig({
  testDir: "tests/e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: { baseURL, trace: "retain-on-failure" },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "phone", use: { ...devices["Pixel 7"] } },
  ],
  webServer: local
    ? {
        command: "npm run dev",
        url: "http://localhost:3000/status",
        reuseExistingServer: true,
        timeout: 120_000,
        env: { EPS_DEMO_SIGNIN: "on", EPS_DEMO_SIGNIN_PER_MINUTE: "200", EPS_MAGIC_LINK_PER_MINUTE: "200", ...(process.env as Record<string, string>) },
      }
    : undefined,
});
