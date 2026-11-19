import { expect, test } from '@playwright/test'
import { admin, apiAs, createMember } from './fixtures'

const base = '/api/v1/gear-manager/'
test('Gear-Manager schützt Seite, Skripte und Modelldaten vor Gastzugriff', async ({request}) => {
  for (const file of ['', 'viewer.js', 'data/mo3/1.mo3', 'data/textures/1.webp']) {
    expect((await request.get(base + file)).status()).toBe(401)
  }
  await apiAs(request, admin)
  expect((await request.get(base + 'V17_Teststand/LocalViewerServer.ps1')).status()).toBe(404)
  if (!process.env.GEAR_MANAGER_DIR) expect((await request.get(base)).status()).toBe(503)
})

test('V17 lädt Figur und Helm vom eigenen Server', async ({page}) => {
  test.skip(!process.env.GEAR_MANAGER_DIR, 'Privates V17-Paket erforderlich')
  const errors: string[] = []
  page.on('pageerror', e => errors.push(e.message))
  page.on('request', r => {if (/^https?:/.test(r.url()) && new URL(r.url()).hostname !== '127.0.0.1') errors.push(r.url())})
  page.on('response', r => {if (r.status() >= 400) errors.push(`${r.status()} ${r.url()}`)})
  const account = {username:'gear-test', password:'gear-preview-test', name:'Gear-Test', character:'Testfigur', characterClass:'Paladin', mainSpec:'Schutz', secondSpec:'Heilig'}
  const {characterID} = await createMember(page.request, account)
  await apiAs(page.request, {username:account.username,password:account.password})
  await page.route('**/api/v1/players/*/characters', async route => {
    const response = await route.fetch()
    const body = await response.json()
    Object.assign(body.data.find((c: {id:number}) => c.id === characterID), {external_source:'rising-gods',external_profile_id:'12345'})
    await route.fulfill({response, json:body})
  })
  await page.route('**/api/v1/external/rising-gods/characters?*', route => route.fulfill({json:{data:[{profile_id:'12345',race:'Zwerg'}]}}))
  await page.goto('/#/gear-manager')
  await expect(page.getByLabel('Mein Charakter')).toHaveValue(String(characterID))
  const planner = page.frameLocator('iframe[title="BiS-Ausrüstung und 3D-Anprobe"]')
  await expect(planner.locator('#newchar, #deletechar')).toHaveCount(0)
  await expect(planner.locator('#charRace')).toHaveValue('3')
  await expect(planner.locator('#charRace')).toBeDisabled()
  await expect(planner.locator('#model_3d canvas')).toBeVisible()
  const frameHeight = () => page.locator('.gear-planner-frame').evaluate(el => el.clientHeight)
  const fullHeight = await frameHeight()
  await planner.locator('#itemSearch').fill('kein-passender-gegenstand')
  await expect.poll(frameHeight).toBeLessThan(fullHeight)
  await planner.locator('#itemSearch').fill('')
  await expect.poll(frameHeight).toBeGreaterThanOrEqual(fullHeight)
  expect(await planner.locator('html').evaluate(el => el.scrollHeight - el.clientHeight)).toBeLessThanOrEqual(1)
  const wish = planner.getByRole('button', {name:/^Zur Wunschliste/}).first()
  const result = page.waitForResponse(r => r.url().endsWith(`/characters/${characterID}/wishlist/items`) && r.request().method() === 'POST')
  await wish.click()
  const added = await result
  expect(added.status()).toBe(201)
  const {data: {item_id}} = await added.json()
  const remove = planner.getByRole('button', {name:/^Von Wunschliste entfernen/}).first()
  const failedURL = (await added.url()) + '/' + item_id
  await page.route(`**/wishlist/items/${item_id}`, route => route.fulfill({status:503}), {times:1})
  await remove.click()
  await expect(planner.locator('#wish-status')).toContainText('nicht entfernt')
  const deleted = page.waitForResponse(r => r.url().endsWith(`/characters/${characterID}/wishlist/items/${item_id}`) && r.request().method() === 'DELETE')
  await remove.click()
  expect((await deleted).ok()).toBeTruthy()
  await expect(remove).toHaveCount(0)
  const confirmation = planner.getByRole('button', {name:/^✓ Entfernt/}).first()
  await expect(confirmation).toBeDisabled()
  await expect(confirmation).toHaveCount(0)
  await expect(wish).toBeEnabled()
  const helm = planner.locator('#gear-slot-0 input[type=checkbox]').first()
  const mesh = page.waitForResponse(r => r.ok() && r.url().endsWith('/data/meta/armor/1/178592.json'))
  await helm.check()
  await mesh
  await expect(planner.locator('#viewerStatus')).toContainText('3D-Charakter geladen')
  const canvas = planner.locator('#model_3d canvas').first()
  await canvas.hover()
  const rect = await canvas.boundingBox()
  if (!rect) throw Error('3D-Ansicht fehlt')
  await page.mouse.down()
  await page.mouse.move(rect.x + rect.width * .8, rect.y + rect.height * .5, {steps: 6})
  await page.mouse.up()
  await page.reload()
  await expect(helm).toBeChecked()
  expect(errors).toEqual([`503 ${failedURL}`])
})
