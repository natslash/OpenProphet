import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  retries: 0,
  use: {
    baseURL: 'http://localhost:3737',
    headless: true,
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'node agent/server.js',
    port: 3737,
    timeout: 15_000,
    reuseExistingServer: true,
  },
});
