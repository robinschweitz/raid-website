import { expect, test } from '@playwright/test'

// ADR 083: the guide is reachable before an account exists, from the login card,
// and every screenshot it shows actually loads.
test('Anleitung ist ohne Login erreichbar und zeigt alle Bilder', async ({ page }, testInfo) => {
  await page.goto('/#/raids')
  await page.getByRole('link', { name: 'Anleitung mit Bildern lesen' }).click()
  await expect(page.getByRole('heading', { name: 'Anleitung', level: 1 })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Konto bekommen und einloggen' })).toBeVisible()

  await page.getByRole('button', { name: /Raidleitung/ }).click()
  await expect(page.getByRole('heading', { name: 'Raids anlegen und vorausplanen' })).toBeInViewport()

  const images = page.locator('.guide-shot img')
  const count = await images.count()
  expect(count).toBeGreaterThan(20)
  for (let index = 0; index < count; index += 1) {
    const image = images.nth(index)
    await image.scrollIntoViewIfNeeded()
    await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.complete && element.naturalWidth)).toBe(1280)
  }
  await page.getByRole('heading', { name: 'Anleitung', level: 1 }).scrollIntoViewIfNeeded()
  await testInfo.attach('anleitung', { body: await page.screenshot(), contentType: 'image/png' })
})
