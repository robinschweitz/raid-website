import { test, expect } from '@playwright/test'
import { admin, signIn } from './fixtures'
import { guideSnapshot } from './guide-snapshot'
test('guide: recover undelivered registration codes', async ({ page }, info) => {
  await signIn(page, admin)
  await page.goto('/#/registration-requests')
  await expect(page.getByText('Bereits erstellte Codes können nicht erneut angezeigt werden', { exact: false })).toBeVisible()
  await guideSnapshot(page, info, '53-registrierung')
})
