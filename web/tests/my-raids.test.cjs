const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

function load(relative, overrides = {}) {
  const filename = path.resolve(__dirname, '../src', relative)
  const module = { exports: {} }
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    fileName: filename,
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  vm.runInThisContext(`(function(require,module,exports){${source}\n})`, { filename })((name) => overrides[name], module, module.exports)
  return module.exports
}

const { myRaidStatus, quickSignupChoice, upcomingRaids } = load('myRaids.ts', { './characterOptions': { canonicalClass: (value) => value } })
const entry = (extra) => ({ id: 1, player_id: 7, character_id: 3, character_name: 'Funkenflug', class: 'Magier', spec: 'Feuer', is_withdrawn: false, is_accepted: false, attendance_id: 0, ...extra })
const overview = (extra) => ({ absences: [], entries: [], is_enabled: true, is_open: true, class_specs: { Magier: ['Arkan', 'Feuer', 'Frost'] }, ...extra })

test('status follows absence, then acceptance, then signup', () => {
  assert.equal(myRaidStatus(overview(), 7).state, 'open')
  assert.equal(myRaidStatus(overview({ is_open: false }), 7).state, 'closed')
  const signed = myRaidStatus(overview({ entries: [entry()] }), 7)
  assert.deepEqual([signed.state, signed.label, signed.detail], ['signed', 'Angemeldet', 'Funkenflug (Feuer)'])
  assert.equal(myRaidStatus(overview({ entries: [entry({ is_withdrawn: true })] }), 7).state, 'open')
  assert.equal(myRaidStatus(overview({ entries: [entry({ player_id: 8 })] }), 7).state, 'open', 'other players do not count')
  assert.equal(myRaidStatus(overview({ entries: [entry({ is_accepted: true })] }), 7).label, 'Bestätigt')
  const absent = myRaidStatus(overview({ entries: [entry({ is_accepted: true })], absences: [{ player_id: 7, reason: 'Urlaub', created_at: '' }] }), 7)
  assert.deepEqual([absent.state, absent.reason], ['absent', 'Urlaub'])
  const unsure = myRaidStatus(overview({ entries: [entry()], absences: [{ player_id: 7, reason: 'Spätschicht', status: 'TENTATIVE', created_at: '' }] }), 7)
  assert.deepEqual([unsure.state, unsure.label, unsure.detail, unsure.reason], ['tentative', 'Unsicher', 'Funkenflug (Feuer)', 'Spätschicht'])
  assert.equal(myRaidStatus(overview({ absences: [{ player_id: 7, reason: '', created_at: '' }] }), 7).state, 'absent', 'absences without status are declines')
})

test('quick signup offers the main character with its main spec', () => {
  const specs = { Magier: ['Arkan', 'Feuer', 'Frost'], Priester: ['Disziplin', 'Heilig', 'Schatten'] }
  const chars = [
    { id: 1, name: 'Twink', class: 'Priester', main_spec: 'Heilig', is_main: false, active: true },
    { id: 2, name: 'Funkenflug', class: 'Magier', main_spec: 'Feuer', is_main: true, active: true },
  ]
  assert.deepEqual(quickSignupChoice(chars, specs), { character: chars[1], spec: 'Feuer' })
  assert.equal(quickSignupChoice([{ ...chars[1], is_main: false, main_spec: 'Unbekannt' }], specs).spec, 'Arkan', 'unknown main spec falls back to the first valid one')
  assert.equal(quickSignupChoice([{ ...chars[1], active: false }], specs), null)
  assert.equal(quickSignupChoice([], specs), null)
})

test('upcoming raids are active, not started and soonest first', () => {
  const now = Date.parse('2026-10-18T10:00:00Z')
  const raid = (id, start, extra) => ({ id, name: `R${id}`, status: 'PLANNED', archived_at: null, scheduled_start: start, ...extra })
  const raids = [
    raid(1, '2026-10-25T18:00:00Z'), raid(2, '2026-10-22T18:00:00Z'), raid(3, '2026-10-17T18:00:00Z'),
    raid(4, '2026-10-23T18:00:00Z', { status: 'CANCELLED' }), raid(5, '2026-10-24T18:00:00Z', { archived_at: '2026-10-01T00:00:00Z' }),
    raid(6, null), raid(7, '2026-11-01T18:00:00Z', { status: 'OPEN' }),
  ]
  assert.deepEqual(upcomingRaids(raids, now, 4).map((row) => row.id), [2, 1, 7])
  assert.deepEqual(upcomingRaids(raids, now, 1).map((row) => row.id), [2])
})
