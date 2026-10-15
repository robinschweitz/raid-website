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
  const localRequire = (name) => overrides[name] ?? require(path.resolve(path.dirname(filename), name))
  vm.runInThisContext(`(function(require,module,exports){${source}\n})`, { filename })(localRequire, module, module.exports)
  return module.exports
}

const grouping = { rosterGroup: (row) => row.group }
const { recruitmentNeeds, renderRecruitmentText } = load('recruitmentText.ts', { './raidRoster': grouping })

test('counts occupied spots, excludes bench and clamps overfilled groups', () => {
  const attendance = [
    { group: 'TANK', is_bench: false },
    { group: 'MELEE', is_bench: false },
    { group: 'RANGED', is_bench: false },
    { group: 'RANGED', is_bench: true },
    { group: 'UNKNOWN', is_bench: false },
  ]
  const result = recruitmentNeeds(attendance, { TANK: 2, HEAL: 2, MELEE: 0, RANGED: 3 })
  assert.deepEqual({ ...result.counts }, { TANK: 1, HEAL: 0, MELEE: 1, RANGED: 1 })
  assert.deepEqual({ ...result.missing }, { TANK: 1, HEAL: 2, MELEE: 0, RANGED: 2 })
  assert.equal(result.unclassified, 1)
})

test('updates all occurrences of placeholders and leaves custom wording intact', () => {
  const missing = { TANK: 1, HEAL: 0, MELEE: 5, RANGED: 3 }
  assert.equal(renderRecruitmentText('Noch {{TANK}} Tank, {{RDD}} RDD, {{MDD}} MDD! {{TANK}} Tank.', missing),
    'Noch 1 Tank, 3 RDD, 5 MDD! 1 Tank.')
  assert.equal(renderRecruitmentText('{{OTHER}}', missing), '{{OTHER}}')
})
