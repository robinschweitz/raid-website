import { expect, test, type APIRequestContext, type Page, type TestInfo } from '@playwright/test'
import { DatabaseSync } from 'node:sqlite'
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { admin, apiAs } from './fixtures'

// Screenshots for the user guide (Anleitung page). Runs only with DOCS_TOUR=1:
//   DOCS_TOUR=1 npx playwright test e2e/zz-docs-tour.spec.ts
test.skip(!process.env.DOCS_TOUR, 'only for the user guide screenshots')
test.describe.configure({ mode: 'serial' })

const out = path.join(import.meta.dirname, '..', 'docs-screenshots')
// The in-app guide (src/pages/Guide.tsx) serves compressed copies from here.
const published = path.join(import.meta.dirname, '..', 'public', 'anleitung')
const databasePath = path.join(import.meta.dirname, '..', '.e2e', 'raid.db')
const pw = (name: string) => `${name.toLowerCase()}-passwort-123`
const people = {
  Thorgrim: { role: 'RAIDLEADER', char: ['Thorgrim', 'Krieger', 'Schutz (Tank)', 'Furor'] },
  Mira: { role: 'RAIDASSISTENT', char: ['Mirabelle', 'Paladin', 'Heilig', 'Vergeltung'] },
  Sternfall: { role: 'MEMBER', char: ['Sternfall', 'Priester', 'Schatten', 'Disziplin'] },
  Funkenflug: { role: 'MEMBER', char: ['Funkenflug', 'Magier', 'Feuer', 'Frost'] },
  Eisenfaust: { role: 'MEMBER', char: ['Eisenfaust', 'Todesritter', 'Unheilig', 'Frost'] },
  Lichtwind: { role: 'MEMBER', char: ['Lichtwind', 'Druide', 'Wiederherstellung (Baum)', 'Gleichgewicht (Eule)'] },
  Schattenpfeil: { role: 'MEMBER', char: ['Schattenpfeil', 'Jäger', 'Treffsicherheit', 'Überleben'] },
  Nebelhauch: { role: 'MEMBER', char: ['Nebelhauch', 'Hexer', 'Dämonologie', 'Gebrechen'] },
} as const
type Person = keyof typeof people
const ids: Record<string, number> = {}
const chars: Record<string, number> = {}
let futureRaid = 0
let pastRaid = 0

const login = (request: APIRequestContext, name: Person) => apiAs(request, { username: name.toLowerCase(), password: pw(name) })

async function shot(page: Page, testInfo: TestInfo, name: string, focus?: string) {
  mkdirSync(out, { recursive: true })
  if (focus) await page.getByText(focus, { exact: false }).first().evaluate((element) => element.scrollIntoView({ block: 'start' }))
  if (focus) await page.mouse.wheel(0, -90)
  await page.waitForTimeout(400)
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  const png = await page.screenshot({ path: path.join(out, `${name}.png`) })
  await testInfo.attach(name, { path: path.join(out, `${name}.png`), contentType: 'image/png' })
  mkdirSync(published, { recursive: true })
  writeFileSync(path.join(published, `${name}.webp`), await toWebP(page, png))
}

// Chromium encodes WebP itself, so the guide images need no extra dependency.
async function toWebP(page: Page, png: Buffer): Promise<Buffer> {
  const dataURL = await page.evaluate(async (source) => {
    const image = new Image()
    image.src = source
    await image.decode()
    const canvas = document.createElement('canvas')
    canvas.width = image.naturalWidth
    canvas.height = image.naturalHeight
    canvas.getContext('2d')?.drawImage(image, 0, 0)
    return canvas.toDataURL('image/webp', 0.8)
  }, `data:image/png;base64,${png.toString('base64')}`)
  return Buffer.from(dataURL.split(',')[1], 'base64')
}

async function signIn(page: Page, name: string, password: string) {
  await page.goto('/#/login')
  await page.getByLabel('Benutzername').fill(name)
  await page.getByLabel('Passwort').fill(password)
  await page.locator('form').getByRole('button', { name: 'Anmelden' }).click()
  await expect(page.getByRole('button', { name: 'Abmelden' })).toBeVisible()
}
const signInAs = (page: Page, name: Person) => signIn(page, name.toLowerCase(), pw(name))

