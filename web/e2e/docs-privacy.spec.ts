import { test, expect } from '@playwright/test'
import { guideSnapshot } from './guide-snapshot'
test('guide: public privacy page', async ({ page }, info) => {
  await page.goto('/#/datenschutz')
  await expect(page.getByRole('heading', { name: 'Datenschutzerklärung', exact: true })).toBeVisible()
  await expect(page.getByText('Lade…', { exact: true })).toHaveCount(0)
  await guideSnapshot(page, info, 'privacy-page')
})
