import { expect, test } from '@playwright/test'
import { admin, apiAs } from './fixtures'

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
  await apiAs(page.request,admin)
  await page.goto(base)
  page.on('dialog', d => d.accept('Testfigur'))
  await page.locator('#newchar').click()
  await expect(page.locator('#model_3d canvas')).toBeVisible()
  const helm = page.locator('#gear-slot-0 input[type=checkbox]').first()
  const mesh = page.waitForResponse(r => r.ok() && r.url().endsWith('/data/mo3/331359.mo3'))
  await helm.check()
  await mesh
  await expect(page.locator('#viewerStatus')).toContainText('3D-Charakter geladen')
  const canvas = page.locator('#model_3d canvas').first()
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