const thursday = (weeksFromNow: number) => {
  const d = new Date()
  d.setUTCDate(d.getUTCDate() + ((4 - d.getUTCDay() + 7) % 7) + 7 * weeksFromNow)
  d.setUTCHours(18, 0, 0, 0)
  return d
}

test.beforeAll(async ({ request }) => {
  const api = await apiAs(request, admin)
  const me = (await (await request.get('/api/v1/auth/me')).json()).data
  ids.admin = me.id
  for (const rank of ['Offizier', 'Raider', 'Initiat']) await api.post('/api/v1/guild-ranks', { name: rank })
  await api.post('/api/v1/raid-groups', { name: 'Stammgruppe', color: '#7ad7ff' })
  const db = new DatabaseSync(databasePath)
  const now = new Date().toISOString().replace('T', ' ').replace('Z', '+00:00')
  for (const [name, person] of Object.entries(people)) {
    const created = await api.post('/api/v1/players', { name })
    ids[name] = created.id
    await api.put(`/api/v1/players/${created.id}/account`, { username: name.toLowerCase(), password: pw(name), role: person.role })
    const [charName, cls, main, second] = person.char
    const row = db.prepare('INSERT INTO characters (player_id, name, class, main_spec, second_spec, is_main, active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, 1, ?, ?)').run(created.id, charName, cls, main, second, now, now)
    chars[name] = Number(row.lastInsertRowid)
  }
  const twink = db.prepare("INSERT INTO characters (player_id, name, class, main_spec, second_spec, is_main, active, created_at, updated_at) VALUES (?, 'Morgentau', 'Schamane', 'Wiederherstellung', 'Elementar', 0, 1, ?, ?)").run(ids.Sternfall, now, now)
  chars.Morgentau = Number(twink.lastInsertRowid)
  db.prepare("INSERT INTO enhance_point_transactions (player_id, amount, type, reason, actor_id, created_at) VALUES (?, 150, 'adjustment', 'Startguthaben aus der alten Liste', ?, ?)").run(ids.Sternfall, ids.admin, now)
  db.prepare("INSERT INTO enhance_point_transactions (player_id, amount, type, reason, actor_id, created_at) VALUES (?, 40, 'adjustment', 'Startguthaben aus der alten Liste', ?, ?)").run(ids.Funkenflug, ids.admin, now)
  db.close()

  const catalog = (await (await request.get('/api/v1/loot-catalog')).json()).data
  const icc = catalog.instances.find((row: { name: string }) => /Eiskrone/.test(row.name))
  const series = await api.post('/api/v1/raid-series', { name: 'ICC25 Donnerstag', name_template: 'ICC25-{number}', next_raid_number: 1, raid_leader_id: ids.Thorgrim, weekdays: [4], interval_weeks: 1, start_time: '20:00', timezone: 'Europe/Berlin', catalog_instance_id: icc.id, raid_size: 25 })
  const past = await api.post('/api/v1/raids', { name: '', raid_leader_id: ids.Thorgrim, series_id: series.id, scheduled_start: thursday(-1).toISOString(), catalog_instance_id: icc.id, raid_size: 25 })
  pastRaid = past.id
  const roles: Record<string, string> = { Thorgrim: 'TANK', Mira: 'HEAL', Lichtwind: 'HEAL' }
  for (const name of ['Thorgrim', 'Mira', 'Sternfall', 'Funkenflug', 'Eisenfaust', 'Lichtwind', 'Schattenpfeil']) {
    await api.post(`/api/v1/raids/${past.id}/attendance`, { player_id: ids[name], character_id: chars[name], role: roles[name] ?? 'DD' })
  }
  const drops = catalog.drops.filter((row: { raid_size: number; boss_id: string }) => row.raid_size === 25 && catalog.bosses.some((boss: { id: string; instance_id: string }) => boss.id === row.boss_id && boss.instance_id === icc.id))
  const names = new Map<string, string>(catalog.items.map((row: { item_id: string; name: string }) => [row.item_id, row.name]))
  for (const row of drops) row.name = names.get(row.item_id) ?? row.item_id
  const byName = (pattern: RegExp) => drops.find((row: { name: string }) => pattern.test(row.name))
  const axe = byName(/Bryntroll/) ?? drops[0]
  const second = drops.find((row: { item_id: string }) => row.item_id !== axe.item_id)
  await api.post(`/api/v1/raids/${past.id}/loot`, { catalog_boss_id: axe.boss_id, item_id: axe.item_id, award_method: 'MAIN', awarded_to_player_id: ids.Eisenfaust, awarded_to_character_id: chars.Eisenfaust })
  await api.post(`/api/v1/raids/${past.id}/loot`, { catalog_boss_id: second.boss_id, item_id: second.item_id, award_method: 'SECOND', awarded_to_player_id: ids.Funkenflug, awarded_to_character_id: chars.Funkenflug })
  await api.post(`/api/v1/raid-series/${series.id}/plan`, { until: thursday(3).toISOString().slice(0, 10) })
  await api.post(`/api/v1/raids/${past.id}/archive`, {})
  const raids = (await (await request.get('/api/v1/raids')).json()).data
  futureRaid = raids.filter((row: { status: string; archived_at: string | null; scheduled_start: string }) => !row.archived_at && row.status === 'PLANNED')
    .sort((a: { scheduled_start: string }, b: { scheduled_start: string }) => Date.parse(a.scheduled_start) - Date.parse(b.scheduled_start))[0].id
  await api.put(`/api/v1/raids/${futureRaid}/recruitment-template`, {
    text: 'LFM ICC25 Donnerstag 20 Uhr – suchen noch {{SETUP_KURZ}}. /w Thorgrim',
    setup: [
      ['Maintank', 'Offtank', 'Krieger Furor', 'Schurke Kampf', 'Todesritter Unheilig'],
      ['Paladin Vergeltung', 'Paladin Vergeltung', 'Irgendein MDD', 'Irgendein MDD', 'Irgendein MDD'],
      ['Jäger Treffsicherheit', 'Hexer Dämonologie', 'Druide Eule', 'Druide Eule', 'Magier Feuer'],
      ['Schamane Elementar', 'Priester Schatten', 'Priester Schatten', 'Irgendein RDD', 'Irgendein RDD'],
      ['Paladin Heilig', 'Schamane Wiederherstellung', 'Druide Wiederherstellung', 'Priester Disziplin', 'Irgendein Heiler'],
    ],
  })

  const poll = await api.post('/api/v1/polls', { title: 'Zusätzlicher Raidtag?', description: 'Welcher Tag passt euch für einen zweiten ICC-Raid?', poll_type: 'CHOICE', options: ['Mittwoch', 'Freitag', 'Sonntag'] })
  const signup = async (name: Person, characterID: number, spec: string) => (await login(request, name)).put(`/api/v1/raids/${futureRaid}/signups`, { choices: [{ character_id: characterID, spec }] })
  await signup('Thorgrim', chars.Thorgrim, 'Schutz (Tank)')
  await signup('Mira', chars.Mira, 'Heilig')
  await signup('Funkenflug', chars.Funkenflug, 'Feuer')
  await signup('Eisenfaust', chars.Eisenfaust, 'Unheilig')
  await signup('Schattenpfeil', chars.Schattenpfeil, 'Treffsicherheit')
  await (await login(request, 'Lichtwind')).put(`/api/v1/raids/${futureRaid}/absences/${ids.Lichtwind}`, { reason: 'Spätschicht, komme evtl. später', status: 'TENTATIVE' })
  await (await login(request, 'Nebelhauch')).put(`/api/v1/raids/${futureRaid}/absences/${ids.Nebelhauch}`, { reason: 'Urlaub', status: 'DECLINED' })
  for (const [name, choice] of [['Funkenflug', 'Freitag'], ['Eisenfaust', 'Sonntag'], ['Schattenpfeil', 'Freitag'], ['Mira', 'Freitag']] as const) {
    await (await login(request, name)).post(`/api/v1/polls/${poll.id}/vote`, { choices: [choice] })
  }
  const sternfall = await login(request, 'Sternfall')
  const distinct = drops.filter((row: { item_id: string }, index: number) => row.item_id !== axe.item_id && drops.findIndex((other: { item_id: string }) => other.item_id === row.item_id) === index)
  const wishDrops = [distinct[5], distinct[25], distinct[60]].filter(Boolean)
  for (const [index, drop] of wishDrops.entries()) {
    await sternfall.post(`/api/v1/characters/${chars.Sternfall}/wishlist/items`, { item_id: drop.item_id, item_name: drop.name, priority: 3 - index, notes: '' })
  }
  await sternfall.post(`/api/v1/players/${ids.Sternfall}/locks`, { raid_id: futureRaid, item_id: wishDrops[0].item_id, item_name: wishDrops[0].name, boss: catalog.bosses.find((boss: { id: string }) => boss.id === wishDrops[0].boss_id)?.name ?? '' })
  const lead = await login(request, 'Thorgrim')
  const overview = (await (await request.get(`/api/v1/raids/${futureRaid}/signups`)).json()).data
  for (const entry of overview.entries.filter((row: { character_name: string }) => ['Thorgrim', 'Mirabelle', 'Eisenfaust'].includes(row.character_name))) {
    await lead.post(`/api/v1/raids/${futureRaid}/signups/${entry.id}/accept`, { replace_attendance_id: 0, expected_character_id: 0 })
  }
})

