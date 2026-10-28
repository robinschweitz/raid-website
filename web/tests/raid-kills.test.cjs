const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const srcDir = path.resolve(__dirname, '../src')

function load(file, requires) {
  const source = ts.transpileModule(fs.readFileSync(path.join(srcDir, file), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const module = { exports: {} }
  const jsx = (type, props, key) => ({ type, props, key })
  new Function('require', 'module', 'exports', source)((name) => {
    if (name === 'react/jsx-runtime') return { jsx, jsxs: jsx }
    if (requires[name]) return requires[name]
    throw new Error(`Unexpected import ${name}`)
  }, module, module.exports)
  return module.exports
}

const text = (node) => Array.isArray(node) ? node.map(text).join('') : node && typeof node === 'object' ? text(node.props?.children) : node == null || typeof node === 'boolean' ? '' : String(node)
const find = (node, test, found = []) => {
  if (Array.isArray(node)) node.forEach((child) => find(child, test, found))
  else if (node && typeof node === 'object') { if (test(node)) found.push(node); find(node.props?.children, test, found) }
  return found
}

const { BossProgression } = load('components/BossProgression.tsx', {
  './bossData': { getBossesForInstance: () => [{ name: 'Lord Mark’gar', icon: 'a', id: 'ICCLordMarrowgar' }, { name: 'Professor Seuchenfaust', icon: 'b', id: 'ICCPutricide' }] },
  '../types': { catalogModeLabels: { NORMAL: 'Normal', HEROIC: 'Heroisch', HARDMODE: 'Hardmode' } },
})

test('TestReqRaid018_AC01 recorded kills mark their boss and show the difficulty', () => {
  const tree = BossProgression({ catalogInstanceId: '631', kills: [{ id: 1, boss: 'Professor Seuchenmord', catalog_boss_id: 'ICCPutricide', difficulty: 'HEROIC' }] })
  const killed = find(tree, (n) => n.type === 'li' && /killed/.test(n.props.className))
  assert.equal(killed.length, 1)
  assert.match(text(killed[0]), /Professor Seuchenfaust · getötet \(Heroisch\)/)
  assert.match(text(tree), /1 von 2 getötet/)
})

test('TestReqRaid018_AC02 a raid without kills says none are recorded and marks nothing', () => {
  const tree = BossProgression({ catalogInstanceId: '631', kills: [] })
  assert.equal(find(tree, (n) => n.type === 'li' && /killed/.test(n.props.className)).length, 0)
  assert.match(text(tree), /Keine Kills erfasst/)
  assert.doesNotMatch(text(BossProgression({ catalogInstanceId: '631' })), /getötet|Keine Kills/)
})

test('TestReqRaid018_AC03 manage controls render only for staff', () => {
  const source = fs.readFileSync(path.join(srcDir, 'components/RaidKills.tsx'), 'utf8')
  const guarded = ['kills/${kill.id}`, { difficulty', 'remove`', 'Kill eintragen</summary>']
  for (const marker of guarded) {
    const at = source.indexOf(marker)
    assert.ok(at > 0, marker)
    assert.match(source.slice(Math.max(0, at - 400), at), /canWrite/)
  }
  assert.match(source, /Für diesen Raid sind keine Kills erfasst/)
})

test('TestReqRaid018_AC01 every ICC encounter carries the ID of a catalog boss, so differently spelled names still match', () => {
  const source = fs.readFileSync(path.join(srcDir, 'components/bossData.ts'), 'utf8')
  const exports = {}
  new Function('exports', ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText)(exports)
  const catalog = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../scripts/data/catalog-wotlk335.json'), 'utf8'))
  const catalogNames = new Map()
  const walk = (node) => {
    if (Array.isArray(node)) node.forEach(walk)
    else if (node && typeof node === 'object') {
      if (typeof node.id === 'string' && /^ICC/.test(node.id) && node.names && node.names.deDE) catalogNames.set(node.id, node.names.deDE)
      Object.values(node).forEach(walk)
    }
  }
  walk(catalog)
  const bosses = exports.getBossesForInstance('631')
  assert.equal(bosses.length, 12)
  for (const boss of bosses) assert.ok(catalogNames.has(boss.id), `${boss.name}: ${boss.id}`)
  assert.equal(new Set(bosses.map((boss) => boss.id)).size, 12)
  for (const id of ['ICCLordMarrowgar', 'ICCLanathel', 'ICCGunshipBattle']) assert.ok(bosses.some((boss) => boss.id === id), id)
})
