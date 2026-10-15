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