test('Erste Schritte', async ({ page }, testInfo) => {
  await page.goto('/#/login')
  await shot(page, testInfo, '01-login')
  await page.getByRole('button', { name: 'Code anfordern' }).click()
  await shot(page, testInfo, '02-code-anfordern')
  await page.getByRole('button', { name: 'Registrieren' }).click()
  await shot(page, testInfo, '03-registrieren')
  await page.goto('/#/')
  await page.waitForTimeout(800)
  await shot(page, testInfo, '04-oeffentliche-uebersicht')
})

test('Mitglied', async ({ page, context }, testInfo) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await signInAs(page, 'Sternfall')
  await expect(page.getByRole('heading', { name: 'Meine nächsten Raids' })).toBeVisible()
  await page.waitForTimeout(800)
  await shot(page, testInfo, '10-uebersicht-mitglied')
  await page.mouse.wheel(0, 900)
  await shot(page, testInfo, '11-uebersicht-unten')
  await page.mouse.wheel(0, -2000)
  const first = page.locator('.my-raid').first()
  await first.getByRole('button', { name: 'Kann nicht' }).click()
  await first.getByLabel('Grund (optional)').fill('Familienfeier')
  await shot(page, testInfo, '12-absage-mit-grund', 'Meine nächsten Raids')
  await first.getByRole('button', { name: 'Abbrechen' }).click()
  await first.getByRole('button', { name: /anmelden$/ }).click()
  await expect(first.getByText('Angemeldet', { exact: true })).toBeVisible()
  await page.locator('.my-raid').nth(1).getByRole('button', { name: 'Unsicher' }).click()
  await page.locator('.my-raid').nth(1).getByLabel('Was ist unsicher? (optional)').fill('komme evtl. später')
  await page.locator('.my-raid').nth(1).getByRole('button', { name: 'Als unsicher melden' }).click()
  await expect(page.locator('.my-raid').nth(1).getByText('Unsicher', { exact: true })).toBeVisible()
  await shot(page, testInfo, '13-angemeldet-und-unsicher', 'Meine nächsten Raids')
  await first.getByRole('link', { name: 'Anmeldung ansehen' }).click()
  await expect(page.getByLabel('Dein Status')).toBeVisible()
  await shot(page, testInfo, '14-raid-anmeldungen-mitglied')
  await page.mouse.wheel(0, 700)
  await shot(page, testInfo, '15-raid-anmeldungen-formular')

  await page.goto('/#/loot-catalog')
  await page.getByPlaceholder('Name oder Item-ID').fill('Bryntroll')
  await page.locator('.catalog-wishlist').first().getByRole('button', { name: 'Sternfall' }).click()
  await expect(page.locator('.catalog-wishlist-banner')).toContainText('steht jetzt auf der Wunschliste')
  await shot(page, testInfo, '16-beute-wunsch')
  await page.goto('/#/wishlist')
  await page.getByRole('button', { name: 'Export kopieren' }).click().catch(() => {})
  await page.waitForTimeout(500)
  await shot(page, testInfo, '17-wunschliste')
  await page.getByLabel('Wunschlisten-Export').scrollIntoViewIfNeeded().catch(() => {})
  await shot(page, testInfo, '18-wunschliste-export')

  await page.goto(`/#/players/${ids.Sternfall}`)
  await page.waitForTimeout(600)
  await shot(page, testInfo, '19-profil-ep')
  await page.getByRole('button', { name: 'Item-Locks' }).click()
  await page.waitForTimeout(300)
  await shot(page, testInfo, '20-item-locks')
  await page.getByRole('button', { name: 'Konto' }).click()
  await page.waitForTimeout(300)
  await shot(page, testInfo, '21-konto')
  await page.goto('/#/polls')
  await page.waitForTimeout(600)
  await shot(page, testInfo, '22-umfragen')
})

