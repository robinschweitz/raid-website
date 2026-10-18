import { expect, test } from '@playwright/test'
import { admin, assistant, berlinDate, member, seedGuild, signIn, signOut, snapshot } from './fixtures'

// ADR 078: raids of a series are planned ahead and players sign up in advance.
// The steps build on each other, so they run in order against one database.
test.describe.configure({ mode: 'serial' })

test.describe('Raids im Voraus planen', () => {
  test.beforeAll(async ({ request }) => {
    await seedGuild(request)
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
    await page.getByRole('button', { name: 'Serie bearbeiten' }).click()
    await page.getByLabel('Wochen im Voraus').fill('6')
    await snapshot(page, testInfo, '03-raidserie-automatisch-planen')
    await page.getByRole('button', { name: 'Serie speichern' }).click()
    await expect(page.getByText(/Automatisch\s+6 Wochen\s+im Voraus geplant/)).toBeVisible()
    await expect(page.getByText(/Nächste Nummer: 7/)).toBeVisible()
    await snapshot(page, testInfo, '04-raidserie-gespeichert', page.getByText(/Automatisch\s+6 Wochen\s+im Voraus geplant/))

    await page.goto('/#/raids')
    await page.getByLabel('Raid-Ansicht').selectOption('upcoming')
    const calendar = page.getByRole('region', { name: 'Raidübersicht' })
    await expect(calendar.getByRole('link', { name: /^ICC-\d+$/ })).toHaveCount(6)
    await snapshot(page, testInfo, '05-sechs-wochen-geplant')
    await signOut(page)
  })

  test('Mitglied meldet sich Wochen vorher für einen geplanten Raid an', async ({ page }, testInfo) => {
    await signIn(page, member)
    await page.goto('/#/raids')
    await page.getByRole('region', { name: 'Raidübersicht' }).getByRole('link', { name: 'ICC-4' }).click()
    await page.getByRole('button', { name: 'Anmeldungen' }).click()
    await expect(page.getByText('Anmeldung offen')).toBeVisible()
    await page.getByRole('checkbox', { name: /Funkenflug/ }).check()
    await page.getByRole('button', { name: 'Ausgewählte Charaktere anmelden' }).click()
    await expect(page.getByText(/Funkenflug · Magier · Angemeldet/)).toBeVisible()
    await snapshot(page, testInfo, '06-mitglied-angemeldet', page.getByRole('heading', { name: 'Charakteroptionen nach Spieler' }))
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
})
