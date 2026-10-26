import { expect, test } from '@playwright/test'
import { admin, apiAs, createMember, signIn } from './fixtures'

test('addon loot asks for a boss difficulty and records loot and kill together', async ({ page, request }) => {
  const member = { username: 'addon-kill-member', password: 'addon-kill-password', name: 'Addon Kill', character: 'Importmage', characterClass: 'Magier', mainSpec: 'Feuer', secondSpec: 'Frost' }
  const { playerID, characterID } = await createMember(request, member)
  const api = await apiAs(request, admin)
  const actor = (await (await request.get('/api/v1/auth/me')).json()).data
  const raid = await api.post('/api/v1/raids', { name: 'Addon kill import', raid_leader_id: actor.id, status: 'OPEN', catalog_instance_id: '631', raid_size: 25 })
  await api.post(`/api/v1/raids/${raid.id}/attendance`, { player_id: playerID, character_id: characterID, role: 'DD' })
  const drops = (await (await request.get('/api/v1/loot-catalog/items?boss_id=36612&raid_size=25')).json()).data
  const drop = drops.find((row: { mode: string }) => row.mode === 'NORMAL')
  expect(drop).toBeTruthy()
  await signIn(page, admin)
  await page.goto(`/#/raids/${raid.id}`)
  await page.getByRole('button', { name: 'Loot', exact: true }).click()
  const panel = page.getByRole('region', { name: 'Loot aus Addon importieren' })
  await panel.getByLabel('Loot-Export').fill(`RS1|LOOT|${raid.id}|Recorder|1\n1|2|36612|${drop.item_id}|Importmage|given`)
  await panel.getByRole('button', { name: 'Vorschau', exact: true }).click()
  const mode = panel.getByRole('combobox', { name: /^Schwierigkeit für / })
  await expect(mode).toHaveCount(1)
  await expect(panel.getByRole('button', { name: '1 Gegenstand übernehmen' })).toBeDisabled()
  await mode.selectOption('NORMAL')
  await panel.getByRole('button', { name: '1 Gegenstand übernehmen' }).click()
  await expect(panel.getByRole('status', { name: 'Importergebnis' })).toContainText('Übernommen')
  const kills = (await (await request.get(`/api/v1/raids/${raid.id}/kills`)).json()).data
  expect(kills.some((row: { catalog_boss_id: string; difficulty: string }) => row.catalog_boss_id === '36612' && row.difficulty === 'NORMAL')).toBeTruthy()
})
