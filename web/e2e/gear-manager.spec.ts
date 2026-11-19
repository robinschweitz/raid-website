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
  await page.goto('/#/gear-manager')
  await expect(page.getByLabel('Mein Charakter')).toHaveValue(String(characterID))
  const planner = page.frameLocator('iframe[title="BiS-Ausrüstung und 3D-Anprobe"]')
  await expect(planner.locator('#newchar, #deletechar')).toHaveCount(0)
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
  expect((await result).status()).toBe(201)
  await expect(planner.getByRole('button', {name:/^Auf der Wunschliste/}).first()).toBeDisabled()
  const helm = planner.locator('#gear-slot-0 input[type=checkbox]').first()
  const mesh = page.waitForResponse(r => r.ok() && r.url().endsWith('/data/mo3/331359.mo3'))
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
  expect(errors).toEqual([])
})
