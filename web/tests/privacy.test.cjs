const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')

// The privacy page (FR-30.1, FR-30.4, FR-30.8).
const page = fs.readFileSync(path.join(__dirname, '..', 'src', 'pages', 'Privacy.tsx'), 'utf8')
const app = fs.readFileSync(path.join(__dirname, '..', 'src', 'App.tsx'), 'utf8')

test('TestReqPrivacy010_AC01 the page is routed, public, and reads the contact from the API', () => {
  assert.match(app, /parts\[0\] === 'datenschutz'\) return <PrivacyPage \/>/)
  assert.match(app, /path !== '\/datenschutz'/)
  assert.match(page, /api\.get<PrivacyContact>\('\/api\/v1\/privacy-contact'\)/)
  assert.match(page, /contact\.name/)
  assert.match(page, /contact\.address/)
})

test('TestReqPrivacy010_AC02 the page explains audit retention and pseudonymisation', () => {
  assert.match(page, /ohne zeitliche Begrenzung/)
  assert.match(page, /Nur Administratoren können das Protokoll lesen/)
  assert.match(page, /Charakternamen/)
  assert.match(page, /nicht anonym/)
  assert.match(page, /Pseudonymisierung/)
  assert.doesNotMatch(page, /dangerouslySetInnerHTML/)
})
