import { expect, test } from '@playwright/test'
import { admin, signIn, snapshot } from './fixtures'

// ADR 088: players without a second factor see a thin reminder that leads to the
// account-security explainer and from there straight to their account tab.
test('MFA-Hinweis führt zur Erklärung und ins eigene Konto', async ({ page }, testInfo) => {
  await signIn(page, admin)
  const reminder = page.getByRole('note', { name: 'Hinweis zur Kontosicherheit' })
  await expect(reminder).toContainText('nur mit einem Passwort geschützt')
  await snapshot(page, testInfo, 'mfa-hinweis')

  await reminder.getByRole('link').click()
  await expect(page.getByRole('heading', { name: 'Warum ein zweiter Faktor?', level: 1 })).toBeVisible()
  await expect(reminder).toBeHidden()
  await expect(page.getByText('Dein Konto hat noch keinen zweiten Faktor.')).toBeVisible()
  await expect(page.getByRole('link', { name: /One simple action you can take/ })).toHaveAttribute('href', /microsoft\.com/)
  await snapshot(page, testInfo, 'kontosicherheit')
  await snapshot(page, testInfo, 'kontosicherheit-quellen', page.getByRole('heading', { name: 'Quellen' }))

  await page.getByRole('link', { name: 'Jetzt einrichten' }).click()
  await expect(page.getByRole('heading', { name: 'Multi-Faktor-Authentifizierung' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Authenticator einrichten' })).toBeVisible()

  await page.goto('/#/')
  await expect(reminder).toBeVisible()
  await reminder.getByRole('button', { name: 'Hinweis für diese Sitzung ausblenden' }).click()
  await expect(reminder).toBeHidden()
  await page.reload()
  await expect(page.getByRole('button', { name: 'Abmelden' })).toBeVisible()
  await expect(reminder).toBeHidden()
})

test('Kontosicherheit ist ohne Login lesbar', async ({ page }) => {
  await page.goto('/#/kontosicherheit')
  await expect(page.getByRole('heading', { name: 'Warum ein zweiter Faktor?', level: 1 })).toBeVisible()
  await expect(page.getByText('Melde dich an, um zu sehen')).toBeVisible()
  await expect(page.getByRole('note', { name: 'Hinweis zur Kontosicherheit' })).toHaveCount(0)
})
