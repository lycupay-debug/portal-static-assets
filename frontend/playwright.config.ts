import { defineConfig, devices } from '@playwright/test'

/**
 * Responsibility: run browser-level regression checks against the built static frontend.
 * Dependencies: Playwright's Chromium runtime and Vite preview serving frontend/dist.
 * Collaboration: tests exercise public route rendering and client-side navigation; API calls
 * remain outside this smoke suite, so these checks do not claim authenticated business-flow coverage.
 * Boundaries: uses an isolated local preview, no production URL or real credentials; the runner
 * owns browser lifecycle and shuts down the preview process after the suite.
 */
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'npm run preview -- --host 127.0.0.1 --port 4173 --strictPort',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
})
