import { expect, test } from '@playwright/test'
import { admin, apiAs, createMember, signIn } from './fixtures'

test('the loot form records an item that went to nobody together with its kill', async ({ page, request }, testInfo) => {
  const member = { username: `disenchanter-${testInfo.retry}`, password: 'disenchanter-password', name: 'Entzauberer', character: 'Staubmagier', characterClass: 'Magier', mainSpec: 'Feuer', secondSpec: 'Frost' }
  const { playerID, characterID } = await createMember(request, member)
  const api = await apiAs(request, admin)
  const actor = (await (await request.get('/api/v1/auth/me')).json()).data
  const raid = await api.post('/api/v1/raids', { name: 'Loot ohne Empfänger', raid_leader_id: actor.id, status: 'OPEN', scheduled_start: '2020-01-02T19:00:00Z', catalog_instance_id: '631', raid_size: 25 })
  await api.post(`/api/v1/raids/${raid.id}/attendance`, { player_id: playerID, character_id: characterID, role: 'DD' })
  const drops = (await (await request.get('/api/v1/loot-catalog/items?boss_id=36612&raid_size=25')).json()).data
  const drop = drops.find((row: { mode: string }) => row.mode === 'NORMAL')
  expect(drop).toBeTruthy()

  await signIn(page, admin)
  await page.goto(`/#/raids/${raid.id}`)
  await page.getByRole('button', { name: /^Loot \(\d+\)$/ }).click()
  await page.getByText('Loot erfassen', { exact: true }).click()
  await expect(page.getByLabel('Loot-Boss', { exact: true }).locator('option[value="36612"]')).toHaveCount(1)
  await page.getByLabel('Loot-Boss', { exact: true }).selectOption('36612')
  await expect(page.getByLabel('Loot-Modus', { exact: true })).toBeEnabled()
  await page.getByLabel('Loot-Modus', { exact: true }).selectOption('NORMAL')
  await page.getByLabel('Loot-Item', { exact: true }).fill(String(drop.item_id))

  await page.getByLabel('Loot-Vergabemethode', { exact: true }).selectOption('DISENCHANTED')
  await expect(page.getByRole('button', { name: 'Eintragen', exact: true })).toBeDisabled()
  await expect(page.getByLabel('Loot-Verbleib bei', { exact: true })).toHaveCount(0)
  await page.getByLabel('Loot-Empfänger', { exact: true }).fill(String(characterID))
  await page.getByRole('button', { name: 'Eintragen', exact: true }).click()

  const list = page.getByRole('region', { name: 'Beute ohne Empfänger' })
  await expect(list.getByRole('cell', { name: 'Entzaubert', exact: true })).toBeVisible()
  await expect(list.getByRole('cell', { name: 'Staubmagier', exact: true })).toBeVisible()
  await expect(page.getByText('Loot ohne Empfänger gespeichert.')).toBeVisible()
  await expect(page.getByLabel('Loot-Vergabemethode', { exact: true })).toHaveValue('MAIN')
  await expect(page.getByLabel('Loot-Empfänger', { exact: true })).toHaveCount(1)
  const kills = page.getByRole('region', { name: 'Bosskills' })
  await expect(kills.getByRole('cell', { name: /Mark/ })).toBeVisible()
  await expect(kills.getByRole('combobox')).toHaveValue('NORMAL')
  await expect(page.locator('main')).toContainText(drop.name)
  await expect(page.getByRole('button', { name: 'Loot (0)', exact: true })).toBeVisible()

  await page.getByLabel('Loot-Vergabemethode', { exact: true }).selectOption('BANK')
  await expect(page.getByLabel('Loot-Empfänger', { exact: true })).toHaveCount(0)
  await expect(page.getByLabel('Loot-Verbleib bei', { exact: true })).toBeVisible()

  await page.goto('/#/')
  await expect(page.getByText('1 von 12 getötet')).toBeVisible()
  await expect(page.locator('.boss-icon.killed')).toHaveCount(1)
  await api.post(`/api/v1/raids/${raid.id}/cancel`, {})
})
