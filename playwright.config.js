// @ts-check
const { defineConfig, devices } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './e2e/tests',
  testMatch: '**/*.spec.js',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  timeout: 30_000,

  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  /* 테스트 실행 시 두 프론트엔드 앱을 자동 기동 */
  webServer: [
    {
      command: 'cd GoJIRA-App && REACT_APP_MSW=true npm start',
      port: 3001,
      timeout: 60_000,
      reuseExistingServer: !process.env.CI,
    },
    {
      command: 'cd Platform-App && REACT_APP_MSW=true npm start',
      port: 3000,
      timeout: 60_000,
      reuseExistingServer: !process.env.CI,
    },
  ],
});
