import { expect, test } from '@playwright/test'
import { apiAs, createMember } from './fixtures'

test('initial account fixtures do not use direct Admin password assignment', async ({ request }) => {
  const member = { username: 'fixture-api-user', password: 'fixture-api-password', name: 'Fixture API', character: 'Fixturemage', characterClass: 'Magier', mainSpec: 'Feuer', secondSpec: 'Frost' }
  await createMember(request, member)
  await apiAs(request, { username: member.username, password: member.password })
  expect((await (await request.get('/api/v1/auth/me')).json()).data.username).toBe(member.username)
})
