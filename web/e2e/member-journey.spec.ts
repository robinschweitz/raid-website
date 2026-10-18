import { expect, test } from '@playwright/test'
import { createMember, createPlannedSeries, signIn, snapshot } from './fixtures'

// ADR 081: what a regular member does every week — sign up, say they can't
// come, fill the wishlist and copy it into the addon — without hunting for it.
const sternfall = { username: 'e2e-sternfall', password: 'sternfall-password', name: 'Sternfall', character: 'Sternfall', characterClass: 'Priester', mainSpec: 'Schatten', secondSpec: 'Disziplin' }

test.describe.configure({ mode: 'serial' })

test.describe('Mitglied: Anmelden, Absagen, Wunschliste', () => {
  test.beforeAll(async ({ request }) => {
    await createMember(request, sternfall)
    await createPlannedSeries(request, 'Mittwochsraid', 'MW-{number}', 3, 3)
  })

  test('meldet sich direkt auf der Übersicht an und sagt einen Raid ab', async ({ page }, testInfo) => {
    await signIn(page, sternfall)
    // Signing in from the login link lands on the overview, not on "Seite nicht gefunden".
    await expect(page.getByRole('heading', { name: 'Meine nächsten Raids' })).toBeVisible()
    await expect(page.getByText('Seite nicht gefunden.')).toHaveCount(0)
    const first = page.getByRole('listitem', { name: 'MW-1' })
    const second = page.getByRole('listitem', { name: 'MW-2' })
    await expect(first.getByText('Noch nicht angemeldet')).toBeVisible()
    await snapshot(page, testInfo, 'nachher-1-uebersicht-meine-raids')

    await first.getByRole('button', { name: 'Mit Sternfall (Schatten) anmelden' }).click()
    await expect(first.getByText('Angemeldet', { exact: true })).toBeVisible()
    await expect(first.getByText('mit Sternfall (Schatten)')).toBeVisible()

    await second.getByRole('button', { name: 'Kann nicht kommen' }).click()
    await second.getByLabel('Grund (optional)').fill('Urlaub')
    await snapshot(page, testInfo, 'nachher-2-absage-mit-grund', page.getByRole('heading', { name: 'Meine nächsten Raids' }))
    await second.getByRole('button', { name: 'Absage senden' }).click()
    await expect(second.getByText('Abgesagt', { exact: true })).toBeVisible()
    await expect(second.getByText('· Urlaub')).toBeVisible()
    await snapshot(page, testInfo, 'nachher-3-angemeldet-und-abgesagt', page.getByRole('heading', { name: 'Meine nächsten Raids' }))

    await second.getByRole('button', { name: 'Doch dabei' }).click()
    await expect(second.getByText('Noch nicht angemeldet')).toBeVisible()
  })

  test('ein Raid öffnet für Mitglieder direkt bei den Anmeldungen', async ({ page }, testInfo) => {
    await signIn(page, sternfall)
    await page.getByRole('listitem', { name: 'MW-1' }).getByRole('link', { name: 'MW-1' }).click()
    await expect(page.getByRole('button', { name: 'Anmeldungen', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByLabel('Dein Status')).toContainText('Angemeldet mit Sternfall (Schatten)')
    await snapshot(page, testInfo, 'nachher-4-raid-anmeldungen')

    // Also via the raid list, without the special link.
    await page.goto('/#/raids')
    await page.getByRole('region', { name: 'Raidübersicht' }).getByRole('link', { name: 'MW-3' }).click()
    await expect(page.getByRole('button', { name: 'Anmeldungen', exact: true })).toHaveAttribute('aria-pressed', 'true')
  })

  test('Wunschliste füllen und für das Addon kopieren', async ({ page, context }, testInfo) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await signIn(page, sternfall)
    await page.getByRole('navigation', { name: 'Hauptnavigation' }).getByRole('link', { name: 'Beute' }).click()
    const banner = page.locator('.catalog-wishlist-banner')
    await expect(banner).toContainText('Klick bei einem Item auf deinen Charakter')
    await page.getByPlaceholder('Name oder Item-ID').fill('Bryntroll')
    await page.locator('.catalog-wishlist').first().getByRole('button', { name: 'Sternfall' }).click()
    await expect(banner).toContainText('steht jetzt auf der Wunschliste von Sternfall')
    await snapshot(page, testInfo, 'nachher-5-beute-hinweis')

    await banner.getByRole('link', { name: /Meine Wunschliste/ }).click()
    await expect(page.getByRole('heading', { name: 'Meine Wunschliste' })).toBeVisible()
    const exportBox = page.getByLabel('Addon-Export')
    await expect(exportBox).toContainText('/wish import')
    await exportBox.getByRole('button', { name: 'Export kopieren' }).click()
    await expect(exportBox.getByText(/Kopiert/)).toBeVisible()
    const copied = await page.evaluate(() => navigator.clipboard.readText())
    expect(copied).toMatch(/^WL2;Sternfall;/)
    await snapshot(page, testInfo, 'nachher-6-wunschliste-export')
  })

  test('auf dem Handy', async ({ page }, testInfo) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await signIn(page, sternfall)
    await expect(page.getByRole('navigation', { name: 'Mobile Hauptnavigation' }).getByRole('link', { name: 'Wunschliste' })).toBeVisible()
    await snapshot(page, testInfo, 'nachher-7-handy-uebersicht')
  })
})
