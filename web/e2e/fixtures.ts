import { expect, type APIRequestContext, type Locator, type Page, type TestInfo } from '@playwright/test'
import { DatabaseSync } from 'node:sqlite'
import { createHmac } from 'node:crypto'
import { mkdirSync } from 'node:fs'
import path from 'node:path'

export const admin = { username: 'e2e-admin', password: 'e2e-admin-password' }
export const assistant = { username: 'e2e-raidassistent', password: 'raidassistent-password' }
export const member = { username: 'e2e-mitglied', password: 'mitglied-password' }

const databasePath = path.join(import.meta.dirname, '..', '.e2e', 'raid.db')

// Admin functions need a second factor (FR-3.10, SC-040), so the e2e Admin has an
// authenticator secret and logs in with a code, like a real Admin.
const adminTOTPSecret = 'JBSWY3DPEHPK3PXP'

function base32Decode(value: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const char of value.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(char).toString(2).padStart(5, '0')
  const bytes: number[] = []
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2))
  return Buffer.from(bytes)
}

/** The current RFC 6238 code for a base32 secret, as an authenticator app shows it. */
export function totpCode(secret = adminTOTPSecret): string {
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(Date.now() / 1000 / 30)))
  const sum = createHmac('sha1', base32Decode(secret)).update(counter).digest()
  const offset = sum[sum.length - 1] & 0x0f
  return String((sum.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

/**
 * Gives the e2e Admin its authenticator secret (idempotent). It also forgets
 * the last used TOTP step, so tests can sign the Admin in several times within
 * one 30-second step; the replay rule (SC-012) is covered by Go tests.
 */
function ensureAdminTOTP() {
  const db = new DatabaseSync(databasePath)
  const now = new Date().toISOString().replace('T', ' ').replace('Z', '+00:00')
  db.prepare(`INSERT INTO player_mfa_settings (player_id, totp_secret, totp_enabled_at, created_at, updated_at)
    SELECT id, ?, ?, ?, ? FROM players WHERE username = ?
    ON CONFLICT(player_id) DO UPDATE SET totp_secret = excluded.totp_secret, totp_enabled_at = excluded.totp_enabled_at, totp_last_step = NULL`).run(adminTOTPSecret, now, now, now, admin.username)
  db.close()
}

/** Signs in through the API and returns a request context that sends the CSRF token. */
export async function apiAs(request: APIRequestContext, user: { username: string; password: string }) {
  const csrf = async () => (await (await request.get('/api/v1/auth/csrf')).json()).data.token as string
  if (user.username === admin.username) ensureAdminTOTP()
  const token = await csrf()
  const login = await request.post('/api/v1/auth/login', { data: user, headers: { 'X-CSRF-Token': token } })
  expect(login.ok(), await login.text()).toBeTruthy()
  const started = (await login.json()).data
  if (started?.mfa_required) {
    const verified = await request.post('/api/v1/auth/mfa/verify', { data: { challenge: started.challenge, code: totpCode() }, headers: { 'X-CSRF-Token': await csrf() } })
    expect(verified.ok(), await verified.text()).toBeTruthy()
  }
  const send = async (method: 'post' | 'put' | 'patch', url: string, data: unknown) => {
    const response = await request[method](url, { data, headers: { 'X-CSRF-Token': await csrf() } })
    expect(response.ok(), `${method.toUpperCase()} ${url}: ${await response.text()}`).toBeTruthy()
    const body = await response.text()
    return body ? JSON.parse(body).data : undefined
  }
  return {
    post: (url: string, data: unknown) => send('post', url, data),
    put: (url: string, data: unknown) => send('put', url, data),
    patch: (url: string, data: unknown) => send('patch', url, data),
  }
}

/**
 * Seeds the guild for the planning scenario: a weekly Thursday series, a raid
 * assistant and a member with one character. Characters normally come from the
 * Rising-Gods armory, which the tests cannot reach, so the character row is
 * written straight into the test database.
 */
export async function seedGuild(request: APIRequestContext) {
  const api = await apiAs(request, admin)
  const adminPlayer = (await (await request.get('/api/v1/auth/me')).json()).data
  const series = await api.post('/api/v1/raid-series', {
    name: 'Donnerstagsraid', name_template: 'ICC-{number}', next_raid_number: 1, raid_leader_id: adminPlayer.id,
    weekdays: [4], interval_weeks: 1, start_time: '20:00', timezone: 'Europe/Berlin',
  })
  const people: Record<string, number> = {}
  for (const [user, role, name] of [[assistant, 'RAIDASSISTENT', 'Raidassistent'], [member, 'MEMBER', 'Mitglied']] as const) {
    const created = await api.post('/api/v1/players', { name })
    await provisionAccount(request, api, created.id, user.username, user.password, role)
    people[role] = created.id
  }
  const db = new DatabaseSync(databasePath)
  const now = new Date().toISOString().replace('T', ' ').replace('Z', '+00:00')
  const character = db.prepare("INSERT INTO characters (player_id, name, class, main_spec, second_spec, is_main, active, created_at, updated_at) VALUES (?, 'Funkenflug', 'Magier', 'Feuer', 'Frost', 1, 1, ?, ?)").run(people.MEMBER, now, now)
  db.close()
  return { seriesID: series.id as number, memberID: people.MEMBER, characterID: Number(character.lastInsertRowid) }
}

export interface MemberSetup { username: string; password: string; name: string; character: string; characterClass: string; mainSpec: string; secondSpec: string }

/** Creates a member account with one main character, as an admin would. */
export async function createMember(request: APIRequestContext, setup: MemberSetup) {
  const api = await apiAs(request, admin)
  const created = await api.post('/api/v1/players', { name: setup.name })
  await provisionAccount(request, api, created.id, setup.username, setup.password, 'MEMBER')
  const db = new DatabaseSync(databasePath)
  const now = new Date().toISOString().replace('T', ' ').replace('Z', '+00:00')
  const character = db.prepare('INSERT INTO characters (player_id, name, class, main_spec, second_spec, is_main, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)').run(created.id, setup.character, setup.characterClass, setup.mainSpec, setup.secondSpec, now, now)
  db.close()
  return { playerID: created.id as number, characterID: Number(character.lastInsertRowid) }
}

/** Creates a weekly series and plans its raids for the next weeks. */
export async function createPlannedSeries(request: APIRequestContext, name: string, template: string, weekday: number, weeks: number) {
  const api = await apiAs(request, admin)
  const me = (await (await request.get('/api/v1/auth/me')).json()).data
  const series = await api.post('/api/v1/raid-series', { name, name_template: template, next_raid_number: 1, raid_leader_id: me.id, weekdays: [weekday], interval_weeks: 1, start_time: '20:00', timezone: 'Europe/Berlin' })
  await api.post(`/api/v1/raid-series/${series.id}/plan`, { until: berlinDate(7 * weeks) })
  return series.id as number
}

export async function signIn(page: Page, user: { username: string; password: string }) {
  const usesTOTP = user.username === admin.username
  if (usesTOTP) ensureAdminTOTP()
  await page.goto('/#/login')
  await page.getByLabel('Benutzername').fill(user.username)
  await page.getByLabel('Passwort').fill(user.password)
  await page.locator('form').getByRole('button', { name: 'Anmelden' }).click()
  if (usesTOTP) {
    await page.getByLabel('MFA-Code').fill(totpCode())
    await page.locator('form').getByRole('button', { name: 'Anmelden' }).click()
  }
  await expect(page.getByRole('button', { name: 'Abmelden' })).toBeVisible()
}

export async function signOut(page: Page) {
  await page.getByRole('button', { name: 'Abmelden' }).click()
  await expect(page.getByLabel('Benutzername')).toBeVisible()
}

/**
 * Saves a screenshot of the visible window to web/e2e-screenshots and attaches
 * it to the report. The sidebar is fixed, so full-page captures would repeat
 * it; scroll `focus` into view instead.
 */
export async function snapshot(page: Page, testInfo: TestInfo, name: string, focus?: Locator) {
  const dir = path.join(import.meta.dirname, '..', 'e2e-screenshots')
  mkdirSync(dir, { recursive: true })
  if (focus) await focus.scrollIntoViewIfNeeded()
  await page.getByText('Zum Inhalt').evaluate((link) => (link as HTMLElement).blur()).catch(() => {})
  const file = path.join(dir, `${name}.png`)
  await page.screenshot({ path: file })
  await testInfo.attach(name, { path: file, contentType: 'image/png' })
}

/** The local calendar day in Berlin, `days` days from now, as YYYY-MM-DD. */
export function berlinDate(days: number) {
  const date = new Date(Date.now() + days * 86_400_000)
  return new Intl.DateTimeFormat('sv-SE', { timeZone: 'Europe/Berlin' }).format(date)
}


// Account setup follows the same reset-code path an Admin and member use.
async function provisionAccount(request: APIRequestContext, api: Awaited<ReturnType<typeof apiAs>>, playerID: number, username: string, password: string, role: string) {
  await api.put(`/api/v1/players/${playerID}/account`, { username, role })
  const generated = await api.post(`/api/v1/players/${playerID}/reset-code`, {})
  const csrf = (await (await request.get('/api/v1/auth/csrf')).json()).data.token as string
  const response = await request.post('/api/v1/auth/reset-password', { data: { code: generated.code, password }, headers: { 'X-CSRF-Token': csrf } })
  expect(response.status(), await response.text()).toBe(204)
}
