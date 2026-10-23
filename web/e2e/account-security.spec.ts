import { expect, test } from '@playwright/test'
import { admin, apiAs, createMember, signIn, snapshot } from './fixtures'

// ADR 092: players without a second factor see a thin reminder that leads to the
// account-security explainer and from there straight to their account tab.
const reader = { username: 'e2e-sicherheit', password: 'sicherheit-password', name: 'Sicherheit', character: 'Wachsam', characterClass: 'Priester', mainSpec: 'Heilig', secondSpec: 'Disziplin' }

test('MFA-Hinweis führt zur Erklärung und ins eigene Konto', async ({ page, request }, testInfo) => {
  await createMember(request, reader)
  await signIn(page, reader)
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

// FR-3.10, SC-040: an Admin without a second factor lands on the account tab, sees a
// notice that cannot be hidden, and Admin pages say why they are locked.
test('Admin ohne zweiten Faktor wird zur Einrichtung geschickt', async ({ page, request }, testInfo) => {
  const newAdmin = { username: 'e2e-admin-ohne-mfa', password: 'admin-ohne-mfa-password', name: 'Neuer Admin', character: 'Wachturm', characterClass: 'Krieger', mainSpec: 'Schutz', secondSpec: 'Waffen' }
  const { playerID } = await createMember(request, newAdmin)
  const api = await apiAs(request, admin)
  await api.put(`/api/v1/players/${playerID}/account`, { username: newAdmin.username, password: newAdmin.password, role: 'ADMIN' })

  await signIn(page, newAdmin)
  const notice = page.getByRole('alert').filter({ hasText: 'Als Admin brauchst du einen zweiten Faktor' })
  await expect(notice).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`#/players/${playerID}/konto$`))
  await expect(page.getByRole('button', { name: 'Authenticator einrichten' })).toBeVisible()
  await expect(notice.getByRole('button')).toHaveCount(0)
  await snapshot(page, testInfo, 'admin-mfa-pflicht')

  await page.goto('/#/audit-log')
  await expect(page.getByText('Als Admin musst du zuerst einen zweiten Faktor')).toBeVisible()
})
