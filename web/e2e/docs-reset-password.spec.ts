import { test, expect } from '@playwright/test'
import { guideSnapshot } from './guide-snapshot'
test('guide: redeem password reset code', async ({ page }, info) => {
  await page.goto('/#/passwort-zuruecksetzen')
  await expect(page.getByRole('heading', { name: 'Passwort zurücksetzen', exact: true })).toBeVisible()
  await guideSnapshot(page, info, '23-passwort-zuruecksetzen')
})
