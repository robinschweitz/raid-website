import { expect, test } from '@playwright/test'
import { admin, apiAs, createMember, signIn } from './fixtures'

test('a player redeems a reset code and signs in with the new password', async ({ page, request }) => {
  const member = { username: 'reset-journey', password: 'initial-password', name: 'Reset Journey', character: 'Resetmage', characterClass: 'Magier', mainSpec: 'Feuer', secondSpec: 'Frost' }
  const { playerID } = await createMember(request, member)
  const api = await apiAs(request, admin)
  const generated = await api.post(`/api/v1/players/${playerID}/reset-code`, {})
  await page.goto('/#/passwort-zuruecksetzen')
  await page.getByLabel('Reset-Code', { exact: true }).fill(generated.code)
  await page.getByLabel('Neues Passwort', { exact: true }).fill('replacement-password')
  await page.getByRole('button', { name: 'Passwort zurücksetzen', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Dein Passwort wurde geändert')
  await signIn(page, { username: member.username, password: 'replacement-password' })
  await expect(page.getByRole('button', { name: 'Abmelden' })).toBeVisible()
})
