const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

function load(relative, overrides) {
  const filename = path.resolve(__dirname, '../src', relative)
  const mod = { exports: {} }
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    fileName: filename,
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  vm.runInThisContext(`(function(require,module,exports){${source}\n})`, { filename })((name) => overrides[name], mod, mod.exports)
  return mod.exports
}

const tokens = {
  Paladin: 'class-paladin', Krieger: 'class-warrior', Schurke: 'class-rogue',
  Jäger: 'class-hunter', Hexer: 'class-warlock', Magier: 'class-mage',
}
const raidSetup = load('raidSetup.ts', {
  './raidRoster': { rosterGroup: (row) => row.group },
  './characterTheme': { characterClassName: (name) => tokens[name] || 'unknown' },
  './recruitmentText': { emptyTargets: () => ({ TANK: 0, HEAL: 0, MELEE: 0, RANGED: 0 }) },
})

test('specific spec is not consumed by flexible slot, and bench is excluded', () => {
  const setup = [['Irgendein MDD', 'Paladin Vergeltung', 'Leer', 'Leer', 'Leer']]
  const rows = [
    { class: 'Paladin', spec: 'Vergeltung', group: 'MELEE', is_bench: false },
    { class: 'Krieger', spec: 'Furor', group: 'MELEE', is_bench: true },
  ]
  const result = raidSetup.matchRaidSetup(setup, rows)
  assert.deepEqual(result.missingSlots.map((slot) => slot.label), ['Irgendein MDD'])
  assert.equal(result.missing.MELEE, 1)
})

test('duplicates require separate participants', () => {
  const setup = [['Paladin Vergeltung', 'Paladin Vergeltung', 'Leer', 'Leer', 'Leer']]
  const rows = [{ class: 'Paladin', spec: 'Vergeltung', group: 'MELEE', is_bench: false }]
  const result = raidSetup.matchRaidSetup(setup, rows)
  assert.equal(result.missingSlots.length, 1)
})

test('ICC has five groups of five slots', () => {
  assert.equal(raidSetup.iccIdealSetup.length, 5)
  assert.ok(raidSetup.iccIdealSetup.every((group) => group.length === 5))
})

test('short setup summary groups missing slots by role with chat short names', () => {
  const missingSlots = ['Druide Wiederherstellung', 'Priester Disziplin', 'Irgendein MDD', 'Irgendein MDD', 'Magier Feuer', 'Priester Schatten'].map((label) => ({ label }))
  assert.equal(raidSetup.shortSetupSummary(missingSlots), '2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage, SP)')
  assert.equal(raidSetup.shortSetupSummary([{ label: 'Offtank' }]), '1 Tank (OT)')
  assert.equal(raidSetup.shortSetupSummary([]), 'keine offenen Plätze')
})

test('specific wishes come before "beliebig" and duplicates are counted', () => {
  const missingSlots = ['Irgendein RDD', 'Druide Eule', 'Druide Eule', 'Priester Schatten'].map((label) => ({ label }))
  assert.equal(raidSetup.missingSpecs(missingSlots, 'RANGED'), '2x Eule, SP, beliebig')
  assert.equal(raidSetup.missingSpecs(missingSlots, 'HEAL'), '')
})

test('setup placeholders let the template choose the wording', () => {
  const missingSlots = [{ label: 'Druide Wiederherstellung' }, { label: 'Magier Feuer' }]
  assert.equal(raidSetup.renderSetupPlaceholders('LFM {{SETUP_KURZ}}', missingSlots), 'LFM 1 Heal (Baum), 1 RDD (Mage)')
  assert.equal(raidSetup.renderSetupPlaceholders('Heiler: {{HEAL_SPECS}} / Tanks: {{TANK_SPECS}}', missingSlots), 'Heiler: Baum / Tanks: -')
  assert.equal(raidSetup.renderSetupPlaceholders('{{SETUP}}', missingSlots), 'Druide Wiederherstellung, Magier Feuer')
})
