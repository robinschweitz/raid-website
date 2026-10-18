const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')

const guide = fs.readFileSync(path.join(__dirname, '..', 'src', 'pages', 'Guide.tsx'), 'utf8')
const app = fs.readFileSync(path.join(__dirname, '..', 'src', 'App.tsx'), 'utf8')
const imageDir = path.join(__dirname, '..', 'public', 'anleitung')

test('every guide screenshot exists and no unused image is shipped', () => {
  const referenced = [...guide.matchAll(/<Shot name="([^"]+)"/g)].map((match) => `${match[1]}.webp`).sort()
  const shipped = fs.readdirSync(imageDir).filter((file) => file.endsWith('.webp')).sort()
  assert.ok(referenced.length > 20, 'the guide shows screenshots')
  assert.deepEqual([...new Set(referenced)], referenced, 'each screenshot is shown once')
  assert.deepEqual(referenced, shipped)
})

test('every chapter in the contents list has a section and every jump target exists', () => {
  const sections = [...guide.matchAll(/<Section id="([^"]+)"/g)].map((match) => match[1])
  const contents = [...guide.matchAll(/\{ id: '([^']+)', title: /g)].map((match) => match[1])
  assert.deepEqual(contents, sections)
  const firsts = [...guide.matchAll(/first: '([^']+)'/g)].map((match) => match[1])
  for (const id of firsts) assert.ok(sections.includes(id), `jump target ${id}`)
})

test('the guide is routed, in the navigation and readable without login', () => {
  assert.match(app, /parts\[0\] === 'anleitung'\) return <GuidePage \/>/)
  assert.match(app, /to="\/anleitung"><NavIcon name="info" \/>Anleitung<\/Link>/)
  assert.match(app, /!player && path !== '\/' && path !== '\/anleitung'/)
})
