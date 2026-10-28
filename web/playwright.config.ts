import { defineConfig, devices } from '@playwright/test'
import { e2eTOTPKey } from './e2e/totp-key'

// End-to-end tests run the real Go server against a fresh SQLite database in
// web/.e2e. `npm run e2e` builds the frontend first, because the server embeds it.
const port = 8765
export const e2eDatabase = '.e2e/raid.db'

const webServerCommand = process.platform === 'win32'
  ? `powershell -NoProfile -Command "if (Test-Path .e2e) { Remove-Item -Recurse -Force .e2e }; New-Item -ItemType Directory .e2e | Out-Null; go build -C .. -o web/.e2e/server.exe ./cmd/server; Set-Location .e2e; & .\\server.exe"`
  : `rm -rf .e2e && mkdir -p .e2e && (cd .. && go build -o web/.e2e/server ./cmd/server) && cd .e2e && ./server`

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    locale: 'de-DE',
    timezoneId: 'Europe/Berlin',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 960 }, launchOptions: { args: ['--lang=de-DE'] } } }],
  webServer: {
    command: webServerCommand,
    env: {
      PRIVACY_CONTACT_NAME: 'Gildenverwaltung (Test)',
      PRIVACY_CONTACT_ADDRESS: 'kontakt@example.invalid',
      TOTP_ENCRYPTION_KEY: e2eTOTPKey,
      PORT: String(port),
      LISTEN_ADDR: `127.0.0.1:${port}`,
      DATABASE_URL: 'file:raid.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)',
      ADMIN_USERNAME: 'e2e-admin',
      ADMIN_PASSWORD: 'e2e-admin-password',
    },
    url: `http://127.0.0.1:${port}/api/v1/auth/csrf`,
    reuseExistingServer: false,
    timeout: 180_000,
  },
})
