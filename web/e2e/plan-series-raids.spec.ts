import { expect, test } from '@playwright/test'
import { admin, apiAs, assistant, berlinDate, member, seedGuild, signIn, signOut, snapshot } from './fixtures'

// ADR 078: raids of a series are planned ahead and players sign up in advance.
// The steps build on each other, so they run in order against one database.
test.describe.configure({ mode: 'serial' })

let guild: Awaited<ReturnType<typeof seedGuild>>

test.describe('Raids im Voraus planen', () => {
  test.beforeAll(async ({ request }) => {
    guild = await seedGuild(request)
  })

  test('Raidassistent legt die Termine einer Serie bis zu einem Datum an', async ({ page }, testInfo) => {
    await signIn(page, assistant)
    await page.goto('/#/raids')
    await page.getByText('Termine im Voraus anlegen').click()
    await page.getByLabel('Raidserie für Vorausplanung').selectOption({ label: 'Donnerstagsraid' })
    await page.getByLabel('Planen bis').fill(berlinDate(21))
    await snapshot(page, testInfo, '01-raids-termine-im-voraus-anlegen')
    await page.getByRole('button', { name: 'Raids anlegen' }).click()

    await expect(page.getByRole('status').filter({ hasText: /Raids? angelegt\. Die Anmeldung ist offen\./ })).toBeVisible()
    const calendar = page.getByRole('region', { name: 'Raidübersicht' })
    await expect(calendar.getByRole('link', { name: 'ICC-1' })).toBeVisible()
    await expect(calendar.getByRole('link', { name: 'ICC-3' })).toBeVisible()
    await snapshot(page, testInfo, '02-raids-angelegt')

    // Planning the same range again creates nothing new.
    await page.getByRole('button', { name: 'Raids anlegen' }).click()
    await expect(page.getByText('Alle Termine bis zu diesem Datum sind schon angelegt.')).toBeVisible()
    await signOut(page)
  })

  test('Admin stellt eine automatische Vorausplanung für die Serie ein', async ({ page }, testInfo) => {
    await signIn(page, admin)
    await page.goto('/#/raid-series')
    await page.getByRole('listitem').filter({ hasText: 'Donnerstagsraid' }).getByRole('button', { name: 'Serie bearbeiten' }).click()
    await page.getByLabel('Wochen im Voraus').fill('6')
    await snapshot(page, testInfo, '03-raidserie-automatisch-planen')
    await page.getByRole('button', { name: 'Serie speichern' }).click()
    await expect(page.getByText(/Automatisch\s+6 Wochen\s+im Voraus geplant/)).toBeVisible()
    await expect(page.getByText(/Nächste Nummer: [67]/)).toBeVisible()
    await snapshot(page, testInfo, '04-raidserie-gespeichert', page.getByText(/Automatisch\s+6 Wochen\s+im Voraus geplant/))

    await page.goto('/#/raids')
    await page.getByLabel('Raid-Ansicht').selectOption('upcoming')
    const calendar = page.getByRole('region', { name: 'Raidübersicht' })
    await expect.poll(async () => calendar.getByRole('link', { name: /^ICC-\d+$/ }).count()).toBeGreaterThanOrEqual(5)
    await snapshot(page, testInfo, '05-sechs-wochen-geplant')
    await signOut(page)
  })

  test('Mitglied meldet sich Wochen vorher für einen geplanten Raid an', async ({ page }, testInfo) => {
    await signIn(page, member)
    await page.goto('/#/raids')
    await page.getByRole('region', { name: 'Raidübersicht' }).getByRole('link', { name: 'ICC-4' }).click()
    await page.getByRole('button', { name: 'Anmeldungen', exact: true }).click()
    await expect(page.getByText('Anmeldung offen')).toBeVisible()
    await page.getByRole('checkbox', { name: /Funkenflug/ }).check()
    await page.getByRole('button', { name: 'Ausgewählte Charaktere anmelden' }).click()
    await expect(page.getByText(/Funkenflug · Magier · Angemeldet/)).toBeVisible()
    await snapshot(page, testInfo, '06-mitglied-angemeldet', page.getByRole('heading', { name: 'Anmeldeliste' }))
    await signOut(page)
  })

  test('Audit-Protokoll zeigt die vorausgeplanten Raids', async ({ page }, testInfo) => {
    await signIn(page, admin)
    await page.goto('/#/audit-log')
    await page.getByLabel('Protokoll durchsuchen').fill('ICC-')
    await expect(page.getByRole('row').filter({ hasText: 'Vorausgeplant' }).first()).toBeVisible()
    await expect(page.getByRole('row').filter({ hasText: 'Raid: ICC-1' }).filter({ hasText: 'Raidassistent' })).toBeVisible()
    await snapshot(page, testInfo, '07-audit-protokoll')
  })

  test('Gesuchtext fasst die offenen Plätze kurz zusammen', async ({ page, request }, testInfo) => {
    await signIn(page, assistant)
    await page.goto('/#/raids')
    await page.getByRole('region', { name: 'Raidübersicht' }).getByRole('link', { name: 'ICC-1' }).click()
    await page.getByRole('button', { name: 'Gesuchtext' }).click()
    await page.getByRole('button', { name: 'Setup leeren' }).click()
    const slots: [string, string][] = [
      ['Gruppe 1, Platz 1', 'Druide Wiederherstellung'], ['Gruppe 1, Platz 2', 'Priester Disziplin'],
      ['Gruppe 2, Platz 1', 'Irgendein MDD'], ['Gruppe 2, Platz 2', 'Irgendein MDD'],
      ['Gruppe 3, Platz 1', 'Magier Feuer'], ['Gruppe 3, Platz 2', 'Priester Schatten'],
    ]
    for (const [slot, option] of slots) await page.getByLabel(slot).selectOption(option)
    await page.getByLabel('Gesuchtext-Vorlage').fill('LFM ICC25 – suchen noch {{SETUP_KURZ}}, /w me')
    const result = page.getByLabel('Fertiger Gesuchtext')
    await expect(result).toHaveValue('LFM ICC25 – suchen noch 2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage, SP), /w me')
    await page.getByRole('button', { name: 'Vorlage speichern' }).click()
    await expect(page.getByRole('status').filter({ hasText: /^Gespeichert\.$/ })).toBeVisible()
    await snapshot(page, testInfo, '08-gesuchtext-kurz', page.getByRole('heading', { name: 'Textvorlage' }))

    // Once the mage is in the roster, the text no longer asks for one.
    const api = await apiAs(request, admin)
    const raidID = Number(page.url().split('/').pop())
    await api.post(`/api/v1/raids/${raidID}/attendance`, { player_id: guild.memberID, character_id: guild.characterID, role: 'DD' })
    await page.reload()
    await page.getByRole('button', { name: 'Gesuchtext' }).click()
    await expect(result).toHaveValue('LFM ICC25 – suchen noch 2 Heal (Baum, Diszi), 2 MDD (beliebig), 1 RDD (SP), /w me')
    await snapshot(page, testInfo, '09-gesuchtext-nach-aufstellung', page.getByRole('heading', { name: 'Textvorlage' }))
  })
})
