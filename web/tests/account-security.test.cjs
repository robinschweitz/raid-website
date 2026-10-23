const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')

// The account-security explainer and MFA reminder (ADR 092).
const page = fs.readFileSync(path.join(__dirname, '..', 'src', 'pages', 'AccountSecurity.tsx'), 'utf8')
const app = fs.readFileSync(path.join(__dirname, '..', 'src', 'App.tsx'), 'utf8')

test('every cited source exists and every listed source is cited', () => {
  const listed = [...page.matchAll(/^\s+id: '([^']+)',$/gm)].map((match) => match[1])
  const cited = [...new Set([...page.matchAll(/<(?:Cite id|Stat value="[^"]+" source)="([^"]+)"/g)].map((match) => match[1]))]
  assert.ok(listed.length >= 5)
  assert.deepEqual([...new Set(listed)], listed, 'source ids are unique')
  for (const id of cited) assert.ok(listed.includes(id), `cited source ${id} is listed`)
  for (const id of listed) assert.ok(cited.includes(id), `listed source ${id} is cited`)
})

test('every statistic carries a source and sources are https links', () => {
  const stats = [...page.matchAll(/<Stat value="([^"]+)"( source="[^"]+")?/g)]
  assert.ok(stats.length >= 4)
  for (const stat of stats) assert.ok(stat[2], `statistic ${stat[1]} has a source`)
  for (const url of page.matchAll(/url: '([^']+)'/g)) assert.match(url[1], /^https:\/\//)
  assert.match(page, /microsoft\.com\/en-us\/security\/blog\/2019\/08\/20\/one-simple-action/)
  assert.match(page, /security\.googleblog\.com\/2019\/05\/new-research-how-effective-is-basic/)
})

test('the page is routed, public, and the banner sits in the shell', () => {
  assert.match(app, /parts\[0\] === 'kontosicherheit'\) return <AccountSecurityPage \/>/)
  assert.match(app, /path !== '\/kontosicherheit' && path !== '\/addons'/)
  assert.match(app, /<MFAReminderBanner isSignedIn=\{player !== null\} \/>/)
  assert.match(app, /initialView=\{parts\[2\] === 'konto' \? 'account' : undefined\}/)
  assert.match(page, /to=\{`\/players\/\$\{player\.id\}\/konto`\}/)
})
