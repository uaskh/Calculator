import { defineConfig, devices } from '@playwright/test'

// Dedicated ports so the suite never collides with `make dev`.
const API_PORT = Number(process.env.E2E_API_PORT ?? 18080)
const WEB_PORT = Number(process.env.E2E_WEB_PORT ?? 14173)
const isCI = Boolean(process.env.CI)

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  reporter: isCI
    ? [['github'], ['html', { open: 'never' }]]
    : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: `http://127.0.0.1:${WEB_PORT}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'desktop-chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile-chromium', use: { ...devices['Pixel 7'] } },
  ],
  webServer: [
    {
      name: 'api',
      command: 'go run ./cmd/api',
      cwd: '../backend',
      url: `http://127.0.0.1:${API_PORT}/healthz`,
      env: { HTTP_ADDR: `127.0.0.1:${API_PORT}`, LOG_LEVEL: 'warn' },
      reuseExistingServer: !isCI,
      timeout: 120_000,
    },
    {
      name: 'web',
      command: `npm run dev -- --host 127.0.0.1 --port ${WEB_PORT}`,
      url: `http://127.0.0.1:${WEB_PORT}`,
      env: { API_PROXY_TARGET: `http://127.0.0.1:${API_PORT}` },
      reuseExistingServer: !isCI,
      timeout: 120_000,
    },
  ],
})
