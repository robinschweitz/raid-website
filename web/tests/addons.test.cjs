const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')

// The addons pages (ADR 085).
const page = fs.readFileSync(path.join(__dirname, '..', 'src', 'pages', 'Addons.tsx'), 'utf8')
const app = fs.readFileSync(path.join(__dirname, '..', 'src', 'App.tsx'), 'utf8')
const imageDir = path.join(__dirname, '..', 'public', 'addons')

test('every addon screenshot exists and no unused image is shipped', () => {
  const shots = [...page.matchAll(/<Shot name="([^"]+)"/g)].map((match) => match[1])
  assert.deepEqual([...new Set(shots)], shots, 'each screenshot is shown once as a figure')
  const cards = [...page.matchAll(/src="\/addons\/([^"]+)\.webp"/g)].map((match) => match[1])
  const referenced = [...new Set([...shots, ...cards])].map((name) => `${name}.webp`).sort()
  const shipped = fs.readdirSync(imageDir).filter((file) => file.endsWith('.webp')).sort()
  assert.deepEqual(referenced, shipped)
})

test('every contents entry jumps to a section of the page', () => {
  const ids = [...page.matchAll(/id="([^"]+)" aria-labelledby/g)].map((match) => match[1])
  const contents = [...page.matchAll(/\{ id: '([^']+)', title: /g)].map((match) => match[1])
  assert.ok(contents.length >= 8)
  for (const id of contents) assert.ok(ids.includes(id), `section ${id}`)
})

test('downloads come from the server, never a hard-coded version', () => {
  assert.match(page, /api\.get<AddonDownload\[\]>\('\/api\/v1\/addons'\)/)
  assert.match(page, /href=\{addon\.download_url\}/)
  assert.doesNotMatch(page, /\d+\.\d+\.\d+\.zip|github\.com\/robinschweitz\/wow-addons/)
})

test('the addons pages are routed, in the navigation and readable without login', () => {
  assert.match(app, /parts\[0\] === 'addons'\) return <AddonsPage addonID=\{parts\[1\]\} \/>/)
  assert.match(app, /to="\/addons"><NavIcon name="download" \/>Addons<\/Link>/)
  assert.match(app, /path !== '\/addons' && !path\.startsWith\('\/addons\/'\)/)
})
