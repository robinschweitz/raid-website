import { expect, test } from '@playwright/test'
import { admin, apiAs } from './fixtures'

test('the encrypted e2e TOTP fixture supports Admin login', async ({ request }) => {
  await apiAs(request, admin)
  const response = await request.get('/api/v1/auth/me')
  expect(response.status()).toBe(200)
  expect((await response.json()).data.username).toBe(admin.username)
})
