const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

test('raid encounters have correct totals, unique names and bundled artwork', () => {
  const source = fs.readFileSync(path.join(__dirname, '../src/components/bossData.ts'), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const exports = {}
  new Function('exports', compiled)(exports)
  for (const [instance, count] of Object.entries({ 533: 15, 603: 14, 624: 4, 631: 12, 649: 5, 724: 4 })) {
    const bosses = exports.getBossesForInstance(instance)
    assert.equal(bosses.length, count)
    assert.equal(new Set(bosses.map(boss => boss.name)).size, count)
    for (const boss of bosses) {
      const image = fs.readFileSync(path.join(__dirname, `../public/bosses/${boss.icon}.webp`))
      assert.equal(image.toString('ascii', 8, 12), 'WEBP')
    }
  }
  assert.deepEqual(exports.getBossesForInstance(null), [])
  assert.deepEqual(exports.getBossesForInstance('unknown'), [])
})