test('Raidleitung', async ({ page }, testInfo) => {
  await signInAs(page, 'Thorgrim')
  await page.goto('/#/raids')
  await page.waitForTimeout(600)
  await page.getByText('Neuer Raid').click()
  await page.getByLabel('Neue Raidserie').selectOption({ label: 'ICC25 Donnerstag' })
  await shot(page, testInfo, '30-raids-neuer-raid')
  await page.getByText('Neuer Raid').click()
  await page.getByText('Termine im Voraus anlegen').click()
  await page.getByLabel('Raidserie für Vorausplanung').selectOption({ label: 'ICC25 Donnerstag' })
  await shot(page, testInfo, '31-raids-voraus-planen')
  await page.goto(`/#/raids/${futureRaid}`)
  await page.getByRole('button', { name: 'Anmeldungen', exact: true }).click()
  await page.waitForTimeout(600)
  await shot(page, testInfo, '32-raid-anmeldungen-leitung')
  await page.getByRole('heading', { name: 'Charakteroptionen nach Spieler' }).scrollIntoViewIfNeeded()
  await shot(page, testInfo, '33-anmeldungen-uebernehmen')
  await page.getByRole('button', { name: /^Aufstellung/ }).click()
  await page.waitForTimeout(600)
  await shot(page, testInfo, '34-aufstellung')
  await page.locator('button[title="Teilnahme korrigieren"]').first().click()
  await page.waitForTimeout(300)
  await shot(page, testInfo, '35-aufstellung-bearbeiten')
  await page.getByRole('button', { name: 'Bearbeitung abbrechen' }).click()
  await page.getByRole('button', { name: 'Gesuchtext' }).click()
  await page.waitForTimeout(600)
  await shot(page, testInfo, '36-gesuchtext', 'Textvorlage')
  await page.getByRole('button', { name: /^Loot/ }).click()
  await page.waitForTimeout(600)
  await page.getByText('Loot erfassen').first().click()
  await page.waitForTimeout(400)
  await shot(page, testInfo, '37-loot')
  await page.getByRole('button', { name: 'Planung & Details' }).click()
  await page.waitForTimeout(300)
  await shot(page, testInfo, '38-planung-details')
  await page.getByRole('button', { name: 'Archivieren' }).click()
  await expect(page.getByRole('heading', { name: 'EP-Übersicht vor dem Archivieren' })).toBeVisible()
  await shot(page, testInfo, '38b-archivieren-pruefen', 'EP-Übersicht vor dem Archivieren')
  await page.getByRole('button', { name: 'Zurück' }).click()
  await page.goto(`/#/raids/${pastRaid}`)
  await page.getByRole('button', { name: /^Loot/ }).click()
  await page.waitForTimeout(600)
  await shot(page, testInfo, '39-archivierter-raid-loot')
})

test('Admin', async ({ page }, testInfo) => {
  await signIn(page, admin.username, admin.password)
  await page.goto('/#/raid-series')
  await page.waitForTimeout(600)
  await shot(page, testInfo, '50-raidserien')
  await page.goto('/#/players')
  await page.waitForTimeout(600)
  await page.getByText('Neuer Spieler').click()
  await shot(page, testInfo, '51-spieler')
  await page.goto(`/#/players/${ids.Funkenflug}`)
  await page.getByRole('button', { name: 'Konto' }).click()
  await page.waitForTimeout(400)
  await shot(page, testInfo, '52-spieler-konto-admin')
  await page.goto('/#/registration-requests')
  await page.waitForTimeout(600)
  await shot(page, testInfo, '53-registrierung')
  await page.goto('/#/roster')
  await page.waitForTimeout(600)
  await shot(page, testInfo, '54-gildenkonfiguration')
  await page.goto('/#/audit-log')
  await page.waitForTimeout(800)
  await shot(page, testInfo, '55-audit-protokoll')
})
