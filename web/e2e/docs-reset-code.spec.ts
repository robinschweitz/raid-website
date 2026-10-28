import { test, expect } from '@playwright/test'
import { admin, apiAs, signIn } from './fixtures'
import { guideSnapshot } from './guide-snapshot'

test('guide: issue password reset code', async ({ page, request }, info) => {
  const api = await apiAs(request, admin)
  const player = await api.post('/api/v1/players', { name: 'Funkenflug' })
  await signIn(page, admin)
  await page.goto(`/#/players/${player.id}/konto`)
  await expect(page.getByRole('button', { name: 'Reset-Code erstellen', exact: true })).toBeVisible()
  await page.getByRole('heading', { name: 'Konto', exact: true }).evaluate((element) => element.scrollIntoView({ block: 'start' }))
  await guideSnapshot(page, info, '52-spieler-konto-admin')
})
