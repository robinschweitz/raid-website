import { expect, type APIRequestContext, type Locator, type Page, type TestInfo } from '@playwright/test'
import { DatabaseSync } from 'node:sqlite'
import { mkdirSync } from 'node:fs'
import path from 'node:path'

export const admin = { username: 'e2e-admin', password: 'e2e-admin-password' }
export const assistant = { username: 'e2e-raidassistent', password: 'raidassistent-password' }
export const member = { username: 'e2e-mitglied', password: 'mitglied-password' }

const databasePath = path.join(import.meta.dirname, '..', '.e2e', 'raid.db')

/** Signs in through the API and returns a request context that sends the CSRF token. */
export async function apiAs(request: APIRequestContext, user: { username: string; password: string }) {
  const csrf = async () => (await (await request.get('/api/v1/auth/csrf')).json()).data.token as string
  const token = await csrf()
  const login = await request.post('/api/v1/auth/login', { data: user, headers: { 'X-CSRF-Token': token } })
  expect(login.ok(), await login.text()).toBeTruthy()
  const send = async (method: 'post' | 'put' | 'patch', url: string, data: unknown) => {
    const response = await request[method](url, { data, headers: { 'X-CSRF-Token': await csrf() } })
    expect(response.ok(), `${method.toUpperCase()} ${url}: ${await response.text()}`).toBeTruthy()
    return (await response.json()).data
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
    await api.put(`/api/v1/players/${created.id}/account`, { username: user.username, password: user.password, role })
    people[role] = created.id
  }
  const db = new DatabaseSync(databasePath)
  const now = new Date().toISOString().replace('T', ' ').replace('Z', '+00:00')
  const character = db.prepare("INSERT INTO characters (player_id, name, class, main_spec, second_spec, is_main, active, created_at, updated_at) VALUES (?, 'Funkenflug', 'Magier', 'Feuer', 'Frost', 1, 1, ?, ?)").run(people.MEMBER, now, now)
  db.close()
  return { seriesID: series.id as number, memberID: people.MEMBER, characterID: Number(character.lastInsertRowid) }
}

export async function signIn(page: Page, user: { username: string; password: string }) {
  await page.goto('/#/login')
  await page.getByLabel('Benutzername').fill(user.username)
  await page.getByLabel('Passwort').fill(user.password)
  await page.locator('form').getByRole('button', { name: 'Anmelden' }).click()
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
