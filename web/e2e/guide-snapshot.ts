import { expect, type Page, type TestInfo } from '@playwright/test'
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

// Only synthetic test data is captured. Account identifiers and all secret
// fields are masked; a login screen is never a guide screenshot.
export async function guideSnapshot(page: Page, info: TestInfo, name: string) {
  await expect(page.locator('main')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Anmelden', exact: true })).toHaveCount(0)
  await page.evaluate(() => document.fonts.ready)
  const mask = page.locator('input[type="password"], input[aria-label="Benutzername"], input[aria-label="Reset-Code"], input[aria-label="Code"]')
  const dir = path.join(import.meta.dirname, '..', 'e2e-screenshots', 'guide')
  mkdirSync(dir, { recursive: true })
  const png = await page.screenshot({ mask, animations: 'disabled' })
  const webp = await page.evaluate(async (source) => {
    const image = new Image()
    image.src = source
    await image.decode()
    const canvas = document.createElement('canvas')
    canvas.width = image.naturalWidth
    canvas.height = image.naturalHeight
    canvas.getContext('2d')!.drawImage(image, 0, 0)
    return canvas.toDataURL('image/webp', 0.85).split(',')[1]
  }, `data:image/png;base64,${png.toString('base64')}`)
  const file = path.join(dir, `${name}.webp`)
  writeFileSync(file, Buffer.from(webp, 'base64'))
  await info.attach(name, { path: file, contentType: 'image/webp' })
}
