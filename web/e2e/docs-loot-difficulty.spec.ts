import { test, expect } from '@playwright/test'
import { admin, apiAs, signIn } from './fixtures'
import { guideSnapshot } from './guide-snapshot'
test('guide: record loot with a boss difficulty', async ({ page, request }, info) => {
  const api = await apiAs(request, admin)
  const actor = (await (await request.get('/api/v1/auth/me')).json()).data
  const raid = await api.post('/api/v1/raids', { name: 'ICC25 Donnerstag', raid_leader_id: actor.id, catalog_instance_id: '631', raid_size: 25 })
  await signIn(page, admin)
  await page.goto(`/#/raids/${raid.id}`)
  await page.getByRole('button', { name: /^Loot \(\d+\)$/ }).click()
  await page.getByText('Loot erfassen', { exact: true }).click()
  await page.getByLabel('Loot-Boss', { exact: true }).selectOption('36612')
  await expect(page.getByLabel('Loot-Modus', { exact: true })).toBeEnabled()
  await page.getByLabel('Loot-Boss', { exact: true }).scrollIntoViewIfNeeded()
  await guideSnapshot(page, info, '37-loot')
})
