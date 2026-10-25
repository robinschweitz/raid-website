const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')
const crypto = require('node:crypto')

// Exercise the real component handlers with deferred requests, without a DOM dependency.
// This small hook runner models render/commit and keyed remounts, not browser behavior.
function setup(me = { id: 1, role: 'ADMIN' }) {
  let current
  const cache = new Map()
  const requests = []
  const navigations = []
  const prompts = []
  const confirms = []
  const confirmAnswers = []
  const intervals = []
  const auth = { player: me }
  const jsx = (type, props, key) => ({ type, props: props ?? {}, key })
  const slot = (create) => {
    const index = current.index++
    return current.slots[index] ?? (current.slots[index] = create())
  }
  const react = {
    useState(initial) {
      const owner = current
      const state = slot(() => ({ value: typeof initial === 'function' ? initial() : initial }))
      return [state.value, (value) => {
        if (!owner.active) return
        state.value = typeof value === 'function' ? value(state.value) : value
      }]
    },
    useRef(initial) { return slot(() => ({ current: initial })) },
    useCallback(fn, deps) {
      const state = slot(() => ({}))
      if (!state.deps || deps.some((value, i) => !Object.is(value, state.deps[i]))) {
        state.deps = deps
        state.fn = fn
      }
      return state.fn
    },
    useEffect(effect, deps) {
      const owner = current
      const state = slot(() => ({}))
      if (!state.deps || deps.some((value, i) => !Object.is(value, state.deps[i]))) {
        state.deps = deps
        owner.effects.push(() => {
          state.cleanup?.()
          state.cleanup = effect()
        })
      }
    },
    createContext: () => ({ Provider: 'provider' }),
    useContext: () => auth,
  }
  react.useLayoutEffect = react.useEffect
  const api = Object.fromEntries(['get', 'post', 'postEnvelope', 'put', 'patch', 'del'].map((method) => [method, (url, body) => {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    requests.push({ method, url, body, resolve, reject })
    return promise
  }]))

  function load(relative) {
    const filename = path.resolve(__dirname, '../src', relative)
    if (cache.has(filename)) return cache.get(filename)
    const module = { exports: {} }
    cache.set(filename, module.exports)
    const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      fileName: filename,
      compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
    }).outputText
    const localRequire = (name) => {
      if (name === 'react') return react
      if (name === 'react/jsx-runtime') return { jsx, jsxs: jsx, Fragment: 'fragment' }
      if (name === 'qrcode') return { default: { toDataURL: async () => '' } }
      if (name === 'recharts') return new Proxy({}, { get: () => 'div' })
      if (name.endsWith('/auth')) return { useAuth: () => auth }
      if (name.endsWith('/router')) return { Link: 'a', navigate: (url) => navigations.push(url), useRoute: () => '/' }
      const resolved = path.resolve(path.dirname(filename), name)
      const extension = fs.existsSync(`${resolved}.ts`) ? '.ts' : '.tsx'
      const exports = load(path.relative(path.resolve(__dirname, '../src'), resolved + extension))
      return name.endsWith('/api') ? { ...exports, api } : exports
    }
    vm.runInThisContext(`(function(require,module,exports,window,crypto){${source}\n})`, { filename })(localRequire, module, module.exports, { prompt: () => prompts.shift(), confirm: (message) => { confirms.push(message); return confirmAnswers.length ? confirmAnswers.shift() : true }, setInterval: (fn) => { intervals.push(fn); return intervals.length }, clearInterval: () => {} }, crypto)
    return module.exports
  }

  function mount(Component, props = {}) {
    let instance
    let tree
    const unmount = () => {
      if (!instance) return
      instance.active = false
      for (const state of instance.slots) state.cleanup?.()
    }
    const render = (next = props) => {
      props = next
      // Detail page wrappers contain no hooks and supply the entity key.
      let type = Component, key
      if (Component.name === 'RaidDetailPage' || Component.name === 'PlayerDetailPage' || Component.name === 'CharacterDetailPage') {
        const element = Component(props)
        type = element.type
        key = element.key
      }
      if (!instance || instance.key !== key) {
        unmount()
        instance = { key, slots: [], effects: [], active: true }
      }
      current = instance
      instance.index = 0
      tree = type(props)
      current = undefined
      const effects = instance.effects.splice(0)
      effects.forEach((effect) => effect())
      return tree
    }
    render()
    return { render, unmount, get tree() { return tree } }
  }

  return { load, mount, requests, api, auth, navigations, prompts, confirms, confirmAnswers, intervals }
}

function nodes(tree, predicate) {
  if (!tree || typeof tree !== 'object') return []
  if (Array.isArray(tree)) return tree.flatMap((child) => nodes(child, predicate))
  if (typeof tree.type === 'function' && ['RaidRosterBoard', 'RaidLootList', 'SpecSelect'].includes(tree.type.name)) return nodes(tree.type(tree.props), predicate)
  return [...(predicate(tree) ? [tree] : []), ...nodes(tree.props?.children, predicate)]
}
function text(tree) {
  if (tree == null || typeof tree === 'boolean') return ''
  if (Array.isArray(tree)) return tree.map(text).join(' ')
  if (typeof tree !== 'object') return String(tree)
  if (typeof tree.type === 'function' && ['RaidRosterBoard', 'RaidLootList', 'SpecSelect'].includes(tree.type.name)) return text(tree.type(tree.props))
  return text(tree.props?.children)
}
const button = (view, label) => nodes(view.tree, (n) => n.type === 'button' && text(n) === label)[0]
const form = (view, index = 0) => nodes(view.tree, (n) => n.type === 'form')[index]
const input = (view, placeholder) => nodes(view.tree, (n) => n.type === 'input' && n.props.placeholder === placeholder)[0]
const labelled = (view, label) => nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === label)[0]
const select = (view, label) => nodes(view.tree, (n) => (n.type === 'select' || n.type === 'input') && n.props['aria-label'] === label)[0]
const suggestions = (view, label) => nodes(view.tree, (n) => n.type === 'datalist' && n.props.id === select(view, label)?.props.list)[0]
const checkbox = (view, label) => nodes(view.tree, (n) => n.type === 'label' && n.props.className === 'check' && text(n).trim() === label)[0].props.children[0]
const event = { preventDefault() {} }
const change = (value) => ({ target: { value } })
const flush = async (view) => { for (let i = 0; i < 8; i++) await Promise.resolve(); view.render() }
const writes = (s) => s.requests.filter((r) => r.method !== 'get')
const raid = (id) => ({ id, name: `Raid ${id}`, status: 'OPEN' })

test('roster combat groups use stored class/spec and preserve tank and healer roles', () => {
  const { rosterGroup } = setup().load('raidRoster.ts')
  for (const [characterClass, spec, expected] of [
    ['Todesritter', 'Frost', 'MELEE'], ['Krieger', 'Furor', 'MELEE'],
    ['Paladin', 'Vergeltung', 'MELEE'], ['Schurke', 'Kampf', 'MELEE'],
    ['Jäger', 'Überleben', 'RANGED'], ['Magier', 'Frost', 'RANGED'],
    ['Priester', 'Schatten', 'RANGED'], ['Hexer', 'Gebrechen', 'RANGED'],
    ['Druide', 'Wilder Kampf (Cat)', 'MELEE'], ['Druide', 'Gleichgewicht (Eule)', 'RANGED'],
    ['Schamane', 'Verstärkung', 'MELEE'], ['Schamane', 'Elementar', 'RANGED'],
    ['Druide', '', 'UNKNOWN'], ['Schamane', 'Wiederherstellung', 'UNKNOWN'],
    ['', '', 'UNKNOWN'], ['Unbekannt', 'Frost', 'UNKNOWN'],
    [' Druide ', ' Gleichgewicht (Eule) ', 'RANGED'], ['Warlock', '', 'RANGED'],
  ]) {
    assert.equal(rosterGroup({ role: 'DD', class: characterClass, spec }), expected, `${characterClass}: ${spec}`)
    for (const role of ['TANK', 'HEAL']) assert.equal(rosterGroup({ role, class: characterClass, spec }), role)
  }
})

test('roster separates melee, ranged, unknown and bench while preserving inline editing', () => {
  const s = setup()
  const { RaidRosterBoard } = s.load('components/RaidRosterBoard.tsx')
  const attendance = [
    { id: 1, character_name: 'Katze', class: 'Druide', spec: 'Wilder Kampf (Cat)', role: 'DD' },
    { id: 2, character_name: 'Eule', class: 'Druide', spec: 'Gleichgewicht (Eule)', role: 'DD' },
    { id: 3, character_name: 'Altbestand', role: 'DD' },
    { id: 4, character_name: 'Bankplatz', class: 'Schamane', spec: 'Elementar', role: 'DD', is_bench: true },
    { id: 5, character_name: 'Bär', class: 'Druide', spec: 'Wächter (Tank / Bär)', role: 'TANK' },
  ]
  let edited
  const props = { attendance, playerName: () => 'Besitzer', canWrite: false, pending: false, onEdit: (row) => { edited = row } }
  const view = s.mount(RaidRosterBoard, props)
  const section = (name) => nodes(view.tree, (n) => n.type === 'section' && n.props['aria-label'] === name)[0]
  for (const [group, name] of [['Tanks', 'Bär'], ['Nahkampf', 'Katze'], ['Fernkampf', 'Eule'], ['Schaden · unklar', 'Altbestand'], ['Ersatzbank', 'Bankplatz']]) {
    assert.match(text(section(group)), new RegExp(name))
    assert.equal(nodes(section(group), (n) => n.type === 'li').length, 1)
  }
  assert.match(text(section('Ersatzbank')), /Fernkampf/)
  assert.doesNotMatch(text(section('Fernkampf')), /Bankplatz/)
  assert.equal(nodes(view.tree, (n) => n.type === 'button').length, 0)
  view.render({ ...props, canWrite: true })
  nodes(section('Nahkampf'), (n) => n.type === 'button')[0].props.onClick()
  assert.equal(edited, attendance[0])
  view.render({ ...props, canWrite: true, editingID: 1, children: 'EDITOR' })
  assert.match(text(section('Nahkampf')), /Katze\s+bearbeiten.*EDITOR/)
  assert.doesNotMatch(text(section('Fernkampf')), /EDITOR/)
  view.render({ ...props, attendance: [] })
  assert.equal(section('Schaden · unklar'), undefined)
  assert.equal(nodes(view.tree, (n) => n.props.className === 'setup-empty').length, 4)
})

test('every WotLK class has a distinct class icon and unknown classes fall back', () => {
  const { ClassEmblem } = setup().load('components/ClassEmblem.tsx')
  const classes = ['Todesritter', 'Druide', 'Jäger', 'Magier', 'Paladin', 'Priester', 'Schamane', 'Schurke', 'Hexer', 'Krieger']
  const graphics = classes.map((characterClass) => {
    const tree = ClassEmblem({ characterClass })
    assert.equal(tree.type, 'img')
    assert.equal(tree.props['aria-hidden'], 'true')
    assert.match(tree.props.src, /^https:\/\/wow\.zamimg\.com\/images\/wow\/icons\/large\/classicon_[a-z]+\.jpg$/)
    return tree.props.src
  })
  assert.equal(new Set(graphics).size, 10)
  assert.ok(graphics.every(Boolean))
  const fallback = ClassEmblem({ characterClass: '' })
  assert.equal(fallback.type, 'span')
  assert.equal(fallback.props['aria-hidden'], 'true')
  const board = setup().load('components/RaidRosterBoard.tsx').RaidRosterBoard({
    attendance: classes.map((characterClass, id) => ({ id, class: characterClass, role: 'DD' })),
    playerName: () => '', canWrite: false, pending: false, onEdit: () => {},
  })
  assert.deepEqual(nodes(board, (n) => n.type?.name === 'ClassEmblem').map((n) => n.props.characterClass).sort(), [...classes].sort())
})

test('raid signups submit several own characters and guard duplicate submissions', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, entries: [], class_specs: { Magier: ['Arkan', 'Feuer', 'Frost'] } })
  resolveIf(s, '/api/v1/players/1/characters', [1, 2].map((id) => ({ id, player_id: 1, name: `Magier ${id}`, class: 'Magier', main_spec: 'Feuer' })))
  await flush(view)
  for (const node of nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')) node.props.onChange({ target: { checked: true } })
  view.render()
  const selects = nodes(view.tree, (n) => n.type === 'select')
  selects[1].props.onChange(change('Arkan'))
  view.render()
  form(view).props.onSubmit(event)
  form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 1)
  assert.deepEqual(writes(s)[0].body, { choices: [{ character_id: 1, spec: 'Feuer' }, { character_id: 2, spec: 'Arkan' }] })
  assert.equal(button(view, 'Anmeldungen öffnen'), undefined)
  view.unmount()
})

test('members can withdraw their own pending signup but not accepted ones or other players', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }, { id: 3, name: 'Andere' }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: { Magier: ['Feuer'] }, entries: [
    { id: 4, player_id: 2, character_id: 11, character_name: 'Meiner', class: 'Magier', spec: 'Feuer', is_accepted: false, is_withdrawn: false },
    { id: 5, player_id: 3, character_id: 12, character_name: 'Fremder', class: 'Magier', spec: 'Feuer', is_accepted: false, is_withdrawn: false },
    { id: 6, player_id: 2, character_id: 13, character_name: 'Bestaetigt', class: 'Magier', spec: 'Feuer', attendance_id: 44, is_accepted: true, is_withdrawn: false },
  ] })
  resolveIf(s, '/api/v1/players/2/characters', [])
  await flush(view)
  const withdraws = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Zurückziehen')
  assert.equal(withdraws.length, 1, 'only the own, not yet accepted signup can be withdrawn')
  withdraws[0].props.onClick()
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/signups/4/withdraw')
})

test('a signup that closes while a member is choosing explains what happened', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: { Magier: ['Feuer'] }, entries: [] })
  resolveIf(s, '/api/v1/players/2/characters', [{ id: 11, player_id: 2, name: 'Neuling', class: 'Magier', main_spec: 'Feuer', active: true }])
  await flush(view)
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')[0].props.onChange({ target: { checked: true } })
  view.render()
  assert.doesNotMatch(text(view.tree), /gerade geschlossen/)
  const refresh = button(view, 'Anmeldungen aktualisieren').props.onClick()
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: false, is_open: false, class_specs: { Magier: ['Feuer'] }, entries: [] })
  resolveIf(s, '/api/v1/players/2/characters', [])
  await flush(view)
  await refresh
  await flush(view)
  assert.match(text(view.tree), /gerade geschlossen\. Die angekreuzten Charaktere wurden nicht mehr angemeldet/)
  assert.equal(form(view), undefined)
})

test('after the deadline the signup form and withdraw buttons disappear even if the server still says open', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2000-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: { Magier: ['Feuer'] }, entries: [
    { id: 4, player_id: 2, character_id: 11, character_name: 'Meiner', class: 'Magier', spec: 'Feuer', is_accepted: false, is_withdrawn: false },
  ] })
  resolveIf(s, '/api/v1/players/2/characters', [{ id: 11, player_id: 2, name: 'Meiner', class: 'Magier', main_spec: 'Feuer', active: true }])
  await flush(view)
  assert.match(text(view.tree), /Anmeldung geschlossen/)
  assert.equal(form(view), undefined)
  assert.equal(button(view, 'Zurückziehen'), undefined)
})

test('staff can switch the roster slot to an alternative and the unchanged character is sent for safety', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, {
    raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' },
    attendance: [{ id: 44, player_id: 2, character_id: 11 }], players: [{ id: 2, name: 'Spieler' }], onRosterChange: async () => {},
  })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [
    { id: 4, player_id: 2, character_id: 11, character_name: 'Erster', class: 'Magier', spec: 'Feuer', attendance_id: 44, is_accepted: true, is_withdrawn: false },
    { id: 5, player_id: 2, character_id: 12, character_name: 'Alternative', class: 'Magier', spec: 'Frost', attendance_id: 44, is_accepted: false, is_withdrawn: false },
  ] })
  resolveIf(s, '/api/v1/players/1/characters', [])
  await flush(view)
  button(view, 'Auf diesen Charakter wechseln').props.onClick()
  view.render()
  assert.match(text(view.tree), /ersetzen\?/)
  button(view, 'Charakterwechsel bestätigen').props.onClick()
  assert.deepEqual(writes(s)[0].body, { replace_attendance_id: 44, expected_character_id: 11 })
})

test('signup form hints at the missing choice and hides the button without own characters', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: { Magier: ['Feuer', 'Frost'] }, entries: [] })
  resolveIf(s, '/api/v1/players/2/characters', [{ id: 11, player_id: 2, name: 'Neuling', class: 'Magier', main_spec: 'Feuer', active: true }])
  await flush(view)
  assert.match(text(view.tree), /Mindestens einen Charakter ankreuzen/)
  assert.equal(button(view, 'Ausgewählte Charaktere anmelden').props.disabled, true)
  const box = nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')[0]
  box.props.onChange({ target: { checked: true } })
  view.render()
  assert.doesNotMatch(text(view.tree), /Mindestens einen Charakter ankreuzen/)
  assert.equal(button(view, 'Ausgewählte Charaktere anmelden').props.disabled, false)
})

test('signup form without own characters offers no submit button', async () => {
  const s = setup({ id: 3, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [] })
  resolveIf(s, '/api/v1/players/3/characters', [])
  await flush(view)
  assert.match(text(view.tree), /noch keine Charaktere zugeordnet/)
  assert.equal(button(view, 'Ausgewählte Charaktere anmelden'), undefined)
})

test('raid assistants explicitly confirm alternative switches after signup closure', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, {
    raid: { ...raid(8), scheduled_start: '2000-01-01T20:00:00Z' },
    attendance: [{ id: 44, player_id: 2, character_id: 11 }], players: [{ id: 2, name: 'Spieler' }], onRosterChange: async () => {},
  })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: false, class_specs: {}, entries: [
    { id: 4, player_id: 2, character_id: 11, character_name: 'Erster', class: 'Magier', spec: 'Feuer', attendance_id: 44, is_accepted: true, is_withdrawn: false },
    { id: 5, player_id: 2, character_id: 12, character_name: 'Alternative', class: 'Magier', spec: 'Frost', attendance_id: 44, is_accepted: false, is_withdrawn: false },
  ] })
  resolveIf(s, '/api/v1/players/1/characters', [])
  await flush(view)
  assert.equal(form(view), undefined)
  button(view, 'Auf diesen Charakter wechseln').props.onClick()
  view.render()
  assert.equal(writes(s).length, 0)
  button(view, 'Charakterwechsel bestätigen').props.onClick()
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/signups/5/accept')
  assert.deepEqual(writes(s)[0].body, { replace_attendance_id: 44, expected_character_id: 11 })
  view.unmount()
})
const player = (id, role = 'MEMBER') => ({ id, name: `Spieler ${id}`, role, username: `user${id}` })
const character = (id, playerID) => ({ id, player_id: playerID, name: `Char ${id}`, class: 'Priest', main_spec: 'Shadow', second_spec: 'Holy' })
const seriesRow = { id: 3, name: 'Configured raid', raid_leader_id: 2, weekdays: [0, 3], interval_weeks: 2, start_time: '20:30', timezone: 'Europe/Berlin', catalog_instance_id: 'map:631', raid_size: 25 }
const lock = (playerID) => ({ id: 9, player_id: playerID, status: 'ACTIVE' })
const emptyCatalog = () => ({ sources: [], instances: [], bosses: [], items: [], drops: [] })
const catalogFixture = () => ({
  sources: [{ source: { id: 'reviewed-source', revision: 'pinned-revision', url: 'https://example.test/source', license: 'GPL-2.0', attribution: 'Source authors' }, raw_hash: 'source-hash', notices: ['Teilbestand: nur ausdrücklich geprüfte Drops, kein vollständiger Raidkatalog.'] }],
  instances: [{ id: 'map:631', name: 'Eiskronenzitadelle' }, { id: 'map:603', name: 'Ulduar' }],
  bosses: [{ id: 'boss:1', instance_id: 'map:631', name: 'Gespeicherter Boss' }, { id: 'boss:2', instance_id: 'map:631', name: 'Zweiter Boss' }, { id: 'boss:3', instance_id: 'map:603', name: 'Anderer Raidboss' }],
  items: [{ item_id: '12345', name: 'Katalogitem' }],
  drops: [
    { item_id: '12345', name: 'Katalogitem', boss_id: 'boss:1', raid_size: 25, mode: 'NORMAL', conditional: true, source_variants: [{ difficulty: '0', loot_mode: 1 }, { difficulty: 'source:2/ref:8', loot_mode: 8 }] },
    { item_id: '12345', name: 'Katalogitem', boss_id: 'boss:1', raid_size: 25, mode: 'HEROIC', conditional: false, source_variants: [{ difficulty: '0', loot_mode: 4 }] },
    { item_id: '12345', name: 'Katalogitem', boss_id: 'boss:1', raid_size: 10, mode: 'NORMAL', conditional: false, source_variants: [{ difficulty: 'source:base', loot_mode: 1 }] },
    { item_id: '12345', name: 'Katalogitem', boss_id: 'boss:3', raid_size: 10, mode: 'NORMAL', conditional: false, source_variants: [{ difficulty: 'source:ulduar', loot_mode: 1 }] },
  ],
})
const itemDrops = (catalog, bossID, size) => catalog.drops.filter((drop) => drop.boss_id === bossID && (size == null || drop.raid_size === size))
const sourceLoot = (options = {}) => ({ ...options, catalog: catalogFixture(), raid: { ...options.raid, catalog_instance_id: 'map:631', raid_size: 25 } })
async function chooseSourceDrop(s, view) {
  const catalog = catalogFixture()
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  const option = nodes(suggestions(view, 'Loot-Item'), (node) => node.type === 'option' && node.props.value)[0]
  select(view, 'Loot-Item').props.onChange(change(option.props.value))
  view.render()
}

function resolveIf(s, url, value) {
  const request = s.requests.find((r) => r.method === 'get' && r.url === url && !r.resolved)
  if (!request) return false
  request.resolved = true
  request.resolve(value)
  return true
}

function resolveRaid(s, id, options = {}) {
  for (const r of s.requests.filter((r) => r.method === 'get' && !r.resolved)) {
    r.resolved = true
    r.resolve(r.url === `/api/v1/raids/${id}` ? { ...raid(id), ...options.raid }
      : r.url === '/api/v1/players' ? [player(1), player(2)]
      : r.url === '/api/v1/characters' ? options.characters ?? [character(10, 1), character(11, 1), character(20, 2), character(21, 2)]
      : r.url === '/api/v1/character-options' ? { Priester: ['Disziplin', 'Heilig', 'Schatten'], Magier: ['Feuer', 'Frost'] }
      : r.url.endsWith('/attendance') ? options.attendance ?? []
      : r.url.endsWith('/loot') ? options.loot ?? []
      : r.url.endsWith('/signups') ? { absences: options.absences ?? [] }
      : r.url.endsWith('/shadowmourne') ? options.shadowmourne ?? { raid_id: id, editable: true, tasks: [] }
      : r.url === '/api/v1/loot-catalog' ? options.catalog ?? emptyCatalog() : [])
  }
}

async function startArchive(s, view, rows = []) {
  const opening = button(view, 'Archivieren').props.onClick()
  const request = s.requests.find((r) => r.method === 'get' && r.url.endsWith('/archive-preview') && !r.resolved)
  request.resolved = true
  request.resolve(rows)
  await opening
  view.render()
  return { done: button(view, 'Jetzt archivieren').props.onClick() }
}

function resolvePlayer(s, id) {
  for (const r of s.requests.filter((r) => r.method === 'get' && !r.resolved)) {
    r.resolved = true
    if (r.url === '/api/v1/character-options') r.resolve({ Todesritter: ['Frost', 'Unheilig'], Magier: ['Feuer', 'Frost'], Priester: ['Heilig', 'Schatten'] })
    else if (r.url.endsWith('/characters')) r.resolve([character(id * 10, id), character(id * 10 + 1, id)])
    else if (r.url.endsWith('/points')) r.resolve({ balance: 200, transactions: [] })
    else if (r.url.endsWith('/loot')) r.resolve([])
    else if (r.url.endsWith('/locks')) r.resolve([lock(id)])
    else r.resolve(player(id))
  }
}

for (const existingOwner of [false, true]) {
  test(`raid quick creation uses ${existingOwner ? 'existing' : 'new'} owner and safely retries unknown results`, async () => {
    const s = setup(player(1, 'RAIDASSISTENT'))
    const { RaidQuickAdd } = s.load('components/RaidQuickAdd.tsx')
    const { useSubmission } = s.load('useSubmission.ts')
    let added = 0
    const view = s.mount(() => RaidQuickAdd({
      raidID: 5, players: [player(2)], options: { Magier: ['Feuer', 'Frost'], Krieger: ['Waffen'] },
      needsReason: false, onAdded: async () => { added++ }, onCancel() {}, ...useSubmission(),
    }))
    const field = (label, type) => nodes(view.tree, (node) => node.type === 'label' && text(node).startsWith(label))
      .flatMap((node) => nodes(node, (child) => child.type === type))[0]
    field('Rising-Gods-Charakter', 'input').props.onChange(change('Newmage'))
    view.render()
    button(view, 'Suchen').props.onClick()
    resolveIf(s, '/api/v1/external/rising-gods/characters?q=Newmage', [{ profile_id: 'rg-1', name: 'Newmage', class: 'Magier', main_spec: 'Feuer', second_spec: 'Frost', level: 80 }])
    await flush(view)
    nodes(view.tree, (node) => node.type === 'button' && node.props.className === 'wishlist-toggle')[0].props.onClick()
    view.render()
    assert.equal(field('Spielername', 'input').props.value, 'Newmage')
    if (existingOwner) select(view, 'Charakter-Besitzer').props.onChange(change('2'))
    view.render()
    const first = form(view).props.onSubmit(event)
    await form(view).props.onSubmit(event)
    assert.equal(writes(s).length, 1)
    const write = writes(s)[0]
    assert.equal(write.url, '/api/v1/raids/5/quick-add')
    assert.equal(write.body.player_id, existingOwner ? 2 : 0)
    assert.equal(write.body.player_name, existingOwner ? '' : 'Newmage')
    assert.equal(write.body.profile_id, 'rg-1')
    assert.equal('main_spec' in write.body, false)
    assert.equal('second_spec' in write.body, false)
    assert.equal('username' in write.body, false)
    write.reject(new Error('connection lost'))
    await first
    view.render()
    assert.equal(nodes(view.tree, (node) => node.type === 'fieldset')[0].props.disabled, true)
    const retry = form(view).props.onSubmit(event)
    assert.deepEqual(writes(s)[1].body, write.body)
    writes(s)[1].resolve({ id: 42 })
    await retry
    assert.equal(added, 1)
  })
}

test('profile spec dropdowns clear incompatible choices on class change and preserve historical values', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  button(view, 'Bearbeiten').props.onClick()
  view.render()
  assert.equal(select(view, 'Main-Spec').props.value, 'Shadow')
  assert.match(text(select(view, 'Main-Spec')), /Shadow.*gespeichert/)
  nodes(view.tree, (node) => node.type === 'label' && text(node).startsWith('Klasse'))
    .flatMap((node) => nodes(node, (child) => child.type === 'select'))[0].props.onChange(change('Magier'))
  view.render()
  assert.equal(select(view, 'Main-Spec').props.value, '')
  assert.match(text(select(view, 'Main-Spec')), /Feuer/)
  assert.doesNotMatch(text(select(view, 'Main-Spec')), /Unheilig/)
})

test('submission guard blocks same-tick duplicates and releases after failure', async () => {
  const s = setup()
  const { useSubmission } = s.load('useSubmission.ts')
  let controls
  const view = s.mount(() => { controls = useSubmission(); return null })
  let reject, calls = 0
  const first = controls.submit(() => { calls++; return new Promise((_, no) => { reject = no }) })
  await controls.submit(async () => { calls++ })
  assert.equal(calls, 1)
  view.render()
  assert.equal(controls.pending, true)
  reject(new Error('offline'))
  await assert.rejects(first, /offline/)
  view.render()
  assert.equal(controls.pending, false)
  await controls.submit(async () => { calls++ })
  assert.equal(calls, 2)
  view.unmount()
  await controls.submit(async () => { calls++ })
  assert.equal(calls, 2)
})

test('raid switch resets drafts, ignores old requests and disables old handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  input(view, 'Loot-Notizen').props.onChange(change('old loot'))
  view.render()
  const oldSubmit = form(view, 1).props.onSubmit
  view.render({ raidID: 2 })
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 0)
  await oldSubmit(event)
  assert.equal(writes(s).length, 0)
  resolveRaid(s, 2)
  await flush(view)
  assert.match(text(view.tree), /Raid 2/)
  assert.equal(input(view, 'Loot-Notizen').props.value, '')
  // Switch while the new entity is still loading, then settle it late.
  view.render({ raidID: 3 })
  const obsolete = s.requests.filter((r) => !r.resolved)
  view.render({ raidID: 4 })
  obsolete.forEach((r) => { r.resolved = true; r.reject(new Error('old failure')) })
  resolveRaid(s, 4)
  await flush(view)
  assert.match(text(view.tree), /Raid 4/)
  assert.doesNotMatch(text(view.tree), /fehlgeschlagen/)
})

test('raid character selection resolves ownership directly and guards stale submits', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  assert.equal(select(view, 'Teilnahme-Spieler'), undefined)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  const stale = form(view).props.onSubmit
  select(view, 'Teilnahme-Charakter').props.onChange(change('20'))
  await stale(event)
  assert.equal(writes(s).length, 0)
  view.render()
  assert.equal(select(view, 'Teilnahme-Charakter').props.value, '20')
  const submit = form(view).props.onSubmit
  const first = submit(event)
  await submit(event)
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].body.character_id, 20)
  assert.equal(writes(s)[0].body.player_id, 2)
  writes(s)[0].reject(new Error('offline'))
  await first
  view.render()
  select(view, 'Teilnahme-Charakter').props.onChange(change(''))
  view.render()
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  assert.equal(s.requests.some((r) => /players\/\d+\/characters/.test(r.url)), false)
})

test('player EP is read-only while switching players', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  button(view, 'Konto').props.onClick()
  view.render()
  input(view, 'Aktuelles Passwort').props.onChange(change('secret'))
  assert.equal(button(view, 'Enhance würfeln'), undefined)
  assert.equal(writes(s).length, 0)
  view.render({ playerID: 2 })
  resolvePlayer(s, 2)
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Kosten 50/)
  assert.equal(button(view, 'Enhance würfeln'), undefined)
})

test('character search filters the suggestions and clears an unselected identity', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  input(view, 'Charaktername oder Spieler').props.onChange(change('Spieler 2'))
  view.render()
  assert.equal(select(view, 'Teilnahme-Charakter').props.value, 'Spieler 2')
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  const characterSuggestions = nodes(suggestions(view, 'Teilnahme-Charakter'), (node) => node.type === 'option').map((node) => node.props.value).join(' ')
  assert.match(characterSuggestions, /Char 20.*Spieler 2/)
  assert.doesNotMatch(characterSuggestions, /Char 11/)
})

test('selected attendance character can be cleared without retaining its identity', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  const clearButton = () => nodes(view.tree, (node) => node.type === 'button' && node.props['aria-label'] === 'Ausgewählten Charakter entfernen')[0]
  assert.ok(clearButton())
  clearButton().props.onClick()
  view.render()
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  assert.equal(clearButton(), undefined)
  assert.equal(select(view, 'Teilnahme-Charakter').props.value, '')
})

test('attendance save announces successful completion', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  const saving = form(view).props.onSubmit(event)
  writes(s)[0].resolve({ id: 1 })
  await flush(view)
  resolveRaid(s, 1)
  await saving
  view.render()
  assert.match(text(view.tree), /Teilnahme hinzugefügt/)
})

test('roster filters are display-only and can be reset', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [recipientRow, { ...recipientRow, id: 10, character_id: 21, character_name: 'Andere Figur', role: 'TANK' }] })
  await flush(view)
  input(view, 'Charakter oder Spieler').props.onChange(change('Saved'))
  view.render()
  const roster = () => nodes(view.tree, (node) => node.type === 'section' && node.props.id === 'raid-roster')[0]
  assert.match(text(roster()), /Saved raid character/)
  assert.doesNotMatch(text(roster()), /Andere Figur/)
  button(view, 'Filter zurücksetzen').props.onClick()
  view.render()
  assert.match(text(roster()), /Andere Figur/)
})

test('roster filters say there are no matches instead of an unfilled slot', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [recipientRow] })
  await flush(view)
  const roster = () => nodes(view.tree, (node) => node.type === 'section' && node.props.id === 'raid-roster')[0]
  assert.match(text(roster()), /Noch nicht besetzt/)
  assert.doesNotMatch(text(roster()), /Keine Treffer/)
  input(view, 'Charakter oder Spieler').props.onChange(change('gibt es nicht'))
  view.render()
  assert.match(text(roster()), /Keine Treffer auf der Ersatzbank/)
  assert.match(text(roster()), /0\s+von\s+1\s+Teilnehmern angezeigt/)
  assert.doesNotMatch(text(roster()), /Noch nicht besetzt/)
})

test('success message can be closed and does not follow the user to another tab', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  const closeButton = () => nodes(view.tree, (node) => node.type === 'button' && node.props['aria-label'] === 'Meldung schließen')[0]
  const addCharacter = async () => {
    select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
    view.render()
    const saving = form(view).props.onSubmit(event)
    writes(s).at(-1).resolve({ id: 1 })
    await flush(view)
    resolveRaid(s, 1)
    await saving
    view.render()
  }
  await addCharacter()
  assert.match(text(view.tree), /Teilnahme hinzugefügt/)
  closeButton().props.onClick()
  view.render()
  assert.doesNotMatch(text(view.tree), /Teilnahme hinzugefügt/)
  assert.equal(closeButton(), undefined)
  await addCharacter()
  assert.match(text(view.tree), /Teilnahme hinzugefügt/)
  nodes(view.tree, (node) => node.type === 'button' && node.props['aria-controls'] === 'raid-loot')[0].props.onClick()
  view.render()
  view.render() // the harness does not re-render after an effect sets state
  assert.doesNotMatch(text(view.tree), /Teilnahme hinzugefügt/)
})

test('character suggestions hide internal IDs unless two characters look identical', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { characters: [character(10, 1), { ...character(11, 1), name: 'Char 10' }, character(20, 2)].map((row) => ({ ...row, active: true })) })
  await flush(view)
  const values = nodes(suggestions(view, 'Teilnahme-Charakter'), (node) => node.type === 'option').map((node) => node.props.value)
  assert.deepEqual(values, ['Char 10 · Priest · Spieler 1 · #10', 'Char 10 · Priest · Spieler 1 · #11', 'Char 20 · Priest · Spieler 2'])
  select(view, 'Teilnahme-Charakter').props.onChange(change(values[1]))
  view.render()
  assert.equal(button(view, 'Hinzufügen').props.disabled, false)
})

test('an exact item ID selects the item while only the name is shown', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
  await flush(view)
  const catalog = catalogFixture()
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  const drops = itemDrops(catalog, 'boss:1', 25)
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', drops)
  await flush(view)
  const wanted = drops.find((drop) => drop.mode === 'NORMAL')
  select(view, 'Loot-Item').props.onChange(change(` ${wanted.item_id} `))
  view.render()
  assert.match(select(view, 'Loot-Item').props.value, /Katalogitem/)
  assert.doesNotMatch(select(view, 'Loot-Item').props.value, new RegExp(String(wanted.item_id)))
  assert.ok(nodes(view.tree, (node) => node.type === 'button' && node.props['aria-label'] === 'Ausgewähltes Item entfernen')[0])
})

for (const [label, submits] of [['Lock anlegen', true], ['Stornieren', false], ['Anlegen', true], ['Speichern', true]]) {
  test(`player mutation ${label} blocks same-tick duplicates`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    // Locate the form by its submit label; own-profile claim forms shift form positions.
    const owner = () => nodes(view.tree, (n) => n.type === 'form' && nodes(n, (b) => b.type === 'button' && text(b) === label).length > 0)[0]
    if (label === 'Anlegen') {
      input(view, 'Mindestens 3 Zeichen').props.onChange(change('Newmage'))
      view.render()
      button(view, 'Suchen').props.onClick()
      resolveIf(s, '/api/v1/external/rising-gods/characters?q=Newmage', [{ profile_id: 'rg-1', name: 'Newmage', class: 'Magier', main_spec: 'Feuer', second_spec: 'Frost', level: 80 }])
      await flush(view)
      nodes(view.tree, (node) => node.type === 'button' && node.props.className === 'wishlist-toggle')[0].props.onClick()
      view.render()
    }
    const invoke = submits ? () => owner().props.onSubmit(event) : button(view, label).props.onClick
    const first = invoke()
    await invoke()
    assert.equal(writes(s).length, 1)
    view.render()
    assert.equal(button(view, label).props.disabled, true)
    writes(s)[0].reject(new Error('offline'))
    await first
    view.render()
    assert.equal(button(view, label).props.disabled, false)
  })
}

test('loot submission blocks duplicates and stays pending through refresh', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot({ attendance: [attendanceRow] }))
  await flush(view)
  await chooseSourceDrop(s, view)
  select(view, 'Loot-Empfänger').props.onChange(change('11'))
  view.render()
  const invoke = () => form(view, 1).props.onSubmit(event)
  const first = invoke()
  await invoke()
  assert.equal(writes(s).length, 1)
  writes(s)[0].resolve({ id: 9 })
  await flush(view)
  assert.equal(button(view, 'Vergeben').props.disabled, true)
  await invoke()
  assert.equal(writes(s).length, 1)
  resolveRaid(s, 1)
  await first
  view.render()
  assert.equal(button(view, 'Vergeben').props.disabled, true)
  assert.equal(input(view, 'Item'), undefined)
})

test('player switch ignores late successful detail loads and old mutation handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  const oldSubmit = form(view).props.onSubmit
  view.render({ playerID: 2 })
  const oldReads = s.requests.filter((r) => !r.resolved)
  view.render({ playerID: 3 })
  oldReads.forEach((r) => { r.resolved = true; r.resolve(r.url.endsWith('/2') ? player(2) : []) })
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Spieler 2/)
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 0)
  resolvePlayer(s, 3)
  await flush(view)
  await oldSubmit(event)
  assert.equal(writes(s).length, 0)
  assert.match(text(view.tree), /Spieler 3/)
})

test('character profile shows old names from the header dropdown', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/CharacterDetail.tsx').CharacterDetailPage, { characterID: 10 })
  resolveIf(s, '/api/v1/characters/10', { ...character(10, 1), external_source: 'rising-gods', external_profile_id: '10' })
  await flush(view)
  resolveIf(s, '/api/v1/players/1', player(1))
  resolveIf(s, '/api/v1/players/1/loot', [{ id: 1, raid_id: 2, item_name: 'Old staff', boss: 'Boss', awarded_to_character_id: 10, is_main_spec: true, award_method: 'MAIN', awarded_at: '2026-10-01T18:00:00Z' }])
  resolveIf(s, '/api/v1/characters/10/name-history', [{ id: 2, old_name: 'Oldchar', new_name: 'Char 10', created_at: '2026-10-01T18:00:00Z' }])
  await flush(view)
  assert.equal(s.requests.some((request) => request.url === '/api/v1/players/1/characters' || request.url === '/api/v1/players/1/points'), false)
  assert.doesNotMatch(text(view.tree), /Weitere Charaktere|Enhance Points/)
  assert.doesNotMatch(text(view.tree), /Frühere Namen\s+Oldchar/)
  nodes(view.tree, (n) => n.type === 'button' && n.props.className === 'name-history-toggle')[0].props.onClick()
  view.render()
  assert.match(text(view.tree), /Frühere Namen.*Oldchar.*Char 10/)
  assert.match(text(view.tree), /Old staff/)
  assert.deepEqual(nodes(view.tree, (n) => n.type === 'a').map((n) => n.props.to).filter(Boolean), ['/players/1', '/players/1', '/raids/2'])
  button(view, 'Rising-Gods-Profil neu laden').props.onClick()
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].url, '/api/v1/characters/10/reload-profile')
  writes(s)[0].resolve({ ...character(10, 1), main_spec: 'Holy', second_spec: 'Shadow', external_source: 'rising-gods', external_profile_id: '10' })
  await flush(view)
  assert.match(text(view.tree), /Holy\s+\/\s+Shadow/)
  assert.match(text(view.tree), /Rising-Gods-Profil wurde neu geladen/)
})

test('character profile hides old-name control without name history', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/CharacterDetail.tsx').CharacterDetailPage, { characterID: 10 })
  resolveIf(s, '/api/v1/characters/10', character(10, 1))
  await flush(view)
  resolveIf(s, '/api/v1/players/1', player(1))
  resolveIf(s, '/api/v1/players/1/loot', [])
  resolveIf(s, '/api/v1/characters/10/name-history', [])
  await flush(view)
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && n.props.className === 'name-history-toggle').length, 0)
})

for (const [role, own, allowed] of [['GUEST', true, false], ['MEMBER', true, true], ['MEMBER', false, false], ['ADMIN', false, false], ['ADMIN', true, true]]) {
  test(`EP and lock controls: ${role}, own=${own}`, async () => {
    const s = setup({ id: own ? 1 : 2, role })
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    assert.equal(button(view, 'Enhance würfeln'), undefined)
    for (const label of ['Lock anlegen', 'Stornieren']) {
      assert.equal(Boolean(button(view, label)), allowed, label)
    }
  })
}

for (const [file, component, label] of [
  ['Raids', 'RaidsPage', 'Anlegen'],
  ['Players', 'PlayersPage', 'Anlegen'],
  ['Polls', 'PollsPage', 'Anlegen'],
  ['Roster', 'RosterPage', 'Anlegen'],
]) {
  test(`${file} creation is synchronously guarded and shows pending UX`, async () => {
    const s = setup()
    const view = s.mount(s.load(`pages/${file}.tsx`)[component])
    if (file === 'Raids') {
      s.requests.forEach((r) => r.resolve(r.url === '/api/v1/raid-series' ? [seriesRow] : r.url === '/api/v1/players' ? [player(1)] : r.url === '/api/v1/loot-catalog' ? catalogFixture() : []))
      await flush(view)
      select(view, 'Neue Raidserie').props.onChange(change('3'))
      nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'datetime-local')[0].props.onChange(change('2026-10-05T20:00'))
      view.render()
    }
    const submit = form(view).props.onSubmit
    const first = submit(event)
    await submit(event)
    assert.equal(writes(s).length, 1)
    view.render()
    assert.equal(nodes(view.tree, (n) => n.type === 'button' && text(n).startsWith(label))[0].props.disabled, true)
    writes(s)[0].reject(new Error('offline'))
    await first
    view.render()
    assert.equal(button(view, label).props.disabled, false)
  })
}

test('player list sorts names naturally and filters by the search text', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Players.tsx').PlayersPage)
  const row = (id, name) => ({ id, name, guild_rank_id: null, active: true })
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/players' ? [row(1, 'Spieler10'), row(2, 'Spieler2'), row(3, 'admin'), row(4, 'Spieler1')] : []))
  await flush(view)
  const names = () => nodes(view.tree, (n) => n.type === 'td').map((n) => text(n)).filter((t) => t && t !== 'aktiv')
  assert.deepEqual(names(), ['admin', 'Spieler1', 'Spieler2', 'Spieler10'])
  assert.match(text(view.tree), /4\s+von\s+4\s+Spielern/)
  input(view, 'Name').props.onChange(change(' spieler1 '))
  view.render()
  assert.deepEqual(names(), ['Spieler1', 'Spieler10'])
  assert.match(text(view.tree), /2\s+von\s+4\s+Spielern/)
  input(view, 'Name').props.onChange(change('zzz'))
  view.render()
  assert.match(text(view.tree), /Kein Spieler passt zu Suche und Filter/)
})

test('admins see who has an account, through which registration, and can filter by it', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Players.tsx').PlayersPage)
  const row = (id, name) => ({ id, name, guild_rank_id: null, active: true })
  const character = (id, name, isMain) => ({ id, name, class: 'Magier', is_main: isMain, active: true })
  const account = (id, name, origin, extra = {}) => ({ player_id: id, player_name: name, active: true, role: 'MEMBER', username: null, origin, registered_at: null, registered_character: '', request_id: null, characters: [], ...extra })
  assert.ok(s.requests.some((r) => r.url === '/api/v1/player-accounts'), 'admins load the account overview')
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/players'
    ? [row(1, 'Legacy owner'), row(2, 'Still unclaimed'), row(3, 'admin')]
    : [
        account(1, 'Legacy owner', 'REGISTERED', { username: 'importedhero', registered_at: '2026-10-18T08:00:00Z', registered_character: 'ImportedHero', request_id: 7, characters: [character(10, 'ImportedHero', true), character(11, 'Twinkhero', false)] }),
        account(2, 'Still unclaimed', 'NONE', { role: 'GUEST', characters: [character(12, 'Altchar', true)] }),
        account(3, 'admin', 'ADMIN', { role: 'ADMIN', username: 'admin' }),
      ]))
  await flush(view)
  const page = () => text(view.tree)
  assert.match(page(), /2\s+von\s+3\s*Spielern haben ein Konto/)
  assert.match(page(), /@\s*importedhero/)
  assert.match(page(), /registriert am 18\.10\.2026 über ImportedHero/)
  assert.match(page(), /ImportedHero\s+\(Main\)/)
  assert.match(page(), /Kein Konto/)
  assert.match(page(), /Login vom Admin angelegt/)

  const rows = () => nodes(view.tree, (n) => n.type === 'tr').slice(1).map((n) => text(n))
  select(view, 'Konto').props.onChange(change('without'))
  view.render()
  assert.equal(rows().length, 1)
  assert.match(rows()[0], /Still unclaimed/)
  select(view, 'Konto').props.onChange(change('with'))
  view.render()
  assert.equal(rows().length, 2)

  select(view, 'Konto').props.onChange(change('all'))
  input(view, 'Name, Charakter oder Login').props.onChange(change('twink'))
  view.render()
  assert.equal(rows().length, 1)
  assert.match(rows()[0], /Legacy owner/)
  input(view, 'Name, Charakter oder Login').props.onChange(change('importedhero'))
  view.render()
  assert.match(rows()[0], /Legacy owner/)
})

test('members do not request the admin account overview', async () => {
  const s = setup({ id: 5, role: 'MEMBER' })
  const view = s.mount(s.load('pages/Players.tsx').PlayersPage)
  assert.equal(s.requests.some((r) => r.url === '/api/v1/player-accounts'), false)
  s.requests.forEach((r) => r.resolve([{ id: 1, name: 'Somebody', guild_rank_id: null, active: true }]))
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Konto/)
})

test('shadowmourne list shows stages and lets raid staff count shards, reorder, finish and remove', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { ShadowmournePage } = s.load('pages/Shadowmourne.tsx')
  const view = s.mount(ShadowmournePage)
  const entry = (id, name, extra) => ({ id, character_id: id + 9, character_name: name, character_class: 'Krieger', player_name: `Spieler ${id}`, status: 'ACTIVE', stage: 'PREQUEST', priority: id, shards_collected: 0, shard_target: 50, completed_at: null, ...extra })
  resolveIf(s, '/api/v1/shadowmourne', [
    entry(1, 'Erster', { stage: 'SHARDS', shards_collected: 12 }),
    entry(2, 'Zweiter', { stage: 'BLOOD' }),
    entry(3, 'Fertig', { status: 'COMPLETED', shards_collected: 50, completed_at: '2026-10-18T18:00:00Z' }),
  ])
  resolveIf(s, '/api/v1/characters', [
    { id: 10, name: 'Erster', class: 'Krieger', active: true },
    { id: 30, name: 'Neuer Ritter', class: 'Todesritter', active: true },
    { id: 31, name: 'Magierin', class: 'Magier', active: true },
  ])
  await flush(view)
  assert.match(text(view.tree), /Erster.*12\s*\/\s*50.*Zweiter.*Fertig/)
  const options = nodes(select(view, 'Schattengram-Charakter'), (n) => n.type === 'option').map((n) => text(n))
  assert.deepEqual(options, ['Bitte wählen', 'Neuer Ritter  ( Todesritter )'], 'only eligible, unlisted characters can be added')
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Fertig')[0].props.disabled, true)

  const counting = nodes(view.tree, (n) => n.type === 'button' && n.props['aria-label'] === 'Ein Splitter mehr für Erster')[0].props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/shadowmourne/1')
  assert.deepEqual(writes(s)[0].body, { stage: 'SHARDS', shards_collected: 13, shard_target: 50 })
  writes(s)[0].resolve({})
  await counting
  await flush(view)

  const moving = nodes(view.tree, (n) => n.type === 'button' && n.props['aria-label'] === 'Zweiter nach oben')[0].props.onClick()
  assert.equal(writes(s)[1].url, '/api/v1/shadowmourne/2/move')
  assert.deepEqual(writes(s)[1].body, { position: 1 })
  writes(s)[1].resolve([])
  await moving

  const member = setup({ id: 2, role: 'MEMBER' })
  const memberView = member.mount(member.load('pages/Shadowmourne.tsx').ShadowmournePage)
  resolveIf(member, '/api/v1/shadowmourne', [entry(1, 'Erster', { stage: 'SHARDS', shards_collected: 12 })])
  await flush(memberView)
  assert.match(text(memberView.tree), /Erster/)
  assert.equal(button(memberView, 'Bearbeiten'), undefined)
  assert.equal(button(memberView, 'Eintragen'), undefined)
  assert.equal(member.requests.some((r) => r.url === '/api/v1/characters'), false)
})

test('raid shadowmourne card offers list entries on the step and unlisted roster characters', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { ShadowmourneRaidCard } = s.load('components/ShadowmourneRaidCard.tsx')
  const view = s.mount(ShadowmourneRaidCard, { raidID: 7, rosterKey: '10,11,12' })
  const roster = (id, name, stage) => ({ character_id: id, character_name: name, character_class: 'Paladin', player_id: id, player_name: `Spieler ${id}`,
    entry: stage ? { id: id + 100, character_id: id, character_name: name, stage, priority: id, shards_collected: 5, shard_target: 50 } : null })
  const blood = roster(10, 'Blutsauger', 'BLOOD')
  const other = roster(11, 'Zweitblut', 'BLOOD')
  const unlisted = roster(12, 'Neuling', null)
  const plan = (assigned) => ({ raid_id: 7, editable: true, tasks: [
    { task: 'UNHOLY', assigned: null, suggested: null, options: [unlisted] },
    { task: 'BLOOD', assigned, suggested: blood, options: [blood, other, unlisted] },
    { task: 'FROST', assigned: null, suggested: null, options: [unlisted] },
    { task: 'SHARDS', assigned: null, suggested: null, options: [unlisted] },
  ] })
  resolveIf(s, '/api/v1/raids/7/shadowmourne', plan(null))
  await flush(view)
  assert.match(text(view.tree), /Blut.*Blutsauger.*Vorschlag aus der Liste/)
  const groups = nodes(select(view, 'Einteilung Blut'), (n) => n.type === 'optgroup').map((n) => [n.props.label, nodes(n, (o) => o.type === 'option').length])
  assert.deepEqual(groups, [['Liste: bei Blut', 2], ['Nicht auf der Liste', 1]])
  const assigning = select(view, 'Einteilung Seuche').props.onChange(change('12'))
  assert.equal(writes(s)[0].method, 'put')
  assert.equal(writes(s)[0].url, '/api/v1/raids/7/shadowmourne/UNHOLY')
  assert.deepEqual(writes(s)[0].body, { character_id: 12 })
  const updated = plan(null)
  updated.tasks[0].assigned = unlisted
  writes(s)[0].resolve(updated)
  await assigning
  view.render()
  assert.match(text(view.tree), /Seuche.*Neuling.*Von der Raidleitung eingeteilt/)

  const member = setup({ id: 2, role: 'MEMBER' })
  const memberView = member.mount(member.load('components/ShadowmourneRaidCard.tsx').ShadowmourneRaidCard, { raidID: 7, rosterKey: '' })
  resolveIf(member, '/api/v1/raids/7/shadowmourne', { raid_id: 7, editable: true, tasks: [{ task: 'BLOOD', assigned: null, suggested: null, options: [unlisted] }] })
  await flush(memberView)
  assert.equal(memberView.tree, null, 'members only see the card once someone is suggested or chosen')
})

test('auth distinguishes network failure from anonymous and supports retry', async () => {
  const s = setup()
  const view = s.mount(s.load('auth.tsx').AuthProvider, { children: null })
  s.requests[0].reject(new Error('offline'))
  await flush(view)
  assert.equal(view.tree.props.value.loading, false)
  assert.match(view.tree.props.value.error, /Sitzung konnte nicht geladen/)
  const retry = view.tree.props.value.refresh()
  s.requests.at(-1).resolve(player(1))
  await retry
  view.render()
  assert.equal(view.tree.props.value.error, '')
  assert.equal(view.tree.props.value.player.id, 1)
  const refresh = view.tree.props.value.refresh()
  const { ApiError } = s.load('api.ts')
  s.requests.at(-1).reject(new ApiError(401, 'unauthorized', 'expired'))
  await refresh
  view.render()
  assert.equal(view.tree.props.value.player, null)
  assert.equal(view.tree.props.value.error, '')
})

test('logout clears expired session but preserves identity on network failure', async () => {
  const s = setup()
  const view = s.mount(s.load('auth.tsx').AuthProvider, { children: null })
  s.requests[0].resolve(player(1))
  await flush(view)
  const fail = view.tree.props.value.logout()
  s.requests.at(-1).reject(new Error('offline'))
  await assert.rejects(fail, /offline/)
  view.render()
  assert.equal(view.tree.props.value.player.id, 1)
  const expired = view.tree.props.value.logout()
  const { ApiError } = s.load('api.ts')
  s.requests.at(-1).reject(new ApiError(401, 'unauthorized', 'expired'))
  await expired
  view.render()
  assert.equal(view.tree.props.value.player, null)
})

test('obsolete auth response cannot restore a logged-out identity', async () => {
  const s = setup()
  const view = s.mount(s.load('auth.tsx').AuthProvider, { children: null })
  const initial = s.requests[0]
  const logout = view.tree.props.value.logout()
  s.requests.at(-1).resolve(undefined)
  await logout
  initial.resolve(player(1))
  await flush(view)
  assert.equal(view.tree.props.value.player, null)
  assert.equal(view.tree.props.value.loading, false)
})

test('login has a synchronous guard and German network failure feedback', async () => {
  const s = setup()
  s.auth.login = (username, password) => s.api.post('/login', { username, password })
  const view = s.mount(s.load('pages/Login.tsx').LoginPage)
  const submit = form(view).props.onSubmit
  const first = submit(event)
  await submit(event)
  assert.equal(writes(s).length, 1)
  view.render()
  assert.equal(button(view, 'Anmelden…').props.disabled, true)
  writes(s)[0].reject(new Error('offline'))
  await first
  view.render()
  assert.match(text(view.tree), /Anmelden fehlgeschlagen/)
  // The mode tabs also contain an 'Anmelden' button; check the submit button.
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && n.props.type === 'submit' && text(n) === 'Anmelden')[0].props.disabled, false)
})

test('registration password field states the 1024-byte limit in German (REQ-AUTH-023)', () => {
  const s = setup()
  const view = s.mount(s.load('pages/Login.tsx').LoginPage)
  button(view, 'Registrieren').props.onClick()
  view.render()
  assert.match(text(view.tree), /höchstens 1024 Bytes/)
})

test('login goes back to the password step after too many wrong MFA codes', async () => {
  const s = setup()
  const { ApiError } = s.load('api.ts')
  s.auth.login = async () => ({ mfaRequired: true, challenge: 'challenge-1', factors: { totp_enabled: true, recovery_codes: false, webauthn: false } })
  let verifyError = new ApiError(401, 'invalid_mfa_code', 'invalid MFA code')
  s.auth.verifyMFA = async () => { throw verifyError }
  const view = s.mount(s.load('pages/Login.tsx').LoginPage)
  await form(view).props.onSubmit(event)
  await flush(view)
  const mfaInput = () => nodes(view.tree, (n) => n.type === 'input' && n.props.autoComplete === 'one-time-code')[0]
  mfaInput().props.onChange(change('123456'))
  view.render()
  await form(view).props.onSubmit(event)
  await flush(view)
  assert.match(text(view.tree), /Der Code stimmt nicht/)
  assert.ok(mfaInput())

  verifyError = new ApiError(401, 'mfa_challenge_invalid', 'MFA challenge is no longer valid; log in again')
  await form(view).props.onSubmit(event)
  await flush(view)
  assert.match(text(view.tree), /Bitte melde dich erneut mit deinem Passwort an/)
  assert.equal(mfaInput(), undefined)
})

test('auth gate shows retry when session lookup fails and dashboard for anonymous visitors', () => {
  const s = setup(null)
  s.auth.error = 'Sitzung konnte nicht geladen werden.'
  s.auth.loading = false
  const app = s.load('App.tsx').default()
  const gate = s.mount(app.props.children.type)
  assert.ok(button(gate, 'Erneut versuchen'))
  assert.match(text(gate.tree), /Sitzung konnte nicht geladen/)
  s.auth.error = ''
  gate.render()
  assert.equal(gate.tree.type.name, 'Layout')
})

test('logout UI guards duplicates and handles network rejection without losing identity', async () => {
  const s = setup()
  s.auth.logout = () => s.api.post('/logout')
  const app = s.load('App.tsx').default()
  const gate = s.mount(app.props.children.type)
  const layout = s.mount(gate.tree.type)
  const signOut = button(layout, 'Abmelden').props.onClick
  signOut()
  signOut()
  assert.equal(writes(s).length, 1)
  layout.render()
  assert.equal(button(layout, 'Abmelden…').props.disabled, true)
  writes(s)[0].reject(new Error('offline'))
  await flush(layout)
  assert.match(text(layout.tree), /Abmelden fehlgeschlagen/)
  assert.equal(button(layout, 'Abmelden').props.disabled, false)
  assert.equal(s.auth.player.id, 1)
})

const attendanceRow = {
  id: 7, raid_id: 1, player_id: 1, character_id: 11, role: 'HEAL',
  is_substitute: true, is_bench: false, late_join: true, early_leave: false, notes: 'old attendance note',
}
const lootRow = {
  id: 8, raid_id: 1, item_name: 'Old item', item_id: '123', boss: 'Old boss',
  awarded_to_player_id: 1, awarded_to_character_id: 11, is_main_spec: true, notes: 'old loot note',
}
const recipientRow = { ...attendanceRow, id: 9, player_id: 2, character_id: 20, character_name: 'Saved raid character', class: 'Magier', spec: 'Fire', role: 'DD' }
const completed = { raid: { status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' }, attendance: [attendanceRow, recipientRow], loot: [lootRow] }

test('archived attendance additions require trimmed reason and send all form fields', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('20'))
  view.render()
  await flush(view)
  input(view, 'Teilnahme-Begründung').props.onChange(change('   '))
  view.render()
  await form(view).props.onSubmit(event)
  view.render()
  assert.equal(writes(s).length, 0)
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  assert.match(text(view.tree), /nichtleere Begründung/)
  input(view, 'Teilnahme-Begründung').props.onChange(change('  missed participant  '))
  checkbox(view, 'Ersatzbank').props.onChange({ target: { checked: true } })
  view.render()
  const submit = form(view).props.onSubmit
  const first = submit(event)
  await submit(event)
  const write = writes(s)[0]
  assert.equal(writes(s).length, 1)
  assert.equal(write.method, 'post')
  assert.equal(write.url, '/api/v1/raids/1/attendance')
  assert.equal(write.body.reason, 'missed participant')
  assert.equal(write.body.character_id, 20)
  assert.equal(write.body.is_bench, true)
  assert.equal('notes' in write.body, false)
  assert.equal('is_substitute' in write.body, false)
  write.reject(new Error('offline'))
  await first
})

test('archived attendance edits in place, prefills saved values and PATCHes with an explicit reason', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  button(view, 'Teilnahme korrigieren').props.onClick()
  view.render()
  assert.ok(nodes(view.tree, (n) => n.type === 'li' && /is-editing/.test(n.props.className ?? '') && nodes(n, (child) => child.type === 'form').length === 1).length)
  assert.equal(select(view, 'Teilnahme-Rolle').props.value, 'HEAL')
  assert.equal(checkbox(view, 'Ersatzbank').props.checked, false)
  assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
  await flush(view)
  assert.match(select(view, 'Teilnahme-Charakter').props.value, /Char 11/)
  assert.doesNotMatch(select(view, 'Teilnahme-Charakter').props.value, /#11/)
  await form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  select(view, 'Teilnahme-Rolle').props.onChange(change('TANK'))
  input(view, 'Teilnahme-Begründung').props.onChange(change(' role correction '))
  view.render()
  const first = form(view).props.onSubmit(event)
  const write = writes(s)[0]
  assert.equal(write.method, 'patch')
  assert.equal(write.url, '/api/v1/raids/1/attendance/7')
  assert.equal(write.body.role, 'TANK')
  assert.equal(write.body.character_id, 11)
  assert.equal('is_substitute' in write.body, false)
  assert.equal(write.body.is_bench, false)
  assert.equal('notes' in write.body, false)
  assert.equal(write.body.late_join, true)
  assert.equal(write.body.reason, 'role correction')
  assert.equal('raid_id' in write.body, false)
  write.resolve({ ...attendanceRow, role: 'TANK' })
  await flush(view)
  resolveRaid(s, 1, completed)
  await first
  view.render()
  assert.equal(button(view, 'Teilnahme-Korrektur speichern'), undefined)
  assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
})

test('loot corrections replace identity only from source and preserve audited recipient and method changes', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot(completed))
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  assert.ok(nodes(view.tree, (n) => n.type === 'tr' && nodes(n, (child) => child.type === 'form').length === 1).length)
  assert.equal(input(view, 'Item'), undefined)
  const originalRecipient = select(view, 'Loot-Empfänger').props.value
  assert.match(originalRecipient, /Spieler 1/)
  assert.equal(input(view, 'Loot-Notizen').props.value, 'old loot note')
  await form(view, 1).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  await chooseSourceDrop(s, view)
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  select(view, 'Loot-Vergabemethode').props.onChange(change('SECOND'))
  input(view, 'Loot-Notizen').props.onChange(change('new loot note'))
  input(view, 'Loot-Begründung').props.onChange(change(' wrong recipient '))
  view.render()
  assert.notEqual(select(view, 'Loot-Empfänger').props.value, originalRecipient)
  const submit = form(view, 1).props.onSubmit
  const first = submit(event)
  await submit(event)
  const write = writes(s)[0]
  assert.equal(writes(s).length, 1)
  assert.equal(write.method, 'patch')
  assert.equal(write.url, '/api/v1/raids/1/loot/8')
  assert.equal('item_name' in write.body, false)
  assert.equal(write.body.item_id, '12345')
  assert.equal(write.body.catalog_boss_id, 'boss:1')
  assert.equal('boss' in write.body, false)
  assert.equal(write.body.awarded_to_player_id, 2)
  assert.equal(write.body.awarded_to_character_id, 20)
  assert.equal(write.body.award_method, 'SECOND')
  assert.equal('is_main_spec' in write.body, false)
  assert.equal(write.body.notes, 'new loot note')
  assert.equal(write.body.reason, 'wrong recipient')
  assert.equal('item_lock_id' in write.body, false)
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && /löschen|delete/i.test(text(n))).length, 0)
  write.reject(new Error('offline'))
  await first
})

test('archived loot addition requires reason and sends roster character ID', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot(completed))
  await flush(view)
  await chooseSourceDrop(s, view)
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  input(view, 'Loot-Begründung').props.onChange(change('   '))
  view.render()
  await form(view, 1).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  input(view, 'Loot-Begründung').props.onChange(change(' missed award '))
  view.render()
  const first = form(view, 1).props.onSubmit(event)
  const write = writes(s)[0]
  assert.equal(write.method, 'post')
  assert.equal(write.url, '/api/v1/raids/1/loot')
  assert.equal(write.body.reason, 'missed award')
  assert.equal(write.body.awarded_to_character_id, 20)
  write.reject(new Error('offline'))
  await first
})

test('open raids withdraw loot without a reason while archived raids require one', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [attendanceRow, recipientRow], loot: [lootRow] })
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  assert.equal(input(view, 'Loot-Begründung'), undefined, 'open raids show no reason field')
  assert.equal(button(view, 'Loot zurückziehen').props.disabled, false)
  button(view, 'Loot zurückziehen').props.onClick()
  assert.equal(writes(s)[0].method, 'post')
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/loot/8/withdraw')
  assert.deepEqual(writes(s)[0].body, {})
  writes(s)[0].reject(new Error('offline'))
  await flush(view)
})

test('archived loot withdrawal needs a trimmed reason before it is sent', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  assert.equal(button(view, 'Loot zurückziehen').props.disabled, true)
  input(view, 'Loot-Begründung').props.onChange(change(' 	 '))
  view.render()
  assert.equal(button(view, 'Loot zurückziehen').props.disabled, true)
  button(view, 'Loot zurückziehen').props.onClick()
  assert.equal(writes(s).length, 0)
  input(view, 'Loot-Begründung').props.onChange(change('  wrong recipient  '))
  view.render()
  assert.equal(button(view, 'Loot zurückziehen').props.disabled, false)
  button(view, 'Loot zurückziehen').props.onClick()
  assert.deepEqual(writes(s)[0].body, { reason: 'wrong recipient' })
  writes(s)[0].reject(new Error('offline'))
  await flush(view)
})

test('switching correction records resets reason and invalidates old handlers synchronously', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { ...completed, loot: [lootRow, { ...lootRow, id: 9, item_name: 'Other item' }] })
  await flush(view)
  const buttons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Loot korrigieren')
  buttons[0].props.onClick()
  view.render()
  input(view, 'Loot-Begründung').props.onChange(change('old record reason'))
  view.render()
  const oldSubmit = form(view, 1).props.onSubmit
  buttons[1].props.onClick()
  await oldSubmit(event)
  assert.equal(writes(s).length, 0)
  view.render()
  assert.match(text(form(view, 1)), /Other item/)
  assert.equal(input(view, 'Loot-Begründung').props.value, '')
  button(view, 'Bearbeitung abbrechen').props.onClick()
  view.render()
  assert.equal(input(view, 'Item'), undefined)
  assert.equal(button(view, 'Loot-Korrektur speichern'), undefined)
})

test('pending correction cannot switch to another record before the pending render', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { ...completed, loot: [lootRow, { ...lootRow, id: 9, item_name: 'Other item' }] })
  await flush(view)
  const buttons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Loot korrigieren')
  buttons[0].props.onClick()
  view.render()
  input(view, 'Loot-Begründung').props.onChange(change('reason'))
  view.render()
  const pending = form(view, 1).props.onSubmit(event)
  buttons[1].props.onClick()
  view.render()
  assert.match(text(form(view, 1)), /Old item/)
  assert.equal(input(view, 'Loot-Begründung').props.value, 'reason')
  writes(s)[0].reject(new Error('offline'))
  await pending
})

test('attendance correction row switch invalidates old submits', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { ...completed, attendance: [attendanceRow, { ...attendanceRow, id: 9, player_id: 2, character_id: 20 }] })
  await flush(view)
  const buttons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Teilnahme korrigieren')
  buttons[0].props.onClick()
  view.render()
  input(view, 'Teilnahme-Begründung').props.onChange(change('old reason'))
  view.render()
  const oldSubmit = form(view).props.onSubmit
  buttons[1].props.onClick()
  await oldSubmit(event)
  await flush(view)
  assert.equal(writes(s).length, 0)
  assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
  assert.equal(button(view, 'Teilnahme-Korrektur speichern').props.disabled, true)
  await flush(view)
  input(view, 'Teilnahme-Begründung').props.onChange(change('new reason'))
  view.render()
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/attendance/9')
  assert.equal(writes(s)[0].body.character_id, 20)
  writes(s)[0].reject(new Error('offline'))
  await first
})

for (const kind of ['attendance', 'loot']) {
  test(`${kind} correction entity switch ignores late writes and old submit handlers`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, completed)
    await flush(view)
    button(view, kind === 'attendance' ? 'Teilnahme korrigieren' : 'Loot korrigieren').props.onClick()
    view.render()
    if (kind === 'attendance') {
      await flush(view)
    }
    input(view, kind === 'attendance' ? 'Teilnahme-Begründung' : 'Loot-Begründung').props.onChange(change('reason for old raid'))
    view.render()
    const oldSubmit = form(view, kind === 'attendance' ? 0 : 1).props.onSubmit
    const pending = oldSubmit(event)
    assert.equal(writes(s)[0].method, 'patch')
    view.render({ raidID: 2 })
    resolveRaid(s, 2, { raid: { status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' } })
    await flush(view)
    const count = s.requests.length
    writes(s)[0].resolve({ id: 7 })
    await pending
    await oldSubmit(event)
    view.render()
    assert.equal(writes(s).length, 1)
    assert.equal(s.requests.length, count)
    assert.match(text(view.tree), /Raid 2/)
    assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
    assert.equal(input(view, 'Loot-Begründung').props.value, '')
    assert.doesNotMatch(text(view.tree), /Vorher:/)
  })
}

for (const [status, archived, active, corrections] of [
  ['PLANNED', false, true, true], ['OPEN', false, true, true], ['COMPLETED', false, false, true],
  ['CANCELLED', false, false, false], ['OPEN', true, false, false], ['COMPLETED', true, false, true],
]) {
  test(`raid lifecycle controls: ${status}, archived=${archived}`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, { ...completed, raid: { status, archived_at: archived ? '2026-10-04T00:00:00Z' : null } })
    await flush(view)
    assert.equal(Boolean(button(view, 'Abschließen')), false)
    assert.equal(Boolean(button(view, 'Absagen')), active)
    assert.equal(Boolean(button(view, 'Archivieren')), !archived && status !== 'CANCELLED')
    assert.equal(Boolean(button(view, 'Teilnahme korrigieren')), corrections)
    assert.equal(Boolean(button(view, 'Loot korrigieren')), corrections)
    assert.equal(nodes(view.tree, (n) => n.type === 'form').length, active || corrections ? 2 : 0)
  })
}

test('members cannot access completed raid correction controls', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  assert.equal(button(view, 'Loot korrigieren'), undefined)
  assert.equal(button(view, 'Teilnahme korrigieren'), undefined)
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 0)
})

test('correction reason_required remains visible with draft preserved', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  input(view, 'Loot-Begründung').props.onChange(change('my reason'))
  view.render()
  const first = form(view, 1).props.onSubmit(event)
  const { ApiError } = s.load('api.ts')
  writes(s)[0].reject(new ApiError(400, 'reason_required', 'backend message'))
  await first
  view.render()
  assert.match(text(view.tree), /nichtleere Begründung/)
  assert.match(text(form(view, 1)), /Old item/)
  assert.equal(input(view, 'Loot-Begründung').props.value, 'my reason')
})

for (const status of ['PLANNED', 'OPEN', 'COMPLETED']) {
  for (const kind of ['attendance', 'loot']) {
    test(`${status} unarchived ${kind} edit needs no reason and guards pending PATCH`, async () => {
      const s = setup()
      const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
      resolveRaid(s, 1, { ...completed, raid: { status } })
      await flush(view)
      button(view, kind === 'attendance' ? 'Teilnahme korrigieren' : 'Loot korrigieren').props.onClick()
      view.render()
      if (kind === 'attendance') {
        await flush(view)
      }
      const index = kind === 'attendance' ? 0 : 1
      const reasonField = kind === 'attendance' ? 'Teilnahme-Begründung' : 'Loot-Begründung'
      const saveLabel = kind === 'attendance' ? 'Teilnahme-Korrektur speichern' : 'Loot-Korrektur speichern'
      assert.equal(input(view, reasonField), undefined)
      // Editing one form must not turn the other active-raid addition into a correction.
      assert.equal(input(view, kind === 'attendance' ? 'Loot-Begründung' : 'Teilnahme-Begründung'), undefined)
      if (kind === 'attendance') {
        select(view, 'Teilnahme-Rolle').props.onChange(change('TANK'))
      } else {
        select(view, 'Loot-Vergabemethode').props.onChange(change('SECOND'))
      }
      view.render()
      const submit = form(view, index).props.onSubmit
      const first = submit(event)
      await submit(event)
      const write = writes(s)[0]
      assert.equal(writes(s).length, 1)
      assert.equal(write.method, 'patch')
      assert.equal(write.url, `/api/v1/raids/1/${kind}/${kind === 'attendance' ? 7 : 8}`)
      assert.equal('reason' in write.body, false)
      if (kind === 'attendance') assert.equal(write.body.role, 'TANK')
      else {
        assert.equal('item_name' in write.body, false)
        assert.equal(write.body.award_method, 'SECOND')
      }
      view.render()
      assert.equal(button(view, saveLabel).props.disabled, true)
      write.reject(new Error('offline'))
      await first
      view.render()
      assert.equal(button(view, saveLabel).props.disabled, false)
      assert.equal(input(view, reasonField), undefined)
      button(view, 'Bearbeitung abbrechen').props.onClick()
      view.render()
      assert.equal(input(view, reasonField), undefined)
    })
  }

  test(`${status} additions do not require or send correction reasons`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, sourceLoot({ raid: { status }, attendance: [attendanceRow] }))
    await flush(view)
    assert.equal(input(view, 'Teilnahme-Begründung'), undefined)
    assert.equal(input(view, 'Loot-Begründung'), undefined)
    select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
    view.render()
    await flush(view)
    const attendance = form(view).props.onSubmit(event)
    assert.equal(writes(s)[0].method, 'post')
    assert.equal('reason' in writes(s)[0].body, false)
    writes(s)[0].reject(new Error('offline'))
    await attendance
    view.render()
    await chooseSourceDrop(s, view)
    select(view, 'Loot-Empfänger').props.onChange(change('11'))
    view.render()
    const loot = form(view, 1).props.onSubmit(event)
    assert.equal(writes(s)[1].method, 'post')
    assert.equal('reason' in writes(s)[1].body, false)
    writes(s)[1].reject(new Error('offline'))
    await loot
  })
}

for (const name of ['', ' \t ']) {
  test(`in-place character editor rejects blank name ${JSON.stringify(name)} and cancels without request`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    button(view, 'Bearbeiten').props.onClick()
    view.render()
    nodes(view.tree, (n) => n.props['aria-label'] === 'Charakter bearbeiten: Name')[0].props.onChange(change(name))
    view.render()
    const editor = nodes(view.tree, (n) => n.type === 'form' && n.props.className.includes('in-place-editor'))[0]
    await editor.props.onSubmit(event)
    assert.equal(writes(s).length, 0)
    button(view, 'Bearbeitung abbrechen').props.onClick()
    view.render()
    assert.equal(button(view, 'Charakter speichern'), undefined)
  })
}

test('in-place character editor saves without reason, guards duplicates and preserves failed draft', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  button(view, 'Bearbeiten').props.onClick()
  view.render()
  nodes(view.tree, (n) => n.props['aria-label'] === 'Charakter bearbeiten: Name')[0].props.onChange(change(' New name '))
  view.render()
  const editor = nodes(view.tree, (n) => n.type === 'form' && n.props.className.includes('in-place-editor'))[0]
  assert.ok(nodes(view.tree, (n) => n.type === 'tr' && nodes(n, (child) => child === editor).length).length)
  const first = editor.props.onSubmit(event)
  await editor.props.onSubmit(event)
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].method, 'patch')
  assert.equal(writes(s)[0].body.name, 'New name')
  assert.equal('reason' in writes(s)[0].body, false)
  view.render()
  assert.equal(button(view, 'Charakter speichern').props.disabled, true)
  writes(s)[0].reject(new Error('offline'))
  await first
  view.render()
  assert.equal(nodes(view.tree, (n) => n.props['aria-label'] === 'Charakter bearbeiten: Name')[0].props.value, ' New name ')
})

test('archive preserves successor, invalidates old submits and requires reasons for later changes', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' }, attendance: [recipientRow] })
  await flush(view)
  input(view, 'Loot-Notizen').props.onChange(change('Unrecorded draft'))
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  view.render()
  const oldLootSubmit = form(view, 1).props.onSubmit
  assert.equal(button(view, 'Abschließen'), undefined)
  const first = (await startArchive(s, view)).done
  await button(view, 'Jetzt archivieren').props.onClick()
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].method, 'postEnvelope')
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/archive')
  writes(s)[0].resolve({ data: { ...raid(1), status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' }, next_raid: { ...raid(2), scheduled_start: '2026-10-07T18:30:00Z' } })
  await flush(view)
  resolveRaid(s, 1, { raid: { status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' } })
  await first
  view.render()
  await oldLootSubmit(event)
  assert.equal(writes(s).length, 1)
  assert.match(text(view.tree), /Archiviert/)
  assert.equal(nodes(view.tree, (n) => n.type === 'a' && n.props.to === '/raids/2').length, 1)
  assert.equal(button(view, 'Archivieren'), undefined)
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 2)
  assert.equal(input(view, 'Teilnahme-Begründung').props.required, true)
  assert.equal(input(view, 'Loot-Begründung').props.required, true)
})

for (const missing of ['series_id', 'scheduled_start']) {
  test(`active archive requires ${missing}`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z', [missing]: null } })
    await flush(view)
    assert.equal(button(view, 'Archivieren').props.disabled, true)
    await button(view, 'Archivieren').props.onClick()
    assert.equal(writes(s).length, 0)
    if (missing === 'scheduled_start') assert.match(text(view.tree), /Start eingetragen und gespeichert/)
  })
}

test('completed historical archive has no successor and does not require a series', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { ...completed, raid: { status: 'COMPLETED', archived_at: null } })
  await flush(view)
  assert.match(text(view.tree), /Keine erneute EP-Vergabe/)
  const first = button(view, 'Archivieren').props.onClick()
  writes(s)[0].resolve({ data: { ...raid(1), status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' }, next_raid: null })
  await flush(view)
  resolveRaid(s, 1, { raid: { status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' } })
  await first
  view.render()
  assert.match(text(view.tree), /Kein Nachfolger erstellt/)
})

test('series assignment PATCHes existing active raid and archive errors are localized', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { scheduled_start: '2026-10-04T18:30:00Z' } })
  await flush(view)
  select(view, 'Raidserie zuordnen').props.onChange(change('3'))
  view.render()
  const attach = button(view, 'Planung speichern').props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/raids/1')
  assert.equal(writes(s)[0].body.series_id, 3)
  assert.equal('scheduled_start' in writes(s)[0].body, false)
  writes(s)[0].resolve({ ...raid(1), series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' })
  await attach
  view.render()
  const archive = (await startArchive(s, view)).done
  const { ApiError } = s.load('api.ts')
  writes(s)[1].reject(new ApiError(409, 'local_time_ambiguous', 'English backend message'))
  await archive
  view.render()
  assert.match(text(view.tree), /Zeitumstellung mehrdeutig/)
  assert.equal(button(view, 'Jetzt archivieren').props.disabled, false)
})

test('archived attendance uses saved name, class and actual spec; missing history stays unavailable', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { archived_at: '2026-10-04T19:00:00Z' }, attendance: [
    { ...attendanceRow, character_name: 'Saved old name', class: 'Priester', spec: 'Holy' },
    { ...attendanceRow, id: 8 },
  ] })
  await flush(view)
  assert.match(text(view.tree), /Saved old name/)
  assert.match(text(view.tree), /Priester/)
  assert.match(text(view.tree), /Holy/)
  assert.match(text(view.tree), /Name nicht gespeichert/)
  assert.doesNotMatch(text(view.tree), /#11/)
  assert.equal(s.requests.some((r) => /players\/\d+\/characters/.test(r.url)), false)
})

test('attendance defaults snapshot from selected character, allows actual spec and rejects explicit blanks', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { characters: [character(10, 1), { ...character(11, 1), main_spec: 'Discipline' }] })
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  await flush(view)
  assert.equal(input(view, 'Gespeicherter Charaktername'), undefined)
  assert.equal(select(view, 'Gespeicherte Klasse').props.disabled, true)
  assert.equal(select(view, 'Tatsächlicher Spec').props.disabled, false)
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Shadow')
  button(view, 'Second-Spec übernehmen').props.onClick()
  view.render()
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Holy')
  select(view, 'Teilnahme-Charakter').props.onChange(change('11'))
  view.render()
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Discipline')
  select(view, 'Tatsächlicher Spec').props.onChange(change(''))
  view.render()
  await form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  select(view, 'Tatsächlicher Spec').props.onChange(change('Holy'))
  view.render()
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].body.spec, 'Holy')
  assert.equal(writes(s)[0].body.character_name, 'Char 11')
  assert.equal(writes(s)[0].body.class, 'Priest')
  writes(s)[0].reject(new Error('offline'))
  await first
})

test('attendance correction keeps the saved class locked and changes only the spec', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [{ ...attendanceRow, character_name: 'Old name', class: 'Priester', spec: 'Old spec' }] })
  await flush(view)
  button(view, 'Teilnahme korrigieren').props.onClick()
  view.render()
  await flush(view)
  assert.equal(input(view, 'Gespeicherter Charaktername'), undefined)
  assert.equal(select(view, 'Gespeicherte Klasse').props.value, 'Priester')
  assert.equal(select(view, 'Gespeicherte Klasse').props.disabled, true)
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Old spec')
  select(view, 'Tatsächlicher Spec').props.onChange(change('Schatten'))
  view.render()
  assert.equal(input(view, 'Teilnahme-Begründung'), undefined)
  view.render()
  const first = form(view).props.onSubmit(event)
  assert.equal('character_name' in writes(s)[0].body, false)
  assert.equal('class' in writes(s)[0].body, false)
  assert.equal(writes(s)[0].body.spec, 'Schatten')
  assert.equal('reason' in writes(s)[0].body, false)
  writes(s)[0].reject(new Error('offline'))
  await first
})

test('raid list includes archived raids, filters history and creates with configured leader and UTC start', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Raids.tsx').RaidsPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [{ ...seriesRow, name_template: 'ICC-{number}', next_raid_number: 7 }] : r.url === '/api/v1/players' ? [player(1), player(2)] : [raid(1), { ...raid(2), archived_at: '2026-10-04T19:00:00Z' }]))
  await flush(view)
  assert.equal(s.requests[0].url, '/api/v1/raids?include_archived=true')
  assert.match(text(view.tree), /Raid 2/)
  select(view, 'Raid-Ansicht').props.onChange(change('upcoming'))
  view.render()
  assert.doesNotMatch(text(view.tree), /Raid 2/)
  select(view, 'Raid-Ansicht').props.onChange(change('history'))
  view.render()
  assert.match(text(view.tree), /Raid 2/)
  assert.doesNotMatch(text(view.tree), /Raid 1/)
  select(view, 'Neue Raidserie').props.onChange(change('3'))
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'datetime-local')[0].props.onChange(change('2026-10-05T20:00'))
  view.render()
  assert.equal(select(view, 'Neuer Raidleiter').props.value, '2')
  assert.match(text(view.tree), /Name wird aus der Namensvorlage abgeleitet:\s+ICC-7/)
  const nameInput = nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Neuer Raidname')[0]
  assert.equal(nameInput, undefined)
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].body.name, '')
  assert.equal(writes(s)[0].body.series_id, 3)
  assert.equal(writes(s)[0].body.raid_leader_id, 2)
  assert.equal(writes(s)[0].body.scheduled_start, new Date('2026-10-05T20:00').toISOString())
  writes(s)[0].resolve(raid(3))
  await first
  assert.deepEqual(s.navigations, ['/raids/3'])
})

test('raid assistants plan series raids ahead up to a date', async () => {
  const s = setup(player(1, 'RAIDASSISTENT'))
  const view = s.mount(s.load('pages/Raids.tsx').RaidsPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [seriesRow] : r.url === '/api/v1/players' ? [player(1), player(2)] : [raid(1)]))
  await flush(view)
  const planForm = () => nodes(view.tree, (n) => n.type === 'form')[1]
  assert.equal(nodes(planForm(), (n) => n.type === 'button')[0].props.disabled, true, 'needs series and date')
  select(view, 'Raidserie für Vorausplanung').props.onChange(change('3'))
  labelled(view, 'Planen bis').props.onChange(change('2026-12-31'))
  view.render()
  const planning = planForm().props.onSubmit(event)
  assert.equal(writes(s)[0].url, '/api/v1/raid-series/3/plan')
  assert.deepEqual(writes(s)[0].body, { until: '2026-12-31' })
  writes(s)[0].resolve([raid(5), raid(6)])
  await flush(view)
  s.requests.at(-1).resolve([raid(1), raid(5), raid(6)])
  await planning
  view.render()
  assert.match(text(view.tree), /2 Raids angelegt\. Die Anmeldung ist offen\./)
  assert.match(text(view.tree), /Raid 6/)
  const { ApiError } = s.load('api.ts')
  const failing = planForm().props.onSubmit(event)
  writes(s)[1].reject(new ApiError(400, 'plan_until_past', 'past'))
  await failing
  view.render()
  assert.match(text(view.tree), /Datum in der Zukunft/)
})

test('members do not see raid planning', async () => {
  const s = setup(player(1, 'MEMBER'))
  const view = s.mount(s.load('pages/Raids.tsx').RaidsPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [seriesRow] : r.url === '/api/v1/players' ? [player(1)] : [raid(1)]))
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Termine im Voraus anlegen/)
})

test('series form keeps a rolling planning window', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidSeries.tsx').RaidSeriesPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [{ ...seriesRow, plan_weeks_ahead: 4 }] : [player(1), player(2)]))
  await flush(view)
  assert.match(text(view.tree), /Automatisch\s+4 Wochen\s+im Voraus geplant/)
  button(view, 'Serie bearbeiten').props.onClick()
  view.render()
  assert.equal(labelled(view, 'Wochen im Voraus').props.value, '4')
  labelled(view, 'Wochen im Voraus').props.onChange(change('8'))
  view.render()
  const saving = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].body.plan_weeks_ahead, 8)
  writes(s)[0].reject(new Error('offline'))
  await saving
})

test('raid creation without a series requires an explicit name', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Raids.tsx').RaidsPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [] : r.url === '/api/v1/players' ? [player(1), player(2)] : [raid(1)]))
  await flush(view)
  assert.match(text(view.tree), /Keine Raidserie vorhanden/)
  assert.doesNotMatch(text(view.tree), /Name folgt der Namensvorlage/)
  select(view, 'Neuer Raidleiter').props.onChange(change('2'))
  const fields = nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'CatalogAssociationFields')[0]
  fields.props.onChange('map:631', '25')
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'datetime-local')[0].props.onChange(change('2026-10-05T20:00'))
  view.render()
  const nameInput = nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Neuer Raidname')[0]
  assert.equal(nameInput.props.required, true)
  form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  nameInput.props.onChange(change('Einzelner Raid'))
  view.render()
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].body.name, 'Einzelner Raid')
  assert.equal('series_id' in writes(s)[0].body, false)
  writes(s)[0].resolve(raid(3))
  await first
  assert.deepEqual(s.navigations, ['/raids/3'])
})

test('admin series form starts empty, validates weekdays, creates and edits full explicit schedule', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidSeries.tsx').RaidSeriesPage)
  s.requests.forEach((r) => r.resolve(r.url === '/api/v1/loot-catalog' ? catalogFixture() : r.url === '/api/v1/raid-series' ? [seriesRow] : [player(1), player(2)]))
  await flush(view)
  assert.equal(select(view, 'Serien-Raidleiter').props.value, '')
  for (const placeholder of ['Serienname', 'Wochenintervall', 'IANA-Zeitzone']) assert.equal(input(view, placeholder).props.value, '')
  assert.equal(button(view, 'Serie speichern').props.disabled, true)
  await form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  input(view, 'Serienname').props.onChange(change('Explicit name')); view.render()
  select(view, 'Serien-Raidleiter').props.onChange(change('2')); view.render()
  input(view, 'Wochenintervall').props.onChange(change('3')); view.render()
  input(view, 'IANA-Zeitzone').props.onChange(change('Europe/Berlin')); view.render()
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'time')[0].props.onChange(change('19:45')); view.render()
  checkbox(view, 'Sonntag').props.onChange({ target: { checked: true } }); view.render()
  nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'CatalogAssociationFields')[0].props.onChange('map:631', '25'); view.render()
  const first = form(view).props.onSubmit(event)
  await form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].url, '/api/v1/raid-series')
  assert.deepEqual(JSON.parse(JSON.stringify(writes(s)[0].body)), { name: 'Explicit name', raid_leader_id: 2, weekdays: [0], interval_weeks: 3, start_time: '19:45', timezone: 'Europe/Berlin', plan_weeks_ahead: 0, catalog_instance_id: 'map:631', raid_size: 25 })
  writes(s)[0].resolve({ ...seriesRow, id: 4 })
  await first
  view.render()
  button(view, 'Serie bearbeiten').props.onClick()
  view.render()
  assert.equal(input(view, 'Wochenintervall').props.value, '2')
  assert.equal(input(view, 'IANA-Zeitzone').props.value, 'Europe/Berlin')
  assert.equal(checkbox(view, 'Sonntag').props.checked, true)
  const edit = form(view).props.onSubmit(event)
  assert.equal(writes(s)[1].method, 'patch')
  assert.equal(writes(s)[1].url, '/api/v1/raid-series/3')
  writes(s)[1].reject(new Error('offline'))
  await edit
  view.render()
  assert.match(text(view.tree), /Speichern fehlgeschlagen/)
})

for (const role of ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER']) {
  test(`${role} cannot configure series`, async () => {
    const s = setup(player(1, role))
    const view = s.mount(s.load('pages/RaidSeries.tsx').RaidSeriesPage)
    s.requests.forEach((r) => r.resolve(r.url === '/api/v1/raid-series' ? [seriesRow] : r.url === '/api/v1/loot-catalog' ? emptyCatalog() : [player(2)]))
    await flush(view)
    assert.equal(button(view, 'Serie bearbeiten'), undefined)
    assert.equal(form(view), undefined)
  })
}

test('guard errors from the server are shown in German', async (t) => {
  t.mock.method(global, 'fetch', async () => ({ status: 403, ok: false, text: async () => JSON.stringify({ error: { code: 'forbidden', message: 'insufficient permissions' } }) }))
  const { api } = setup().load('api.ts')
  await assert.rejects(() => api.get('/api/v1/registration-requests'), (err) => err.code === 'forbidden' && /Berechtigung/.test(err.message))
})

test('registration requests tell non-admins the area is admin-only and load nothing', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const view = s.mount(s.load('pages/RegistrationRequests.tsx').RegistrationRequestsPage)
  await flush(view)
  assert.match(text(view.tree), /nur für Admins/)
  assert.equal(s.requests.some((r) => r.url.includes('registration-requests')), false)
})

test('registration history names the player a redeemed code was linked to', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RegistrationRequests.tsx').RegistrationRequestsPage)
  const request = { id: 7, requester_player_id: null, status: 'CONSUMED', purpose: 'NEW_ACCOUNT', external_source: 'rising-gods', external_profile_id: '1', external_profile_url: '', character_name: 'Mirabelle', class: 'Paladin', main_spec: 'Heilig', second_spec: '', level: 80, race: '', faction: '', created_at: '2026-10-12T10:00:00Z', updated_at: '2026-10-12T10:00:00Z', linked_player: { player_id: 3, player_name: 'Mira', username: 'mira', used_at: '2026-10-12T11:00:00Z' } }
  s.requests.forEach((r) => r.resolve([request]))
  await flush(view)
  const page = text(view.tree)
  assert.match(page, /Verknüpfter Spieler/)
  assert.match(page, /Mira/)
  assert.match(page, /@mira ·\s+12\.10\.2026/)
  assert.match(page, /Eingelöst/)
  assert.ok(nodes(view.tree, (n) => n.props && n.props.to === '/players/3').length > 0, 'links to the player')
})

test('guests get no wishlist controls and no wishlist reads in the catalog', async () => {
  const s = setup({ id: 4, role: 'GUEST' })
  const view = s.mount(s.load('pages/LootCatalog.tsx').LootCatalogPage)
  await flush(view)
  assert.equal(s.requests.some((r) => /players\/4\/characters|wishlist/.test(r.url)), false)
  assert.doesNotMatch(text(view.tree), /Wunschliste/)
})

test('fetch wrapper keeps archive sibling fields and sends session credentials', async (t) => {
  const envelope = { data: raid(1), next_raid: raid(2) }
  const calls = []
  t.mock.method(global, 'fetch', async (url, options) => {
    calls.push({ url, options })
    return { status: 200, ok: true, text: async () => JSON.stringify(envelope) }
  })
  const { api } = setup().load('api.ts')
  assert.deepEqual(await api.postEnvelope('/api/v1/raids/1/archive'), envelope)
  assert.deepEqual(await api.get('/api/v1/raids/1'), envelope.data)
  const post = calls.find((call) => call.options.method === 'POST')
  assert.equal(post.options.credentials, 'include')
})

for (const role of ['GUEST', 'MEMBER']) {
  test(`${role} cannot archive or assign a series`, async () => {
    const s = setup(player(1, role))
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' } })
    await flush(view)
    assert.equal(button(view, 'Archivieren'), undefined)
    assert.equal(button(view, 'Planung speichern'), undefined)
    assert.equal(form(view), undefined)
  })
}

test('missing schedule can be saved with series and UTC start before archive is enabled', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  const start = () => nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Geplanter Start')[0]
  assert.equal(start().props.value, '')
  assert.equal(start().props.required, true)
  assert.equal(button(view, 'Planung speichern').props.disabled, true)
  await button(view, 'Planung speichern').props.onClick()
  assert.equal(writes(s).length, 0)
  select(view, 'Raidserie zuordnen').props.onChange(change('3'))
  start().props.onChange(change('2026-10-05T20:15'))
  view.render()
  assert.equal(button(view, 'Archivieren').props.disabled, true)
  const first = button(view, 'Planung speichern').props.onClick()
  assert.equal(writes(s)[0].body.series_id, 3)
  assert.equal(writes(s)[0].body.scheduled_start, new Date('2026-10-05T20:15').toISOString())
  writes(s)[0].resolve({ ...raid(1), series_id: 3, scheduled_start: writes(s)[0].body.scheduled_start })
  await first
  view.render()
  assert.equal(button(view, 'Archivieren').props.disabled, false)
  assert.equal(button(view, 'Planung speichern').props.disabled, true)
  assert.match(text(view.tree), new RegExp(Intl.DateTimeFormat().resolvedOptions().timeZone))
})

test('schedule prefill is browser-local, time-only updates omit series, and blank cannot clear start', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const initial = '2026-10-04T18:30:47Z'
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: initial } })
  await flush(view)
  const start = () => nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Geplanter Start')[0]
  const date = new Date(initial)
  assert.equal(start().props.value, new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16))
  await button(view, 'Planung speichern').props.onClick()
  assert.equal(writes(s).length, 0)
  start().props.onChange(change(''))
  view.render()
  await button(view, 'Planung speichern').props.onClick()
  assert.equal(writes(s).length, 0)
  start().props.onChange(change('2026-10-05T20:45'))
  view.render()
  const first = button(view, 'Planung speichern').props.onClick()
  assert.equal('series_id' in writes(s)[0].body, false)
  assert.equal(writes(s)[0].body.scheduled_start, new Date('2026-10-05T20:45').toISOString())
  const { ApiError } = s.load('api.ts')
  writes(s)[0].reject(new ApiError(409, 'schedule_not_editable', 'English error'))
  await first
  view.render()
  assert.match(text(view.tree), /Planung eines abgeschlossenen/)
  assert.equal(start().props.value, '2026-10-05T20:45')
})

test('time-only schedule repair without a series does not invent one', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Geplanter Start')[0].props.onChange(change('2026-10-05T20:00'))
  view.render()
  const first = button(view, 'Planung speichern').props.onClick()
  assert.equal('series_id' in writes(s)[0].body, false)
  writes(s)[0].resolve({ ...raid(1), scheduled_start: writes(s)[0].body.scheduled_start })
  await first
  view.render()
  assert.equal(button(view, 'Archivieren').props.disabled, true)
})

test('selecting an existing character locks name and class while the spec stays editable', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('11'))
  view.render()
  await flush(view)
  assert.equal(input(view, 'Gespeicherter Charaktername'), undefined)
  assert.equal(select(view, 'Gespeicherte Klasse').props.value, 'Priest')
  assert.equal(select(view, 'Gespeicherte Klasse').props.disabled, true)
  select(view, 'Tatsächlicher Spec').props.onChange(change('Holy'))
  view.render()
  await flush(view)
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Holy')
  assert.equal(select(view, 'Gespeicherte Klasse').props.value, 'Priest')
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].body.character_name, 'Char 11')
  assert.equal(writes(s)[0].body.class, 'Priest')
  assert.equal(writes(s)[0].body.spec, 'Holy')
  writes(s)[0].reject(new Error('offline'))
  await first
})

test('changing character replaces snapshots and ignores obsolete snapshot handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('11'))
  view.render()
  const oldClass = select(view, 'Gespeicherte Klasse').props.onChange
  const oldSpec = select(view, 'Tatsächlicher Spec').props.onChange
  oldSpec(change('Source spec'))
  view.render()
  select(view, 'Teilnahme-Charakter').props.onChange(change('20'))
  oldClass(change('Magier'))
  oldSpec(change('Obsolete spec'))
  view.render()
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Shadow')
  await flush(view)
  assert.equal(input(view, 'Gespeicherter Charaktername'), undefined)
  assert.equal(select(view, 'Gespeicherte Klasse').props.value, 'Priest')
  assert.equal(select(view, 'Tatsächlicher Spec').props.value, 'Shadow')
})

test('role-only historical PATCH omits untouched unknown and blank snapshots', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [{ ...attendanceRow, character_name: '', class: 'Unknown imported class', spec: '' }] })
  await flush(view)
  button(view, 'Teilnahme korrigieren').props.onClick()
  view.render()
  await flush(view)
  assert.equal(select(view, 'Gespeicherte Klasse').props.value, 'Unknown imported class')
  assert.equal(input(view, 'Gespeicherter Charaktername'), undefined)
  select(view, 'Teilnahme-Rolle').props.onChange(change('TANK'))
  assert.equal(input(view, 'Teilnahme-Begründung'), undefined)
  view.render()
  const first = form(view).props.onSubmit(event)
  for (const field of ['character_name', 'class', 'spec']) assert.equal(field in writes(s)[0].body, false)
  assert.equal('notes' in writes(s)[0].body, false)
  assert.equal(writes(s)[0].body.role, 'TANK')
  writes(s)[0].reject(new Error('offline'))
  await first
})

test('reference changes use selected character defaults and allow explicit spec changes', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [{ ...attendanceRow, character_name: 'Historic name', class: 'Unknown', spec: 'Historic spec' }] })
  await flush(view)
  button(view, 'Teilnahme korrigieren').props.onClick()
  view.render()
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('20'))
  view.render()
  select(view, 'Tatsächlicher Spec').props.onChange(change('Holy'))
  await flush(view)
  assert.equal(input(view, 'Teilnahme-Begründung'), undefined)
  view.render()
  const first = form(view).props.onSubmit(event)
  assert.equal(writes(s)[0].body.character_name, 'Char 20')
  assert.equal(writes(s)[0].body.class, 'Priest')
  assert.equal(writes(s)[0].body.spec, 'Holy')
  writes(s)[0].reject(new Error('offline'))
  await first
})

test('roster_incomplete archive errors identify missing row details and leave corrections available', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' }, attendance: [{ ...attendanceRow, character_name: 'Saved name', class: '', spec: '' }] })
  await flush(view)
  assert.match(text(view.tree), /Teilnahme #.*7.*Klasse, tatsächlicher Spec.*fehlt/)
  const first = (await startArchive(s, view)).done
  const { ApiError } = s.load('api.ts')
  writes(s)[0].reject(new ApiError(409, 'roster_incomplete', 'raid roster is incomplete'))
  await first
  view.render()
  assert.match(text(view.tree), /Teilnahme ist unvollständig/)
  assert.match(text(view.tree), /gültige Klasse, tatsächlicher Spec und Rolle/)
  assert.ok(button(view, 'Teilnahme korrigieren'))
  view.render({ raidID: 2 })
  resolveRaid(s, 2, { raid: { archived_at: '2026-10-04T19:00:00Z' }, attendance: [{ ...attendanceRow, raid_id: 2, class: '', spec: '' }] })
  await flush(view)
  assert.equal(button(view, 'Teilnahme korrigieren'), undefined)
  assert.equal(button(view, 'Planung speichern'), undefined)
  assert.match(text(view.tree), /Klasse unbekannt/)
})

for (const method of ['MAIN', 'SECOND', 'ENHANCED', 'BOE']) {
  test(`${method} records an outside winner using only raid roster ownership`, async () => {
    const s = setup(player(1, 'RAIDASSISTENT'))
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow, { ...attendanceRow, raid_id: 2, character_id: 99 }] }))
    await flush(view)
    const recipients = nodes(suggestions(view, 'Loot-Empfänger'), (n) => n.type === 'option')
    assert.match(recipients.map((n) => n.props.value).join(' '), /Saved raid character/)
    assert.equal(recipients.length, 1)
    assert.equal(s.requests.some((r) => /players\/\d+\/characters/.test(r.url)), false)
    await chooseSourceDrop(s, view)
    select(view, 'Loot-Empfänger').props.onChange(change('20'))
    select(view, 'Loot-Vergabemethode').props.onChange(change(method))
    view.render()
    if (method === 'ENHANCED') assert.match(text(view.tree), /aktuellen EP-Stand.*maximal 50 EP.*nur der Gewinner zahlt/)
    assert.equal(nodes(form(view, 1), (n) => n.type === 'input' && n.props.type === 'number').length, 0)
    assert.equal(button(view, 'Enhance würfeln'), undefined)
    const submit = form(view, 1).props.onSubmit
    const first = submit(event)
    await submit(event)
    assert.equal(writes(s).length, 1)
    const write = writes(s)[0]
    assert.equal(write.url, '/api/v1/raids/1/loot')
    assert.deepEqual(JSON.parse(JSON.stringify(write.body)), {
      catalog_boss_id: 'boss:1', item_id: '12345', award_method: method,
      awarded_to_player_id: 2, awarded_to_character_id: 20, notes: '', request_id: write.body.request_id,
    })
    assert.match(write.body.request_id, /^[0-9a-f-]{36}$/)
    assert.equal(s.requests.some((r) => /\/roll|\/spend/.test(r.url)), false)
    write.reject(new Error('offline'))
    await first
  })
}

test('loot retry keeps the request key and changed payload gets a new key', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
  await flush(view)
  await chooseSourceDrop(s, view)
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  select(view, 'Loot-Vergabemethode').props.onChange(change('ENHANCED'))
  view.render()
  const first = form(view, 1).props.onSubmit(event)
  writes(s)[0].reject(new Error('response lost'))
  await first
  view.render()
  const retry = form(view, 1).props.onSubmit(event)
  assert.deepEqual(writes(s)[1].body, writes(s)[0].body)
  writes(s)[1].reject(new Error('offline'))
  await retry
  view.render()
  const obsoleteSubmit = form(view, 1).props.onSubmit
  input(view, 'Loot-Notizen').props.onChange(change('Changed draft'))
  await obsoleteSubmit(event)
  assert.equal(writes(s).length, 2)
  view.render()
  const changed = form(view, 1).props.onSubmit(event)
  assert.notEqual(writes(s)[2].body.request_id, writes(s)[0].body.request_id)
  writes(s)[2].resolve({ ...lootRow, award_method: 'ENHANCED', enhanced_cost: 50 })
  await flush(view)
  resolveRaid(s, 1, { attendance: [recipientRow], loot: [{ ...lootRow, award_method: 'ENHANCED', enhanced_cost: 50 }] })
  await changed
  view.render()
  assert.match(text(view.tree), /50 EP/)
  assert.equal(input(view, 'Item'), undefined)
})

for (const roster of [[], [{ ...recipientRow, raid_id: 2 }]]) {
  test(`loot cannot record a recipient outside this raid (${roster.length} rows)`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, sourceLoot({ attendance: roster }))
    await flush(view)
    await chooseSourceDrop(s, view)
    select(view, 'Loot-Empfänger').props.onChange(change('20'))
    view.render()
    assert.equal(button(view, 'Vergeben').props.disabled, true)
    await form(view, 1).props.onSubmit(event)
    assert.equal(writes(s).length, 0)
  })
}

test('empty catalog offers no loot selectors and legacy boss names are neither fabricated nor discarded', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [recipientRow], loot: [lootRow, { ...lootRow, id: 9, boss: '', is_main_spec: false }] })
  await flush(view)
  assert.match(text(view.tree), /Old boss/)
  assert.match(text(view.tree), /Boss nicht gespeichert \(historisch\)/)
  assert.equal(nodes(view.tree, (n) => n.type === 'select' && /Boss|Item/.test(n.props['aria-label'] ?? '')).length, 0)
  assert.equal(s.requests.filter((r) => r.url.includes('/loot-catalog')).length, 1)
  assert.equal(nodes(view.tree, (n) => n.type === 'div' && n.props.className === 'subcard loot-boss-group').length, 2)
})

test('notes-only enhanced correction preserves method and historical association without sending a cost', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { loot: [{ ...lootRow, awarded_to_character_id: null, award_method: 'ENHANCED', enhanced_cost: 17 }] })
  await flush(view)
  assert.match(text(view.tree), /17 EP/)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  input(view, 'Loot-Notizen').props.onChange(change('Notes correction'))
  assert.equal(input(view, 'Loot-Begründung'), undefined)
  view.render()
  assert.equal(select(view, 'Loot-Vergabemethode').props.value, 'ENHANCED')
  const first = form(view, 1).props.onSubmit(event)
  const write = writes(s)[0]
  for (const key of ['award_method', 'catalog_boss_id', 'item_id', 'boss', 'item_name', 'awarded_to_character_id', 'awarded_to_player_id', 'enhanced_cost', 'amount', 'request_id']) assert.equal(key in write.body, false)
  assert.equal('reason' in write.body, false)
  write.reject(new Error('offline'))
  await first
})

test('correction switch disables stale field handlers as well as stale submissions', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { loot: [lootRow, { ...lootRow, id: 9, item_name: 'Other item' }] })
  await flush(view)
  const corrections = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Loot korrigieren')
  corrections[0].props.onClick()
  view.render()
  const oldNotes = input(view, 'Loot-Notizen').props.onChange
  corrections[1].props.onClick()
  oldNotes(change('obsolete edit'))
  view.render()
  assert.match(text(form(view, 1)), /Other item/)
  assert.equal(input(view, 'Loot-Notizen').props.value, 'old loot note')
})

test('archived enhanced history shows saved class-colored names and no write controls', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { status: 'COMPLETED', archived_at: '2026-10-04T19:00:00Z' },
    attendance: [recipientRow], loot: [{ ...lootRow, awarded_to_player_id: 2, awarded_to_character_id: 20, award_method: 'ENHANCED', enhanced_cost: 23 }] })
  await flush(view)
  assert.match(text(view.tree), /Saved raid character/)
  assert.match(text(view.tree), /23 EP/)
  assert.match(text(view.tree), /Archivierte Historie/)
  assert.ok(nodes(view.tree, (n) => n.props.className === 'setup-member class-mage').length >= 1)
  assert.ok(nodes(view.tree, (n) => n.type === 'span' && n.props.className === 'character-name class-mage').length >= 1)
  assert.equal(form(view), undefined)
  for (const label of ['Vergeben', 'Loot korrigieren', 'Planung speichern', 'Archivieren', 'Abschließen', 'Enhance würfeln']) assert.equal(button(view, label), undefined)
  assert.equal(writes(s).length, 0)
})

test('catalog failure leaves history readable, never manual recording', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = s.requests.find((r) => r.url === '/api/v1/loot-catalog')
  catalog.resolved = true
  catalog.reject(new Error('catalog offline'))
  resolveRaid(s, 1, { attendance: [recipientRow] })
  await flush(view)
  assert.match(text(view.tree), /Raid 1/)
  assert.equal(input(view, 'Boss'), undefined)
  assert.equal(input(view, 'Item'), undefined)
  assert.equal(button(view, 'Vergeben').props.disabled, true)
  assert.ok(select(view, 'Loot-Empfänger'))
  assert.equal(select(view, 'Loot-Kataloginstanz'), undefined)
})

test('associated loot filters by persisted instance and size, preserves mixed source modes and sends catalog identity', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], catalog })
  await flush(view)
  assert.equal(resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631')), true)
  await flush(view)
  assert.equal(select(view, 'Loot-Kataloginstanz'), undefined)
  for (const placeholder of ['Boss', 'Item', 'Item-ID']) assert.equal(input(view, placeholder), undefined)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  assert.equal(resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25)), true)
  await flush(view)
  const itemSuggestions = nodes(suggestions(view, 'Loot-Item'), (node) => node.type === 'option').map((node) => node.props.value).join(' ')
  assert.match(itemSuggestions, /Katalogitem/)
  assert.doesNotMatch(itemSuggestions, /Item-ID|Spieler|Normal/)
  const variants = nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)
  assert.equal(variants.length, 1)
  assert.equal(select(view, 'Loot-Modus').props.value, 'NORMAL')
  assert.deepEqual(nodes(select(view, 'Loot-Modus'), (node) => node.type === 'option').map((node) => node.props.value), ['NORMAL', 'HEROIC'])
  assert.doesNotMatch(text(select(view, 'Loot-Modus')), /source:|Alle|Lootmodus|Difficulty/)
  select(view, 'Loot-Modus').props.onChange(change('HEROIC'))
  view.render()
  assert.equal(nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value).length, 1)
  select(view, 'Loot-Item').props.onChange(change(nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0].props.value))
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  select(view, 'Loot-Vergabemethode').props.onChange(change('ENHANCED'))
  view.render()
  const first = form(view, 1).props.onSubmit(event)
  const write = writes(s)[0]
  assert.equal(write.body.catalog_boss_id, 'boss:1')
  assert.equal(write.body.award_method, 'ENHANCED')
  assert.match(write.body.request_id, /^[0-9a-f-]{36}$/)
  assert.equal(write.body.item_id, '12345')
  for (const key of ['boss', 'item_name', 'boss_id', 'instance_id', 'raid_size', 'difficulty', 'loot_mode']) assert.equal(key in write.body, false)
  write.reject(new Error('offline'))
  await first
  view.render()
  const retry = form(view, 1).props.onSubmit(event)
  assert.deepEqual(writes(s)[1].body, write.body)
  writes(s)[1].reject(new Error('offline'))
  await retry
})

test('after recording catalog loot the boss and mode stay selected for the next item', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  select(view, 'Loot-Modus').props.onChange(change('HEROIC'))
  view.render()
  select(view, 'Loot-Item').props.onChange(change(nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0].props.value))
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  view.render()
  const submit = form(view, 1).props.onSubmit(event)
  writes(s)[0].resolve({ id: 9 })
  await flush(view)
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], catalog })
  await submit
  await flush(view)
  assert.equal(select(view, 'Loot-Boss').props.value, 'boss:1')
  assert.equal(select(view, 'Loot-Modus').props.value, 'HEROIC')
  assert.equal(select(view, 'Loot-Item').props.value, '')
  assert.equal(select(view, 'Loot-Empfänger').props.value, '')
})

test('loot form: recipient type-ahead, submit hint, awarded marker and Enter moves to the recipient', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  const awarded = { ...lootRow, id: 30, item_id: '12345', item_name: 'Katalogitem', awarded_to_character_id: 20, awarded_to_player_id: 2 }
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], catalog, loot: [awarded] })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  assert.match(text(view.tree), /Zuerst ein Item wählen/)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  const itemOptions = nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option').map((n) => n.props.value)
  assert.match(itemOptions[0], /bereits 1× vergeben/)
  select(view, 'Loot-Item').props.onChange(change(itemOptions[0]))
  view.render()
  assert.match(text(view.tree), /Empfänger-Charakter wählen/)
  select(view, 'Loot-Item').props.onKeyDown({ key: 'Enter', preventDefault() {} })
  select(view, 'Loot-Empfänger').props.onChange(change('Saved'))
  view.render()
  const recipientOptions = nodes(suggestions(view, 'Loot-Empfänger'), (n) => n.type === 'option').map((n) => n.props.value)
  assert.equal(recipientOptions.length, 1)
  assert.match(recipientOptions[0], /Saved raid character · DD/)
  assert.equal(button(view, 'Vergeben').props.disabled, true)
  select(view, 'Loot-Empfänger').props.onChange(change(recipientOptions[0]))
  view.render()
  assert.equal(button(view, 'Vergeben').props.disabled, false)
  assert.doesNotMatch(text(view.tree), /Empfänger-Charakter wählen\./)
})

test('Enter in the item field moves focus to the recipient and other keys do not', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  let focused = 0
  select(view, 'Loot-Empfänger').props.ref.current = { focus: () => { focused += 1 } }
  const enter = () => select(view, 'Loot-Item').props.onKeyDown({ key: 'Enter', preventDefault() {} })
  enter()
  assert.equal(focused, 0, 'no item chosen yet: Enter does not jump')
  select(view, 'Loot-Item').props.onChange(change(nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option')[0].props.value))
  view.render()
  select(view, 'Loot-Item').props.onKeyDown({ key: 'a', preventDefault() {} })
  assert.equal(focused, 0)
  enter()
  assert.equal(focused, 1)
})

test('success messages close themselves after a few seconds', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const timers = []
  const realSetTimeout = global.setTimeout
  global.setTimeout = (fn, ms) => { timers.push({ fn, ms }); return timers.length }
  try {
    resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
    await flush(view)
    await chooseSourceDrop(s, view)
    select(view, 'Loot-Empfänger').props.onChange(change('20'))
    view.render()
    const submit = form(view, 1).props.onSubmit(event)
    writes(s)[0].resolve({ id: 9 })
    await flush(view)
    resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
    await submit
    await flush(view)
    const toast = () => nodes(view.tree, (n) => n.props?.className === 'success-message')
    assert.equal(toast().length, 1)
    assert.match(text(toast()), /Loot vergeben\./)
    const dismiss = timers.filter((t) => t.ms === 8000).pop()
    assert.ok(dismiss, 'an 8 second dismiss timer is armed')
    dismiss.fn()
    view.render()
    assert.equal(toast().length, 0)
  } finally {
    global.setTimeout = realSetTimeout
  }
})

test('empty loot list tells staff how to start', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
  await flush(view)
  assert.match(text(view.tree), /Noch kein Loot vergeben\. Mit „Loot erfassen“/)
})

test('cancelling a raid asks for confirmation and a declined prompt sends nothing', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { status: 'OPEN' }, attendance: [recipientRow] })
  await flush(view)
  s.confirmAnswers.push(false)
  await button(view, 'Absagen').props.onClick()
  assert.equal(writes(s).length, 0)
  assert.match(s.confirms[0], /absagen/)
  const accepted = button(view, 'Absagen').props.onClick()
  assert.equal(writes(s).length, 1)
  assert.match(writes(s)[0].url, /cancel$/)
  writes(s)[0].reject(new Error('offline'))
  await accepted
})

test('unsaved association is preview only; saved context invalidates old loot requests and handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  const items = s.requests.at(-1)
  const oldBossHandler = select(view, 'Loot-Boss').props.onChange
  const oldSubmit = form(view, 1).props.onSubmit
  select(view, 'Raidinstanz').props.onChange(change('map:603'))
  view.render()
  assert.equal(s.requests.some((request) => request.url.includes('instance_id=map%3A603')), false)
  assert.equal(writes(s).length, 0)
  assert.equal(button(view, 'Raidinstanz und Größe speichern').props.disabled, true)
  assert.equal(select(view, 'Raidgröße').props.value, '')
  const sizeOptions = nodes(select(view, 'Raidgröße'), (n) => n.type === 'option' && n.props.value)
  assert.deepEqual(sizeOptions.map((node) => Number(node.props.value)), [10])
  select(view, 'Raidgröße').props.onChange(change('10'))
  view.render()
  const save = button(view, 'Raidinstanz und Größe speichern').props.onClick()
  assert.deepEqual(writes(s)[0].body, { catalog_instance_id: 'map:603', raid_size: 10 })
  writes(s)[0].resolve({ ...raid(1), catalog_instance_id: 'map:603', raid_size: 10 })
  await save
  items.reject(new Error('obsolete failure'))
  oldBossHandler(change('boss:2'))
  await oldSubmit(event)
  await flush(view)
  assert.equal(writes(s).length, 1)
  assert.equal(s.requests.some((request) => request.url.includes('items?boss_id=boss%3A2')), false)
  assert.ok(s.requests.some((request) => request.url === '/api/v1/loot-catalog/bosses?instance_id=map%3A603'))
  assert.equal(select(view, 'Loot-Item').props.disabled, true)
  assert.doesNotMatch(text(view.tree), /Items konnten nicht geladen/)
})

for (const sourceState of ['empty', 'failed', 'missing-provenance']) {
  test(`associated raid with ${sourceState} catalog has no invalid free-text fallback and keeps history editable`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    const catalog = sourceState === 'missing-provenance' ? { ...catalogFixture(), sources: [] } : emptyCatalog()
    if (sourceState === 'failed') {
      const read = s.requests.find((request) => request.url === '/api/v1/loot-catalog')
      read.resolved = true
      read.reject(new Error('offline'))
    }
    resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [recipientRow], loot: [lootRow], catalog })
    await flush(view)
    resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', [])
    await flush(view)
    for (const placeholder of ['Boss', 'Item', 'Item-ID']) assert.equal(input(view, placeholder), undefined)
    assert.equal(select(view, 'Loot-Boss').props.disabled, true)
    assert.equal(button(view, 'Vergeben').props.disabled, true)
    await form(view, 1).props.onSubmit(event)
    assert.equal(writes(s).length, 0)
    assert.match(text(view.tree), /historische Einträge bleiben erhalten/)
    button(view, 'Loot korrigieren').props.onClick()
    view.render()
    input(view, 'Loot-Notizen').props.onChange(change('Historical note only'))
    assert.equal(input(view, 'Loot-Begründung'), undefined)
    view.render()
    const save = form(view, 1).props.onSubmit(event)
    assert.deepEqual(writes(s)[0].body, { notes: 'Historical note only' })
    writes(s)[0].reject(new Error('offline'))
    await save
  })
}

for (const catalogBossID of [null, 'old:catalog-boss']) {
  test(`notes-only associated history preserves saved identity ${catalogBossID} and enhanced cost`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    const historical = { ...lootRow, catalog_boss_id: catalogBossID, award_method: 'ENHANCED', enhanced_cost: 17 }
    resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, loot: [historical], attendance: [attendanceRow], catalog: catalogFixture() })
    await flush(view)
    resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', [])
    await flush(view)
    button(view, 'Loot korrigieren').props.onClick()
    view.render()
    assert.match(text(view.tree), /Old boss/)
    assert.match(text(view.tree), /Old item/)
    assert.match(text(view.tree), /17 EP/)
    input(view, 'Loot-Notizen').props.onChange(change('Corrected note'))
    assert.equal(input(view, 'Loot-Begründung'), undefined)
    view.render()
    const save = form(view, 1).props.onSubmit(event)
    assert.deepEqual(writes(s)[0].body, { notes: 'Corrected note' })
    writes(s)[0].reject(new Error('offline'))
    await save
  })
}

test('associated correction sends changed catalog ID and item ID only; recipient changes use roster ownership', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [attendanceRow, recipientRow], loot: [lootRow], catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  assert.equal(input(view, 'Loot-Begründung'), undefined)
  view.render()
  assert.equal(button(view, 'Loot-Korrektur speichern').props.disabled, true)
  await form(view, 1).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  const drop = nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0]
  select(view, 'Loot-Item').props.onChange(change(drop.props.value))
  view.render()
  const save = form(view, 1).props.onSubmit(event)
  assert.equal(writes(s)[0].method, 'patch')
  assert.deepEqual(writes(s)[0].body, {
    catalog_boss_id: 'boss:1', item_id: '12345', awarded_to_player_id: 2, awarded_to_character_id: 20,
    notes: 'old loot note',
  })
  writes(s)[0].reject(new Error('offline'))
  await save
})

test('same catalog item recipient correction omits unchanged catalog IDs and method', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  const recorded = { ...lootRow, catalog_boss_id: 'boss:1', item_id: '12345', item_name: 'Katalogitem', boss: 'Gespeicherter Boss', award_method: 'ENHANCED', enhanced_cost: 19 }
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, attendance: [attendanceRow, recipientRow], loot: [recorded], catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  resolveIf(s, '/api/v1/loot-catalog/items?boss_id=boss%3A1&raid_size=25', itemDrops(catalog, 'boss:1', 25))
  await flush(view)
  select(view, 'Loot-Item').props.onChange(change(nodes(suggestions(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0].props.value))
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  assert.equal(input(view, 'Loot-Begründung'), undefined)
  view.render()
  const save = form(view, 1).props.onSubmit(event)
  assert.deepEqual(writes(s)[0].body, { awarded_to_player_id: 2, awarded_to_character_id: 20, notes: 'old loot note' })
  writes(s)[0].reject(new Error('offline'))
  await save
})

test('association size-only save sends a partial PATCH and cannot clear persisted values', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, catalog })
  await flush(view)
  select(view, 'Raidinstanz').props.onChange(change(''))
  select(view, 'Raidgröße').props.onChange(change(''))
  view.render()
  assert.equal(select(view, 'Raidinstanz').props.value, 'map:631')
  assert.equal(select(view, 'Raidgröße').props.value, '25')
  assert.equal(button(view, 'Raidinstanz und Größe speichern').props.disabled, true)
  select(view, 'Raidgröße').props.onChange(change('10'))
  view.render()
  const saveHandler = button(view, 'Raidinstanz und Größe speichern').props.onClick
  const save = saveHandler()
  await saveHandler()
  assert.equal(writes(s).length, 1)
  assert.deepEqual(writes(s)[0].body, { raid_size: 10 })
  writes(s)[0].resolve({ ...raid(1), catalog_instance_id: 'map:631', raid_size: 10 })
  await save
  view.render()
  assert.equal(select(view, 'Raidgröße').props.value, '10')
})

test('associating an unassociated raid is explicit and leaves manual drafts unusable after save', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { catalog: catalogFixture(), attendance: [recipientRow] })
  await flush(view)
  input(view, 'Loot-Notizen').props.onChange(change('Unsaved draft'))
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  view.render()
  const oldSubmit = form(view, 1).props.onSubmit
  const oldSave = button(view, 'Raidinstanz und Größe speichern').props.onClick
  select(view, 'Raidinstanz').props.onChange(change('map:603'))
  await oldSave()
  assert.equal(writes(s).length, 0)
  view.render()
  assert.equal(input(view, 'Item'), undefined)
  assert.equal(s.requests.some((request) => request.url.includes('/loot-catalog/bosses')), false)
  select(view, 'Raidgröße').props.onChange(change('10'))
  view.render()
  const save = button(view, 'Raidinstanz und Größe speichern').props.onClick()
  writes(s)[0].resolve({ ...raid(1), catalog_instance_id: 'map:603', raid_size: 10 })
  await save
  await oldSubmit(event)
  await flush(view)
  assert.equal(writes(s).length, 1)
  assert.equal(input(view, 'Item'), undefined)
  assert.equal(button(view, 'Vergeben').props.disabled, true)
  assert.ok(s.requests.some((request) => request.url === '/api/v1/loot-catalog/bosses?instance_id=map%3A603'))
})

test('boss switch discards obsolete item variants and prevents stale variant handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, catalog })
  await flush(view)
  resolveIf(s, '/api/v1/loot-catalog/bosses?instance_id=map%3A631', catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  select(view, 'Loot-Boss').props.onChange(change('boss:1'))
  view.render()
  const oldItems = s.requests.at(-1)
  const oldVariantHandler = select(view, 'Loot-Item').props.onChange
  select(view, 'Loot-Boss').props.onChange(change('boss:2'))
  oldItems.resolve(itemDrops(catalog, 'boss:1', 25))
  oldVariantHandler(change(JSON.stringify(['boss:1', '12345', 25, '0', 1])))
  await flush(view)
  assert.equal(select(view, 'Loot-Item').props.disabled, true)
  assert.equal(select(view, 'Loot-Item').props.value, '')
  assert.doesNotMatch(text(suggestions(view, 'Loot-Item')), /Katalogitem/)
})

for (const code of ['catalog_item_invalid', 'request_id_required']) {
  test(`${code} has actionable German feedback without discarding the source draft`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, sourceLoot({ attendance: [recipientRow] }))
    await flush(view)
    await chooseSourceDrop(s, view)
    select(view, 'Loot-Empfänger').props.onChange(change('20'))
    view.render()
    const save = form(view, 1).props.onSubmit(event)
    const { ApiError } = s.load('api.ts')
    writes(s)[0].reject(new ApiError(400, code, 'English server fallback'))
    await save
    view.render()
    assert.doesNotMatch(text(view.tree), /English server fallback/)
    assert.match(text(view.tree), code === 'catalog_item_invalid' ? /Zuordnung prüfen.*Beute erneut wählen/ : /Speicherschlüssel.*erneut speichern/)
    assert.match(select(view, 'Loot-Item').props.value, /Katalogitem/)
  })
}

test('archived associated history keeps boss groups and saved names without catalog write selectors', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { catalog_instance_id: 'map:631', raid_size: 25, archived_at: '2026-10-04T19:00:00Z' },
    attendance: [recipientRow], loot: [{ ...lootRow, awarded_to_player_id: 2, awarded_to_character_id: 20 }, { ...lootRow, id: 9, boss: '' }], catalog: emptyCatalog() })
  await flush(view)
  assert.match(text(view.tree), /Saved raid character/)
  assert.match(text(view.tree), /Old boss/)
  assert.match(text(view.tree), /Boss nicht gespeichert \(historisch\)/)
  assert.equal(nodes(view.tree, (n) => n.type === 'div' && n.props.className === 'subcard loot-boss-group').length, 2)
  assert.equal(select(view, 'Raidinstanz'), undefined)
  assert.equal(select(view, 'Loot-Boss'), undefined)
  assert.equal(form(view), undefined)
  assert.equal(s.requests.some((request) => request.url.includes('/loot-catalog/bosses')), false)
})

test('imported loot displays the sheet character name even without historical attendance', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { attendance: [], loot: [{ ...lootRow, awarded_to_character_name: 'Opinoid' }] })
  await flush(view)
  assert.match(text(view.tree), /Opinoid/)
  assert.doesNotMatch(text(view.tree), /Charaktername historisch nicht gespeichert/)
})

test('an unrelated historical correction does not discard the pending boss list for the same persisted context', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  const catalog = catalogFixture()
  const options = { raid: { catalog_instance_id: 'map:631', raid_size: 25 }, loot: [lootRow], catalog }
  resolveRaid(s, 1, options)
  await flush(view)
  const bosses = s.requests.find((request) => request.url === '/api/v1/loot-catalog/bosses?instance_id=map%3A631')
  // Keep this response pending independently of the detail refresh below.
  bosses.resolved = true
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  input(view, 'Loot-Notizen').props.onChange(change('Unrelated note'))
  assert.equal(input(view, 'Loot-Begründung'), undefined)
  view.render()
  const save = form(view, 1).props.onSubmit(event)
  writes(s)[0].resolve({ ...lootRow, notes: 'Unrelated note' })
  await flush(view)
  resolveRaid(s, 1, options)
  await save
  bosses.resolve(catalog.bosses.filter((boss) => boss.instance_id === 'map:631'))
  await flush(view)
  assert.equal(select(view, 'Loot-Boss').props.disabled, false)
  assert.match(text(select(view, 'Loot-Boss')), /Gespeicherter Boss/)
})

test('raid workspace opens on roster, preserves drafts between views and resets on raid switch', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  const panel = (id) => nodes(view.tree, (n) => n.props.id === id)[0]
  assert.equal(panel('raid-roster').props.hidden, false)
  assert.equal(panel('raid-loot').props.hidden, true)
  assert.equal(panel('raid-planning').props.hidden, true)
  assert.equal(panel('raid-recruitment').props.hidden, true)
  button(view, 'Gesuchtext').props.onClick()
  view.render()
  assert.equal(panel('raid-recruitment').props.hidden, false)
  input(view, 'Charaktername oder Spieler').props.onChange(change('Keep this draft'))
  nodes(view.tree, (n) => n.type === 'button' && n.props['aria-controls'] === 'raid-loot')[0].props.onClick()
  view.render()
  assert.equal(panel('raid-roster').props.hidden, true)
  assert.equal(panel('raid-loot').props.hidden, false)
  button(view, 'Planung & Details').props.onClick()
  view.render()
  assert.equal(panel('raid-planning').props.hidden, false)
  assert.equal(input(view, 'Charaktername oder Spieler').props.value, 'Keep this draft')
  view.render({ raidID: 2 })
  resolveRaid(s, 2)
  await flush(view)
  assert.equal(panel('raid-roster').props.hidden, false)
  assert.equal(input(view, 'Charaktername oder Spieler').props.value, '')
})

test('roster board groups saved roles, separates the Ersatzbank and preserves note details', () => {
  const s = setup()
  const Board = s.load('components/RaidRosterBoard.tsx').RaidRosterBoard
  const attendance = [
    { ...attendanceRow, id: 1, role: 'TANK', character_name: 'Saved tank', class: 'Krieger', spec: 'Schutz' },
    { ...attendanceRow, id: 2, role: 'HEAL', character_name: 'Saved healer', is_substitute: true, notes: 'Assignment note' },
    { ...attendanceRow, id: 3, role: 'DD', character_name: 'Saved damage' },
    { ...attendanceRow, id: 4, role: 'TANK', character_name: 'Bench tank', is_bench: true },
  ]
  let edited
  const props = { attendance, playerName: () => 'Owner', canWrite: true, pending: false, onEdit: (row) => { edited = row } }
  const tree = Board(props)
  const group = (label) => nodes(tree, (n) => n.props['aria-label'] === label)[0]
  assert.match(text(group('Tanks')), /Saved tank/)
  assert.doesNotMatch(text(group('Tanks')), /Bench tank/)
  assert.match(text(group('Ersatzbank')), /Bench tank/)
  assert.match(text(group('Heilung')), /Saved healer.*Assignment note/)
  assert.equal(nodes(tree, (n) => n.type === 'li').length, 4)
  nodes(tree, (n) => n.type === 'button')[0].props.onClick()
  assert.equal(edited.id, 1)
  assert.equal(nodes(Board({ ...props, canWrite: false }), (n) => n.type === 'button').length, 0)
  assert.ok(nodes(Board({ ...props, pending: true }), (n) => n.type === 'button').every((n) => n.props.disabled))
})

test('profile navigation exposes one area and allows own account controls', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  const panel = (id) => nodes(view.tree, (n) => n.props.id === id)[0]
  assert.equal(panel('profile-points').props.hidden, false)
  assert.equal(panel('profile-locks').props.hidden, true)
  button(view, 'Item-Locks').props.onClick()
  view.render()
  assert.equal(panel('profile-points').props.hidden, true)
  assert.equal(panel('profile-locks').props.hidden, false)
  button(view, 'Konto').props.onClick()
  view.render()
  assert.equal(panel('profile-account').props.hidden, false)
})

for (const role of ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']) {
  test(`${role} archived in-place loot editing respects permission, reason, cancel and save`, async () => {
    const s = setup({ id: 1, role })
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, completed)
    await flush(view)
    if (role === 'GUEST' || role === 'MEMBER') {
      assert.equal(button(view, 'Loot korrigieren'), undefined)
      assert.equal(button(view, 'Teilnahme korrigieren'), undefined)
      assert.equal(form(view), undefined)
      return
    }
    button(view, 'Loot korrigieren').props.onClick()
    view.render()
    input(view, 'Loot-Notizen').props.onChange(change('Discard draft'))
    view.render()
    const abandonedSubmit = form(view, 1).props.onSubmit
    button(view, 'Bearbeitung abbrechen').props.onClick()
    await abandonedSubmit(event)
    view.render()
    assert.equal(writes(s).length, 0)
    assert.equal(button(view, 'Loot-Korrektur speichern'), undefined)
    button(view, 'Loot korrigieren').props.onClick()
    view.render()
    assert.equal(input(view, 'Loot-Notizen').props.value, 'old loot note')
    assert.equal(button(view, 'Loot-Korrektur speichern').props.disabled, true)
    input(view, 'Loot-Begründung').props.onChange(change(' \t '))
    view.render()
    await form(view, 1).props.onSubmit(event)
    assert.equal(writes(s).length, 0)
    input(view, 'Loot-Begründung').props.onChange(change('  archive correction  '))
    input(view, 'Loot-Notizen').props.onChange(change('Saved inline'))
    view.render()
    assert.ok(nodes(view.tree, (n) => n.type === 'tr' && nodes(n, (child) => child.type === 'form').length === 1).length)
    const save = form(view, 1).props.onSubmit(event)
    assert.deepEqual(writes(s)[0].body, { notes: 'Saved inline', reason: 'archive correction' })
    writes(s)[0].resolve({ ...lootRow, notes: 'Saved inline' })
    await flush(view)
    resolveRaid(s, 1, { ...completed, loot: [{ ...lootRow, notes: 'Saved inline' }] })
    await save
    view.render()
    assert.equal(button(view, 'Loot-Korrektur speichern'), undefined)
    assert.match(text(view.tree), /Saved inline/)
    assert.equal(button(view, 'Archivieren'), undefined)
  })
}

test('series editor stays inside its selected card and cancelling invalidates its submit', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidSeries.tsx').RaidSeriesPage)
  for (const request of s.requests) request.resolve(request.url === '/api/v1/raid-series' ? [seriesRow] : request.url === '/api/v1/loot-catalog' ? catalogFixture() : [player(1)])
  await flush(view)
  button(view, 'Serie bearbeiten').props.onClick()
  view.render()
  assert.equal(nodes(view.tree, (n) => n.type === 'li' && nodes(n, (child) => child.type === 'form').length === 1).length, 1)
  const oldSubmit = form(view).props.onSubmit
  button(view, 'Bearbeitung abbrechen').props.onClick()
  await oldSubmit(event)
  view.render()
  assert.equal(writes(s).length, 0)
  assert.equal(nodes(view.tree, (n) => n.type === 'li' && nodes(n, (child) => child.type === 'form').length > 0).length, 0)
})

test('addon invite export loads the roster text and reloads with the bench', async () => {
  const s = setup()
  const view = s.mount(s.load('components/AddonInviteExport.tsx').AddonInviteExport, { raidID: 1, rosterKey: '1:10:false' })
  assert.equal(s.requests[0].url, '/api/v1/raids/1/addon-invite-export')
  s.requests[0].resolve({ text: 'RS2;INV;1;Raid 1;25\nShanna;HEAL;Priester;0' })
  await flush(view)
  const exported = nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Einladungs-Export')[0]
  assert.equal(exported.props.value, 'RS2;INV;1;Raid 1;25\nShanna;HEAL;Priester;0')
  assert.match(text(view.tree), /1\s+Charaktere/)
  assert.equal(button(view, 'Für Addon kopieren').props.disabled, false)
  checkbox(view, 'Ersatzbank einbeziehen').props.onChange({ target: { checked: true } })
  view.render()
  assert.equal(s.requests[1].url, '/api/v1/raids/1/addon-invite-export?bench=1')
})

test('addon loot import previews, requires a boss choice and applies the selection', async () => {
  const s = setup()
  let reloads = 0
  const view = s.mount(s.load('components/AddonLootImport.tsx').AddonLootImport, { raidID: 1, onImported: async () => { reloads++ } })
  const payload = 'RS1|LOOT|1|Rüdiger|1\n1|2|36612|50415|Shanna|given\n2|3|-|52027|Shanna|given\n3|4|-|50415|Fremd|given'
  nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Loot-Export')[0].props.onChange(change(payload))
  view.render()
  button(view, 'Vorschau').props.onClick()
  assert.deepEqual(writes(s)[0].body, { payload })
  const row = (line, extra) => ({ line_key: `key-${line}`, line_number: line, looted_at: null, npc_id: '', item_id: '50415', item_name: `Item ${line}`, recipient_name: 'Shanna', character_id: 10, character_name: 'Shanna', catalog_boss_id: null, boss_options: [], award_method: 'MAIN', is_importable: true, is_already_imported: false, warnings: [], errors: [], ...extra })
  const preview = { raid_id: 1, header_raid_id: 1, recorder: 'Rüdiger', warnings: [], recipient_options: [{ character_id: 10, character_name: 'Shanna' }], rows: [
    row(2, { catalog_boss_id: '36612', boss_options: [{ id: '36612', instance_id: '631', name: 'Lord Mark’gar' }] }),
    row(3, { award_method: 'SECOND', boss_options: [{ id: '37813', instance_id: '631', name: 'Saurfang' }, { id: '36626', instance_id: '631', name: 'Fauldarm' }], warnings: ['Boss nicht eindeutig – bitte auswählen'] }),
    row(4, { recipient_name: 'Fremd', character_id: null, character_name: '', is_importable: false, errors: ['Fremd steht nicht in der Aufstellung'] }),
  ] }
  writes(s)[0].resolve(preview)
  await flush(view)
  assert.match(text(view.tree), /Fremd steht nicht in der Aufstellung/)
  assert.equal(button(view, '2 Gegenstände übernehmen').props.disabled, true)
  assert.match(text(view.tree), /Bitte für alle ausgewählten Zeilen Boss und Empfänger wählen/)
  select(view, 'Boss für Zeile 3').props.onChange(change('37813'))
  view.render()
  select(view, 'Vergabeart für Zeile 2').props.onChange(change('ENHANCED'))
  view.render()
  assert.equal(button(view, '2 Gegenstände übernehmen').props.disabled, false)
  button(view, '2 Gegenstände übernehmen').props.onClick()
  assert.deepEqual(writes(s)[1].body, { payload, selections: [
    { line_key: 'key-2', catalog_boss_id: '36612', character_id: 10, award_method: 'ENHANCED' },
    { line_key: 'key-3', catalog_boss_id: '37813', character_id: 10, award_method: 'SECOND' },
  ] })
  writes(s)[1].resolve([
    { line_key: 'key-2', line_number: 2, status: 'created', award_id: 5, character_name: 'Shanna', code: '', message: '' },
    { line_key: 'key-3', line_number: 3, status: 'failed', award_id: null, character_name: '', code: 'insufficient_balance', message: 'Spieler hat keine EP für Enhanced-Loot' },
  ])
  await flush(view)
  assert.equal(reloads, 1)
  assert.deepEqual(writes(s)[2].body, { payload })
  writes(s)[2].resolve(preview)
  await flush(view)
  assert.match(text(view.tree), /Zeile\s+2\s*:\s*Übernommen\s*·\s*Shanna/)
  assert.match(text(view.tree), /Zeile\s+3\s*:\s*Fehler\s*·\s*Spieler hat keine EP für Enhanced-Loot/)
})

test('addon loot import shows RS2 rolls and preselects held drops with a known winner', async () => {
  const s = setup()
  const view = s.mount(s.load('components/AddonLootImport.tsx').AddonLootImport, { raidID: 1, onImported: async () => {} })
  nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Loot-Export')[0].props.onChange(change('RS2;LOOT;1;Rüdiger;1;2'))
  view.render()
  button(view, 'Vorschau').props.onClick()
  const row = (line, extra) => ({ line_key: `key-${line}`, line_number: line, looted_at: null, npc_id: '', item_id: '50415', item_name: `Item ${line}`, recipient_name: 'Rüdiger', state: 'held', character_id: 20, character_name: 'Shanna', catalog_boss_id: '36612', boss_options: [{ id: '36612', instance_id: '631', name: 'Lord Mark’gar' }], award_method: 'MAIN', roll: null, spec: '', roll_range: '', winner_name: '', enhanced_points: 0, is_importable: true, is_already_imported: false, warnings: [], errors: [], ...extra })
  writes(s)[0].resolve({ raid_id: 1, header_raid_id: 1, recorder: 'Rüdiger', warnings: [], recipient_options: [{ character_id: 20, character_name: 'Shanna' }], rows: [
    row(1, { award_method: 'ENHANCED', roll: 131, spec: 'ms', roll_range: '31-130', winner_name: 'Shanna', enhanced_points: 30 }),
    row(2, { character_id: 10, character_name: 'Rüdiger' }),
    row(3, { state: 'parked', recipient_name: 'Thorgrim', character_id: null, character_name: '', is_importable: false, warnings: ['Wird von Thorgrim verwahrt und ist noch nicht verlost – wird nicht übernommen'] }),
    row(4, { state: 'deleted', is_importable: false, character_id: null, character_name: '' }),
    row(5, { state: 'disenchanted', is_importable: false, character_id: null, character_name: '' }),
    row(6, { state: 'bank', is_importable: false, character_id: null, character_name: '' }),
  ] })
  await flush(view)
  const boxes = nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')
  assert.deepEqual(boxes.map((box) => box.props.checked), [true, false, false, false, false, false])
  assert.match(text(view.tree), /Wurf 131 \(31-130, Main-Spec, Enhanced 30 EP\)/)
  assert.match(text(view.tree), /verwahrt, noch nicht verlost/)
  assert.match(text(view.tree), /im Addon gelöscht/)
  assert.match(text(view.tree), /entzaubert/)
  assert.match(text(view.tree), /für die Gildenbank/)
  assert.equal(select(view, 'Vergabeart für Zeile 1').props.value, 'ENHANCED')
})

test('character profile shows the wishlist to its owner and staff only', async () => {
  for (const [me, owner, shown, editable] of [[{ id: 1, role: 'MEMBER' }, 1, true, true], [{ id: 2, role: 'MEMBER' }, 1, false, false], [{ id: 2, role: 'RAIDASSISTENT' }, 1, true, false], [{ id: 1, role: 'GUEST' }, 1, true, false]]) {
    const s = setup(me)
    const view = s.mount(s.load('pages/CharacterDetail.tsx').CharacterDetailPage, { characterID: 10 })
    resolveIf(s, '/api/v1/characters/10', character(10, owner))
    await flush(view)
    const wishlist = nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'CharacterWishlist')
    assert.equal(wishlist.length, shown ? 1 : 0, JSON.stringify(me))
    if (shown) assert.equal(wishlist[0].props.canEdit, editable, JSON.stringify(me))
  }
})

// Node 20 has no global navigator, so the stub replaces the whole global and restores it afterwards.
function stubClipboard(writeText) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  Object.defineProperty(globalThis, 'navigator', { configurable: true, writable: true, value: { clipboard: { writeText } } })
  return () => {
    if (previous) Object.defineProperty(globalThis, 'navigator', previous)
    else delete globalThis.navigator
  }
}

test('character wishlist saves the roll choice and exports it for the addon', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('components/CharacterWishlist.tsx').CharacterWishlist, { characterID: 10, canEdit: true })
  const wish = (id, extra) => ({ id, player_id: 1, character_id: 10, item_id: String(50000 + id), item_name: `Item ${id}`, priority: 0, notes: '', spec: 'MAIN', is_enhanced: false, created_at: '', updated_at: '', ...extra })
  resolveIf(s, '/api/v1/characters/10/wishlist', [wish(1, { priority: 2 }), wish(2)])
  await flush(view)
  assert.equal(select(view, 'Enhanced für Item 1').props.disabled, false)
  select(view, 'Enhanced für Item 1').props.onChange({ target: { checked: true } })
  assert.equal(writes(s)[0].url, '/api/v1/characters/10/wishlist/items/50001')
  assert.deepEqual(writes(s)[0].body, { priority: 2, notes: '', spec: 'MAIN', is_enhanced: true })
  writes(s)[0].resolve(wish(1, { priority: 2, is_enhanced: true }))
  await flush(view)
  assert.equal(select(view, 'Enhanced für Item 1').props.checked, true)
  // Second spec cannot be enhanced, so switching clears it.
  select(view, 'Spec für Item 1').props.onChange(change('SECOND'))
  assert.deepEqual(writes(s)[1].body, { priority: 2, notes: '', spec: 'SECOND', is_enhanced: false })
  writes(s)[1].resolve(wish(1, { priority: 2, spec: 'SECOND' }))
  await flush(view)
  assert.equal(select(view, 'Enhanced für Item 1').props.disabled, true)
  nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Nach oben')[1].props.onClick()
  assert.deepEqual(writes(s)[2].body, { priority: 3, notes: '', spec: 'MAIN', is_enhanced: false })
  writes(s)[2].resolve(wish(2, { priority: 3 }))
  await flush(view)
  assert.match(text(view.tree), /Item 2.*Item 1/)
  // The export box explains the three steps and copies in one click.
  assert.match(text(view.tree), /Ins Wunschlisten-Addon übernehmen.*Export kopieren.*\/wish import.*Strg\+V/)
  let clipboard = ''
  const restoreNavigator = stubClipboard(async (value) => { clipboard = value })
  const copying = button(view, 'Export kopieren').props.onClick()
  const exportRequest = s.requests.find((r) => r.url === '/api/v1/characters/10/wishlist/addon-export')
  exportRequest.resolve({ text: 'WL2;Char;1;70\n50002;ms;0;1;\n50001;os;0;2;' })
  await copying
  await flush(view)
  assert.equal(clipboard, 'WL2;Char;1;70\n50002;ms;0;1;\n50001;os;0;2;')
  assert.match(text(view.tree), /Kopiert – jetzt im Spiel \/wish import/)
  assert.equal(nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Wunschlisten-Export')[0].props.value, 'WL2;Char;1;70\n50002;ms;0;1;\n50001;os;0;2;')
  restoreNavigator()
})

test('wishlist export falls back to manual copy when the clipboard is blocked', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('components/CharacterWishlist.tsx').CharacterWishlist, { characterID: 10, canEdit: true })
  resolveIf(s, '/api/v1/characters/10/wishlist', [{ id: 1, player_id: 1, character_id: 10, item_id: '50001', item_name: 'Item 1', priority: 0, notes: '', spec: 'MAIN', is_enhanced: false, created_at: '', updated_at: '' }])
  await flush(view)
  const restoreNavigator = stubClipboard(async () => { throw new Error('denied') })
  const copying = button(view, 'Export kopieren').props.onClick()
  s.requests.find((r) => r.url === '/api/v1/characters/10/wishlist/addon-export').resolve({ text: 'WL2;Char;1;0' })
  await copying
  await flush(view)
  assert.match(text(view.tree), /markieren und mit Strg\+C kopieren/)
  assert.equal(nodes(view.tree, (n) => n.type === 'textarea')[0].props.value, 'WL2;Char;1;0')
  restoreNavigator()
})

test('empty wishlist points to the loot catalog', async () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const view = s.mount(s.load('components/CharacterWishlist.tsx').CharacterWishlist, { characterID: 10, canEdit: true })
  resolveIf(s, '/api/v1/characters/10/wishlist', [])
  await flush(view)
  assert.match(text(view.tree), /Noch keine Wünsche\.\s+Im\s+Beute-Katalog\s+bei einem Item auf deinen Charakter klicken/)
  assert.equal(button(view, 'Export kopieren'), undefined)
})

test('character wishlist is read-only for staff', async () => {
  const s = setup({ id: 2, role: 'RAIDASSISTENT' })
  const view = s.mount(s.load('components/CharacterWishlist.tsx').CharacterWishlist, { characterID: 10, canEdit: false })
  resolveIf(s, '/api/v1/characters/10/wishlist', [{ id: 1, player_id: 1, character_id: 10, item_id: '50001', item_name: 'Item 1', priority: 0, notes: '', spec: 'SECOND', is_enhanced: false, created_at: '', updated_at: '' }])
  await flush(view)
  assert.equal(nodes(view.tree, (n) => n.type === 'select').length, 0)
  assert.equal(button(view, 'Entfernen'), undefined)
  assert.match(text(view.tree), /Second-Spec/)
  assert.ok(button(view, 'Export kopieren'))
})

test('addon loot import lets staff pick the winner and leaves held drops unselected', async () => {
  const s = setup()
  const view = s.mount(s.load('components/AddonLootImport.tsx').AddonLootImport, { raidID: 1, onImported: async () => {} })
  const payload = 'RS1|LOOT|1|Rüdiger|1\n1|2|36612|50415|Helfer|given\n2|3|36612|50415|Rüdiger|held'
  nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Loot-Export')[0].props.onChange(change(payload))
  view.render()
  button(view, 'Vorschau').props.onClick()
  const row = (line, extra) => ({ line_key: `key-${line}`, line_number: line, looted_at: null, npc_id: '36612', item_id: '50415', item_name: `Item ${line}`, recipient_name: '', state: 'given', character_id: null, character_name: '', catalog_boss_id: '36612', boss_options: [{ id: '36612', instance_id: '631', name: 'Lord Mark’gar' }], award_method: 'MAIN', is_importable: true, is_already_imported: false, warnings: [], errors: [], ...extra })
  writes(s)[0].resolve({ raid_id: 1, header_raid_id: 1, recorder: 'Rüdiger', warnings: [], recipient_options: [{ character_id: 10, character_name: 'Rüdiger' }, { character_id: 20, character_name: 'Shanna' }], rows: [
    row(1, { recipient_name: 'Helfer', warnings: ['Helfer steht nicht in der Aufstellung'] }),
    row(2, { recipient_name: 'Rüdiger', state: 'held', character_id: 10, character_name: 'Rüdiger', warnings: ['Noch beim Plündermeister – Empfänger prüfen'] }),
  ] })
  await flush(view)
  const boxes = nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')
  assert.equal(boxes[0].props.checked, false)
  assert.equal(boxes[1].props.checked, false)
  assert.match(text(view.tree), /Addon:\s*Helfer/)
  assert.match(text(view.tree), /beim Plündermeister/)
  assert.equal(select(view, 'Empfänger für Zeile 2').props.value, '10')
  boxes[0].props.onChange({ target: { checked: true } })
  view.render()
  assert.equal(button(view, '1 Gegenstand übernehmen').props.disabled, true)
  assert.match(text(view.tree), /Boss und Empfänger wählen/)
  select(view, 'Empfänger für Zeile 1').props.onChange(change('20'))
  view.render()
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')[1].props.onChange({ target: { checked: true } })
  view.render()
  select(view, 'Empfänger für Zeile 2').props.onChange(change('20'))
  view.render()
  button(view, '2 Gegenstände übernehmen').props.onClick()
  assert.deepEqual(writes(s)[1].body, { payload, selections: [
    { line_key: 'key-1', catalog_boss_id: '36612', character_id: 20, award_method: 'MAIN' },
    { line_key: 'key-2', catalog_boss_id: '36612', character_id: 20, award_method: 'MAIN' },
  ] })
})

test('addon roster import adds known characters and hands unknown ones to quick-add', async () => {
  const s = setup()
  let reloads = 0
  const quickAdds = []
  const view = s.mount(s.load('components/AddonRosterImport.tsx').AddonRosterImport, { raidID: 1, onImported: async () => { reloads++ }, onQuickAdd: (name) => quickAdds.push(name) })
  const payload = 'RS1|ROSTER|1|Rüdiger|5\nRüdiger|MAGE|1\nShanna|PRIEST|2\nWeltchat|PALADIN|3'
  nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Aufstellungs-Export')[0].props.onChange(change(payload))
  view.render()
  button(view, 'Abgleichen').props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/addon-roster-import/preview')
  assert.deepEqual(writes(s)[0].body, { payload })
  const row = (line, extra) => ({ line_number: line, name: '', class_token: '', first_seen: null, status: 'unknown', character_id: null, character_name: '', player_id: null, class: '', suggested_role: 'DD', suggest_late_join: false, errors: [], ...extra })
  const preview = { raid_id: 1, header_raid_id: 1, recorder: 'Rüdiger', warnings: [], missing: ['Thorgrim'], rows: [
    row(2, { name: 'Rüdiger', status: 'in_roster', character_id: 10, character_name: 'Rüdiger', class: 'Magier' }),
    row(3, { name: 'Shanna', status: 'known', character_id: 20, character_name: 'Shanna', class: 'Priester', suggested_role: 'HEAL', suggest_late_join: true }),
    row(4, { name: 'Weltchat' }),
  ] }
  writes(s)[0].resolve(preview)
  await flush(view)
  assert.match(text(view.tree), /im Spiel nicht gesehen:\s*Thorgrim/)
  assert.match(text(view.tree), /Bereits in der Aufstellung/)
  assert.equal(select(view, 'Rolle für Shanna').props.value, 'HEAL')
  const late = nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Shanna später dazugekommen')[0]
  assert.equal(late.props.checked, true)
  assert.equal(nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Rüdiger übernehmen').length, 0)
  button(view, 'Per Schnellanlage hinzufügen').props.onClick()
  assert.deepEqual(quickAdds, ['Weltchat'])
  select(view, 'Rolle für Shanna').props.onChange(change('DD'))
  view.render()
  button(view, '1 Charakter hinzufügen').props.onClick()
  assert.equal(writes(s)[1].url, '/api/v1/raids/1/addon-roster-import')
  assert.deepEqual(writes(s)[1].body, { payload, selections: [{ character_id: 20, role: 'DD', late_join: true }] })
  writes(s)[1].resolve([{ character_id: 20, character_name: 'Shanna', status: 'added', attendance_id: 7, code: '', message: '' }])
  await flush(view)
  assert.equal(reloads, 1)
  assert.deepEqual(writes(s)[2].body, { payload })
  writes(s)[2].resolve(preview)
  await flush(view)
  assert.match(text(view.tree), /Shanna\s*:\s*Hinzugefügt/)
})

test('roster board opens the add section when the roster import hands over a character', () => {
  const s = setup()
  const Board = s.load('components/RaidRosterBoard.tsx').RaidRosterBoard
  const props = { attendance: [], playerName: () => '', canWrite: true, pending: false, onEdit: () => {} }
  const disclosure = (view) => nodes(view.tree, (n) => n.type === 'details')[0]
  assert.equal(disclosure(s.mount(Board, props)).props.open, undefined)
  assert.equal(disclosure(s.mount(Board, { ...props, isAddOpen: true })).props.open, true)
})

test('quick-add starts with the name handed over from the roster import', () => {
  const s = setup()
  const view = s.mount(s.load('components/RaidQuickAdd.tsx').RaidQuickAdd, { raidID: 1, players: [], options: {}, needsReason: false, pending: false, submit: async (action) => action(), isActive: () => true, onAdded: async () => {}, onCancel: () => {}, initialQuery: 'Weltchat' })
  const field = input(view, 'Mindestens 3 Zeichen')
  assert.equal(field.props.value, 'Weltchat')
  assert.equal(field.props.autoFocus, true)
  assert.equal(button(view, 'Suchen').props.disabled, false)
  assert.equal(s.requests.length, 0)
})


test('anonymous dashboard shows public widgets without profile or personal requests', async () => {
  const s = setup(null)
  const { DashboardPage } = s.load('pages/Dashboard.tsx')
  const view = s.mount(DashboardPage)
  assert.equal(nodes(view.tree, (node) => node.props?.className === 'card widget-profile').length, 0)
  assert.deepEqual(s.requests.map((request) => request.url), [
    '/api/v1/raids?include_archived=true',
    '/api/v1/players',
    '/api/v1/polls',
  ])
  for (const request of s.requests) request.resolve([])
  await flush(view)
  assert.match(text(view.tree), /Gildenübersicht|Bratwurst des Schicksals/)
  assert.equal(s.requests.some((request) => /\/points|\/characters|\/votes/.test(request.url)), false)
  view.unmount()
})

test('a member can announce an absence with a reason and take it back before the raid starts', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [], absences: [] })
  resolveIf(s, '/api/v1/players/2/characters', [])
  await flush(view)
  assert.match(text(view.tree), /Kannst du nicht kommen/)
  const reason = nodes(view.tree, (n) => n.type === 'input' && n.props.placeholder?.startsWith('z. B.'))[0]
  reason.props.onChange(change('Krank'))
  view.render()
  button(view, 'Ich kann nicht kommen').props.onClick()
  assert.equal(writes(s)[0].method, 'put')
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/absences/2')
  assert.deepEqual(writes(s)[0].body, { reason: 'Krank', status: 'DECLINED' })
})

test('a member can say they are unsure and later confirm with "Sicher dabei"', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [], absences: [] })
  resolveIf(s, '/api/v1/players/2/characters', [])
  await flush(view)
  nodes(view.tree, (n) => n.type === 'input' && n.props.placeholder?.startsWith('z. B.'))[0].props.onChange(change('komme evtl. später'))
  view.render()
  button(view, 'Ich bin unsicher').props.onClick()
  assert.deepEqual(writes(s)[0].body, { reason: 'komme evtl. später', status: 'TENTATIVE' })

  const unsure = setup({ id: 2, role: 'MEMBER' })
  const second = unsure.mount(unsure.load('components/RaidSignups.tsx').RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }], onRosterChange: async () => {} })
  resolveIf(unsure, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [], absences: [{ player_id: 2, reason: 'Spätschicht', status: 'TENTATIVE', created_at: '2030-01-01T00:00:00Z' }] })
  resolveIf(unsure, '/api/v1/players/2/characters', [])
  await flush(second)
  assert.match(text(second.tree), /Du bist unsicher.*Spätschicht/s)
  assert.doesNotMatch(text(second.tree), /Du hast abgesagt/)
  assert.equal(nodes(second.tree, (n) => n.type === 'button' && text(n) === 'Ich bin unsicher').length, 0)
  nodes(second.tree, (n) => n.type === 'button' && text(n) === 'Ich kann nicht kommen')[0].props.onClick()
  assert.deepEqual(writes(unsure)[0].body, { reason: '', status: 'DECLINED' })
  writes(unsure)[0].reject(new Error('offline'))
  await flush(second)
  nodes(second.tree, (n) => n.type === 'button' && text(n) === 'Sicher dabei')[0].props.onClick()
  assert.equal(writes(unsure)[1].method, 'del')
})

test('staff can mark a player as unsure; unsure is listed apart from declined', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const view = s.mount(s.load('components/RaidSignups.tsx').RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 1, name: 'Leitung', active: true }, { id: 2, name: 'Wackler', active: true }, { id: 3, name: 'Krank', active: true }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [], absences: [{ player_id: 3, reason: '', status: 'DECLINED', created_at: '2030-01-01T00:00:00Z' }] })
  resolveIf(s, '/api/v1/players/1/characters', [])
  await flush(view)
  assert.match(text(view.tree), /1\s*abgesagt.*Krank.*Hat abgesagt/s)
  select(view, 'Spieler markieren').props.onChange(change('2'))
  view.render()
  button(view, 'Als unsicher markieren').props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/absences/2')
  assert.deepEqual(writes(s)[0].body, { reason: '', status: 'TENTATIVE' })
})

test('an announced absence shows the reason and can be revoked with "Doch dabei"', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = s.load('components/RaidSignups.tsx')
  const view = s.mount(RaidSignups, { raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' }, attendance: [], players: [{ id: 2, name: 'Ich' }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, entries: [], absences: [{ player_id: 2, reason: 'Urlaub', created_at: '2030-01-01T00:00:00Z' }] })
  resolveIf(s, '/api/v1/players/2/characters', [])
  await flush(view)
  assert.match(text(view.tree), /Du hast abgesagt.*Urlaub/s)
  assert.equal(button(view, 'Ich kann nicht kommen'), undefined)
  button(view, 'Doch dabei').props.onClick()
  assert.equal(writes(s)[0].method, 'del')
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/absences/2')
})

test('no absence form once the raid has started; staff can still manage absences', async () => {
  const started = setup({ id: 2, role: 'MEMBER' })
  const { RaidSignups } = started.load('components/RaidSignups.tsx')
  const old = { ...raid(8), scheduled_start: '2000-01-01T20:00:00Z' }
  const view = started.mount(RaidSignups, { raid: old, attendance: [], players: [], onRosterChange: async () => {} })
  resolveIf(started, '/api/v1/raids/8/signups', { is_enabled: false, is_open: false, class_specs: {}, entries: [], absences: [] })
  resolveIf(started, '/api/v1/players/2/characters', [])
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Kannst du nicht kommen/)

  const s = setup({ id: 9, role: 'RAIDASSISTENT' })
  const staffView = s.mount(s.load('components/RaidSignups.tsx').RaidSignups, { raid: old, attendance: [], players: [{ id: 3, name: 'Tom', active: true }, { id: 4, name: 'Eva', active: true }], onRosterChange: async () => {} })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: false, is_open: false, class_specs: {}, entries: [], absences: [{ player_id: 3, reason: 'Telefonisch', created_at: '2030-01-01T00:00:00Z' }] })
  resolveIf(s, '/api/v1/players/9/characters', [])
  await flush(staffView)
  assert.match(text(staffView.tree), /Tom.*Hat abgesagt.*Telefonisch/s)
  button(staffView, 'Absage zurücknehmen').props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/absences/3')
  const options = nodes(staffView.tree, (n) => n.type === 'option').map((n) => text(n))
  assert.ok(options.includes('Eva') && !options.includes('Tom'), 'only players without absence are offered')
})

test('roster rows of players who declined carry a badge and reason', () => {
  const board = setup().load('components/RaidRosterBoard.tsx').RaidRosterBoard({
    attendance: [{ id: 1, player_id: 3, character_name: 'A', class: 'Magier', spec: 'Feuer', role: 'DD' }, { id: 2, player_id: 4, character_name: 'B', class: 'Magier', spec: 'Feuer', role: 'DD' }],
    playerName: () => '', canWrite: false, pending: false, onEdit: () => {}, absences: new Map([[3, 'Krank']]),
  })
  const flat = text(board)
  assert.equal((flat.match(/Hat abgesagt/g) ?? []).length, 1)
  assert.match(flat, /Grund:\s+Krank/)
})

test('archive review shows who gets which EP, who gets none because of loot, and declined players get no EP unless staff ticks them', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, {
    raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' },
    attendance: [recipientRow, { ...recipientRow, id: 10, player_id: 1 }],
    loot: [{ id: 1, raid_id: 1, item_name: 'Schwert', awarded_to_player_id: 1, is_main_spec: true }],
    absences: [{ player_id: 2, reason: 'Krank', created_at: '2026-10-01T00:00:00Z' }],
  })
  await flush(view)
  const opening = button(view, 'Archivieren').props.onClick()
  assert.equal(writes(s).length, 0, 'the first click only opens the review')
  const request = s.requests.find((r) => r.url.endsWith('/archive-preview'))
  request.resolve([{ player_id: 1, balance: 40, award: 0, reason: 'main_spec_loot' }, { player_id: 2, balance: 25, award: 10, reason: 'award' }])
  await opening
  view.render()
  assert.equal(s.confirms.length, 0)
  let flat = text(view.tree)
  assert.match(flat, /Keine EP: Main-Spec-Loot \(Schwert\)/)
  assert.match(flat, /25\s+0\s+25/, 'declined players get no EP by default')
  assert.match(flat, /trotzdem EP geben/)
  const check = nodes(view.tree, (n) => n.type === 'label' && /trotzdem EP geben/.test(text(n)))[0]
  nodes(check, (n) => n.type === 'input')[0].props.onChange({ target: { checked: true } })
  view.render()
  flat = text(view.tree)
  assert.match(flat, /25\s+\+10\s+35/)
  nodes(nodes(view.tree, (n) => n.type === 'label' && /trotzdem EP geben/.test(text(n)))[0], (n) => n.type === 'input')[0].props.onChange({ target: { checked: false } })
  view.render()
  const done = button(view, 'Jetzt archivieren').props.onClick()
  assert.equal(writes(s).length, 1)
  assert.deepEqual(writes(s)[0].body, { without_points: [2] })
  writes(s)[0].reject(new Error('offline'))
  await done
})

test('archive review can be left with "Zurück" without sending anything', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' }, attendance: [recipientRow] })
  await flush(view)
  const opening = button(view, 'Archivieren').props.onClick()
  s.requests.find((r) => r.url.endsWith('/archive-preview')).resolve([])
  await opening
  view.render()
  button(view, 'Zurück').props.onClick()
  view.render()
  assert.ok(button(view, 'Archivieren'))
  assert.equal(writes(s).length, 0)
})

test('staff can bench a declined player straight from the roster', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' }, attendance: [recipientRow, { ...recipientRow, id: 10, player_id: 1 }], absences: [{ player_id: 2, reason: '', created_at: '2026-10-01T00:00:00Z' }] })
  await flush(view)
  const buttons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Auf die Bank setzen')
  assert.equal(buttons.length, 1, 'only the declined player offers it')
  const done = buttons[0].props.onClick()
  assert.equal(writes(s)[0].method, 'patch')
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/attendance/9')
  assert.deepEqual(writes(s)[0].body, { is_bench: true })
  writes(s)[0].reject(new Error('offline'))
  await done
})

test('staff can remove a participant from an open raid after confirming', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { series_id: 3, scheduled_start: '2026-10-04T18:30:00Z' }, attendance: [recipientRow] })
  await flush(view)
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Aus dem Raid entfernen').length, 0, 'only offered while editing a row')
  nodes(view.tree, (n) => n.type === 'button' && n.props.title === 'Teilnahme korrigieren')[0].props.onClick()
  view.render()
  s.confirmAnswers.push(false)
  await button(view, 'Aus dem Raid entfernen').props.onClick()
  assert.equal(writes(s).length, 0, 'declining the confirmation sends nothing')
  assert.match(s.confirms[0], /Saved raid character wirklich aus dem Raid entfernen/)
  const done = button(view, 'Aus dem Raid entfernen').props.onClick()
  assert.equal(writes(s)[0].method, 'del')
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/attendance/9')
  const { ApiError } = s.load('api.ts')
  writes(s)[0].reject(new ApiError(409, 'attendance_has_loot', 'loot'))
  await done
  view.render()
  assert.match(text(view.tree), /zuerst den Loot zurückziehen/)
})

test('completed raids do not offer removing participants', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { raid: { status: 'COMPLETED' }, attendance: [recipientRow] })
  await flush(view)
  nodes(view.tree, (n) => n.type === 'button' && n.props.title === 'Teilnahme korrigieren')[0].props.onClick()
  view.render()
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Aus dem Raid entfernen').length, 0)
})

test('recruitment template is loaded from the server, saved explicitly and the refresh only reloads the roster', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { RaidRecruitment } = s.load('pages/Recruitment.tsx')
  const view = s.mount(RaidRecruitment, { raidID: 8, seriesID: 3, attendance: [] })
  const setupGrid = Array.from({ length: 5 }, () => ['Maintank', 'Offtank', 'Leer', 'Irgendein MDD', 'Irgendein RDD'])
  resolveIf(s, '/api/v1/raids/8/recruitment-template', { text: 'Gespeicherter Text {{TANK}}', setup: setupGrid, updated_by: 1, updated_at: '2030-01-01T10:00:00Z' })
  await flush(view)
  assert.equal(nodes(view.tree, (n) => n.type === 'textarea' && !n.props.readOnly)[0].props.value, 'Gespeicherter Text {{TANK}}')
  assert.equal(button(view, 'Vorlage speichern').props.disabled, true, 'nothing to save yet')
  assert.match(text(view.tree), /Gespeichert\./)
  nodes(view.tree, (n) => n.type === 'textarea' && !n.props.readOnly)[0].props.onChange(change('Neuer Text {{HEAL}}'))
  view.render()
  assert.match(text(view.tree), /Ungespeicherte Änderungen/)
  const saving = button(view, 'Vorlage speichern').props.onClick()
  const put = writes(s)[0]
  assert.equal(put.method, 'put')
  assert.equal(put.url, '/api/v1/raids/8/recruitment-template')
  assert.equal(put.body.text, 'Neuer Text {{HEAL}}')
  put.resolve({ text: 'Neuer Text {{HEAL}}', setup: setupGrid, updated_by: 1, updated_at: '2030-01-02T10:00:00Z' })
  await saving
  await flush(view)
  assert.equal(button(view, 'Vorlage speichern').props.disabled, true)
  // The 15 second refresh asks for the roster only.
  const before = s.requests.length
  s.intervals.at(-1)()
  const fresh = s.requests.slice(before)
  assert.deepEqual(fresh.map((r) => r.url), ['/api/v1/raids/8/attendance'])
  fresh[0].resolve([])
})

test('a failed template save keeps the draft and shows the error', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const { RaidRecruitment } = s.load('pages/Recruitment.tsx')
  const { ApiError } = s.load('api.ts')
  const view = s.mount(RaidRecruitment, { raidID: 8, seriesID: null, attendance: [] })
  resolveIf(s, '/api/v1/raids/8/recruitment-template', null)
  await flush(view)
  nodes(view.tree, (n) => n.type === 'textarea' && !n.props.readOnly)[0].props.onChange(change('Entwurf'))
  view.render()
  const saving = button(view, 'Vorlage speichern').props.onClick()
  writes(s)[0].reject(new ApiError(403, 'forbidden', 'Dafür fehlt dir die Berechtigung.'))
  await saving
  await flush(view)
  assert.match(text(view.tree), /Dafür fehlt dir die Berechtigung/)
  assert.equal(nodes(view.tree, (n) => n.type === 'textarea' && !n.props.readOnly)[0].props.value, 'Entwurf')
  assert.equal(button(view, 'Vorlage speichern').props.disabled, false)
})

test('audit log translates technical snapshots into meaningful changes and stays admin-only', async () => {
  const admin = setup({ id: 1, name: 'Admin', role: 'ADMIN' })
  const view = admin.mount(admin.load('pages/AuditLog.tsx').AuditLogPage)
  assert.equal(admin.requests.length, 1)
  assert.equal(admin.requests[0].url, '/api/v1/audit-log')
  admin.requests[0].resolve([{
    id: 1, actor_id: 1, actor_name: 'Admin', entity_type: 'attendance', entity_id: '42', action: 'correction',
    before_json: '{"raid_id":7,"character_name":"Kleinesonne","class":"Krieger","spec":"Waffen","role":"DD","is_bench":false}',
    after_json: '{"raid_id":7,"character_name":"Kleinesonne","class":"Krieger","spec":"Waffen","role":"HEAL","is_bench":false}',
    reason: 'Rolle korrigiert', created_at: '2026-10-04T18:00:00Z',
  }])
  await flush(view)
  assert.match(text(view.tree), /Admin.*Teilnahme: Kleinesonne in Raid #\s*7.*Korrigiert.*Rolle korrigiert/)
  assert.match(text(view.tree), /Rolle.*Schaden.*Heiler/)
  assert.match(text(view.tree), /"role": "HEAL"/)
  input(view, 'Akteur, Aktion, Objekt oder Grund').props.onChange(change('healer'))
  view.render()
  assert.match(text(view.tree), /Keine Einträge für diese Suche/)

  const member = setup({ id: 2, name: 'Mitglied', role: 'MEMBER' })
  const memberView = member.mount(member.load('pages/AuditLog.tsx').AuditLogPage)
  assert.equal(member.requests.length, 0)
  assert.match(text(memberView.tree), /nur für Admins sichtbar/)
})

test('audit log describes roster additions, wishlist, polls, locks and account changes in German', async () => {
  const admin = setup({ id: 1, name: 'Admin', role: 'ADMIN' })
  const view = admin.mount(admin.load('pages/AuditLog.tsx').AuditLogPage)
  const entry = (id, entity_type, action, before, after, actor_name = 'Admin') => ({ id, actor_id: 1, actor_name, entity_type, entity_id: String(id), action, before_json: before === null ? 'null' : JSON.stringify(before), after_json: after === null ? 'null' : JSON.stringify(after), reason: '', created_at: '2026-10-04T18:00:00Z' })
  admin.requests[0].resolve([
    entry(1, 'attendance', 'correction', null, { raid_id: 7, character_name: 'Kleinesonne', role: 'DD' }),
    entry(2, 'wishlist_item', 'create', null, { character_id: 5, item_name: 'Schattenschneide', priority: 1, is_enhanced: true }, 'Spieler'),
    entry(3, 'wishlist_item', 'delete', { character_id: 5, item_name: 'Schattenschneide' }, null, 'Spieler'),
    entry(4, 'poll', 'vote', null, { title: 'Raidtag', choices: ['Mittwoch'] }, 'Spieler'),
    entry(5, 'item_lock', 'cancel', { raid_id: 7, item_name: 'Schattenschneide', status: 'ACTIVE' }, { raid_id: 7, item_name: 'Schattenschneide', status: 'CANCELLED' }, 'Spieler'),
    entry(6, 'raid', 'create', null, { name: 'ICC25-12', status: 'PLANNED', scheduled_start: '2026-10-08T18:30:00Z' }),
    entry(7, 'player', 'password_change', null, { name: 'Spieler' }, 'Spieler'),
    entry(8, 'guild_rank', 'update', { name: 'Raider', sort_order: 1 }, { name: 'Raider', sort_order: 2 }),
  ])
  await flush(view)
  const flat = text(view.tree)
  assert.match(flat, /Teilnahme: Kleinesonne in Raid #\s*7.*Hinzugefügt/)
  assert.match(flat, /Wunschliste: Schattenschneide.*Auf die Wunschliste gesetzt/)
  assert.match(flat, /Von der Wunschliste entfernt/)
  assert.match(flat, /Umfrage: Raidtag.*Abgestimmt/)
  assert.match(flat, /Item-Lock: Schattenschneide in Raid #\s*7.*Aufgehoben/)
  assert.match(flat, /Status.*Aktiv.*Abgesagt/)
  assert.match(flat, /Raid: ICC25-12.*Angelegt/)
  assert.match(flat, /Geplanter Start.*8\.10\.2026/)
  assert.match(flat, /Spieler: Spieler.*Passwort geändert/)
  assert.match(flat, /Gildenrang: Raider.*Aktualisiert.*Reihenfolge/)
  const links = nodes(view.tree, (n) => n.props && typeof n.props.to === 'string').map((n) => n.props.to)
  assert.ok(links.includes('/characters/5'), 'wishlist entries link to the character')
  assert.ok(links.includes('/polls'))
  assert.ok(links.includes('/roster'))
})

test('MFA reminder shows only for signed-in players without a second factor (ADR 092)', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { MFAReminderBanner } = s.load('components/MFAReminderBanner.tsx')
  const view = s.mount(MFAReminderBanner, { isSignedIn: true })
  assert.equal(view.tree, null, 'nothing while loading')
  const lookup = s.requests.find((r) => r.url === '/api/v1/auth/mfa')
  assert.ok(lookup)
  lookup.resolve({ totp_enabled: false, recovery_codes: true, security_keys: [] })
  await flush(view)
  assert.match(text(view.tree), /nur mit einem Passwort geschützt/)
  const link = nodes(view.tree, (n) => n.type === 'a')[0]
  assert.equal(link.props.to, '/kontosicherheit')

  for (const status of [{ totp_enabled: true, security_keys: [] }, { totp_enabled: false, security_keys: [{ id: 1, name: 'Key', created_at: '2026-01-01T00:00:00Z' }] }]) {
    const other = setup({ id: 3, role: 'MEMBER' })
    const protectedView = other.mount(other.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: true })
    other.requests[0].resolve(status)
    await flush(protectedView)
    assert.equal(protectedView.tree, null)
  }

  const failing = setup({ id: 4, role: 'MEMBER' })
  const failedView = failing.mount(failing.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: true })
  failing.requests[0].reject(new Error('offline'))
  await flush(failedView)
  assert.equal(failedView.tree, null, 'a failed lookup shows no warning')

  const anonymous = setup(null)
  const anonymousView = anonymous.mount(anonymous.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: false })
  await flush(anonymousView)
  assert.equal(anonymousView.tree, null)
  assert.equal(anonymous.requests.length, 0, 'visitors cause no MFA lookup')
})

test('MFA reminder can be hidden and follows MFA changes from the profile (ADR 092)', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { announceMFAChange } = s.load('accountSecurity.ts')
  const view = s.mount(s.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: true })
  s.requests[0].resolve({ totp_enabled: false, security_keys: [] })
  await flush(view)
  assert.ok(view.tree)

  announceMFAChange()
  const refreshed = s.requests.filter((r) => r.url === '/api/v1/auth/mfa')
  assert.equal(refreshed.length, 2)
  refreshed[1].resolve({ totp_enabled: true, security_keys: [] })
  await flush(view)
  assert.equal(view.tree, null, 'disappears after a factor is set up')

  const hide = setup({ id: 5, role: 'MEMBER' })
  const hideView = hide.mount(hide.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: true })
  hide.requests[0].resolve({ totp_enabled: false, security_keys: [] })
  await flush(hideView)
  nodes(hideView.tree, (n) => n.type === 'button' && n.props['aria-label'] === 'Hinweis für diese Sitzung ausblenden')[0].props.onClick()
  hideView.render()
  assert.equal(hideView.tree, null)
})

test('an Admin without a second factor must set one up: fixed notice and account tab (REQ-AUTH-020, FR-3.10)', async () => {
  const s = setup({ id: 7, role: 'ADMIN' })
  const { MFAReminderBanner } = s.load('components/MFAReminderBanner.tsx')
  const view = s.mount(MFAReminderBanner, { isSignedIn: true, adminID: 7, path: '/' })
  s.requests.find((r) => r.url === '/api/v1/auth/mfa').resolve({ totp_enabled: false, recovery_codes: true, security_keys: [] })
  await flush(view)
  assert.equal(view.tree.props.role, 'alert')
  assert.match(text(view.tree), /Als Admin brauchst du einen zweiten Faktor/)
  assert.equal(nodes(view.tree, (n) => n.type === 'a')[0].props.to, '/players/7/konto')
  assert.equal(nodes(view.tree, (n) => n.type === 'button').length, 0, 'the Admin notice cannot be hidden')
  assert.deepEqual(s.navigations, ['/players/7/konto'], 'the first page after login is the account tab')
  view.render()
  assert.deepEqual(s.navigations, ['/players/7/konto'], 'redirects only once')

  const protectedAdmin = setup({ id: 8, role: 'ADMIN' })
  const protectedView = protectedAdmin.mount(protectedAdmin.load('components/MFAReminderBanner.tsx').MFAReminderBanner, { isSignedIn: true, adminID: 8, path: '/' })
  protectedAdmin.requests[0].resolve({ totp_enabled: true, security_keys: [] })
  await flush(protectedView)
  assert.equal(protectedView.tree, null)
  assert.deepEqual(protectedAdmin.navigations, [])
})

test('the mfa_setup_required error is shown in German (REQ-AUTH-020)', async () => {
  const realFetch = globalThis.fetch
  globalThis.fetch = async () => ({ ok: false, status: 403, statusText: 'Forbidden', headers: { get: () => 'application/json' }, json: async () => ({ error: { code: 'mfa_setup_required', message: 'admin requires a second factor' } }), text: async () => '{"error":{"code":"mfa_setup_required","message":"admin requires a second factor"}}' })
  try {
    const s = setup({ id: 7, role: 'ADMIN' })
    const real = s.load('api.ts')
    await assert.rejects(real.api.get('/api/v1/audit-log'), (err) => err.code === 'mfa_setup_required' && /zweiten Faktor/.test(err.message))
  } finally {
    globalThis.fetch = realFetch
  }
})

test('the last_admin error is shown in German (REQ-ACCOUNT-015)', async () => {
  const realFetch = globalThis.fetch
  globalThis.fetch = async () => ({ ok: false, status: 409, statusText: 'Conflict', headers: { get: () => 'application/json' }, json: async () => ({ error: { code: 'last_admin', message: 'at least one active admin must remain' } }), text: async () => '{"error":{"code":"last_admin","message":"at least one active admin must remain"}}' })
  try {
    const s = setup({ id: 7, role: 'ADMIN' })
    const real = s.load('api.ts')
    await assert.rejects(real.api.get('/api/v1/players'), (err) => err.code === 'last_admin' && /mindestens ein aktiver Admin/.test(err.message))
  } finally {
    globalThis.fetch = realFetch
  }
})

test('the account-security deep link opens the profile account tab (ADR 092)', async () => {
  const s = setup({ id: 2, role: 'MEMBER' })
  const { PlayerDetailPage } = s.load('pages/PlayerDetail.tsx')
  const view = s.mount(PlayerDetailPage, { playerID: 2, initialView: 'account' })
  resolvePlayer(s, 2)
  await flush(view)
  const account = nodes(view.tree, (n) => n.type === 'section' && n.props.id === 'profile-account')[0]
  assert.ok(account)
  assert.equal(account.props.hidden, false)
  const plain = s.mount(PlayerDetailPage, { playerID: 2 })
  resolvePlayer(s, 2)
  await flush(plain)
  assert.equal(nodes(plain.tree, (n) => n.type === 'section' && n.props.id === 'profile-account')[0].props.hidden, true)
})

test('loot that went to nobody: listed for everyone, staff record more (ADR 089)', async () => {
  const s = setup()
  const bosses = [{ id: '36612', instance_id: '631', name: 'Lord Mark’gar' }]
  const view = s.mount(s.load('components/UnclaimedLoot.tsx').UnclaimedLoot, { raidID: 1, canWrite: true, needsReason: false, bosses, version: 0 })
  assert.equal(resolveIf(s, '/api/v1/raids/1/unclaimed-loot', [{ id: 1, boss: 'Saurfang', item_id: '50667', item_name: 'Gürtelbund des gerechten Zorns', outcome: 'DISENCHANTED', holder_name: 'Shanna', notes: '', source: 'addon' }]), true)
  await flush(view)
  assert.match(text(view.tree), /Gürtelbund des gerechten Zorns/)
  assert.match(text(view.tree), /Entzaubert/)
  assert.match(text(view.tree), /\(Addon\)/)
  input(view, 'Name des Gegenstands').props.onChange(change('Bryntroll'))
  view.render()
  nodes(view.tree, (n) => n.type === 'form')[0].props.onSubmit({ preventDefault() {} })
  assert.equal(writes(s)[0].url, '/api/v1/raids/1/unclaimed-loot')
  assert.deepEqual(writes(s)[0].body, { catalog_boss_id: '', item_name: 'Bryntroll', outcome: 'NOBODY', holder_name: '', notes: '', reason: '' })

  const reader = setup({ id: 2, role: 'MEMBER' })
  const empty = reader.mount(reader.load('components/UnclaimedLoot.tsx').UnclaimedLoot, { raidID: 1, canWrite: false, needsReason: false, bosses: [], version: 0 })
  assert.equal(resolveIf(reader, '/api/v1/raids/1/unclaimed-loot', []), true)
  await flush(empty)
  assert.doesNotMatch(text(empty.tree) || '', /Beute ohne Empfänger/)
})

test('staff see the signup list with class emblems first and their own signup folded away', async () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const view = s.mount(s.load('components/RaidSignups.tsx').RaidSignups, {
    raid: { ...raid(8), scheduled_start: '2099-01-01T20:00:00Z' },
    attendance: [{ id: 44, player_id: 3, character_id: 30, character_name: 'Direkt', class: 'Priester', spec: 'Heilig', role: 'HEAL' }],
    players: [{ id: 1, name: 'Leitung', active: true }, { id: 2, name: 'Neu', active: true }, { id: 3, name: 'Gesetzt', active: true }], onRosterChange: async () => {},
  })
  resolveIf(s, '/api/v1/raids/8/signups', { is_enabled: true, is_open: true, class_specs: {}, absences: [], entries: [
    { id: 4, player_id: 2, character_id: 20, character_name: 'Funkenflug', class: 'Magier', spec: 'Feuer', attendance_id: 0, is_accepted: false, is_withdrawn: false },
  ] })
  resolveIf(s, '/api/v1/players/1/characters', [])
  await flush(view)
  const flat = text(view.tree)
  assert.match(flat, /Anmeldeliste.*Neu.*Ausstehend.*Funkenflug.*Magier\s+·\s+Feuer.*Gesetzt.*In der Aufstellung.*Direkt.*Priester\s+·\s+Heilig.*Von der Raidleitung eingetragen.*Eigene Anmeldung/s)
  assert.deepEqual(nodes(view.tree, (n) => n.type?.name === 'ClassEmblem').map((n) => n.props.characterClass), ['Magier', 'Priester'])
  assert.equal(nodes(view.tree, (n) => n.type === 'details').length, 1, 'own signup is folded for staff')
  button(view, 'In Aufstellung übernehmen').props.onClick()
  assert.equal(writes(s)[0].url, '/api/v1/raids/8/signups/4/accept')
})

test('poll editor keeps option ids so renamed options keep their votes, and can reopen a poll', () => {
  const s = setup({ id: 1, role: 'RAIDASSISTENT' })
  const saved = []
  const poll = { id: 5, title: 'Raidtag', description: '', poll_type: 'CHOICE', allow_multiple: false, status: 'CLOSED', closes_at: null, options: [{ id: 11, label: 'Mittwoch' }, { id: 12, label: 'Donnerstag' }] }
  const view = s.mount(s.load('components/PollEditor.tsx').PollEditor, { poll, pending: false, onSave: (body) => saved.push(body), onCancel: () => {} })
  nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Antwort 2')[0].props.onChange(change('Donnerstag 20 Uhr'))
  view.render()
  nodes(view.tree, (n) => n.type === 'button' && n.props['aria-label'] === 'Antwort 2 nach oben')[0].props.onClick()
  view.render()
  button(view, 'Antwort hinzufügen').props.onClick()
  view.render()
  nodes(view.tree, (n) => n.type === 'input' && n.props['aria-label'] === 'Antwort 3')[0].props.onChange(change('Samstag'))
  nodes(view.tree, (n) => n.type === 'select')[0].props.onChange(change('OPEN'))
  view.render()
  form(view).props.onSubmit(event)
  assert.deepEqual(saved, [{ title: 'Raidtag', description: '', status: 'OPEN', closes_at: '', options: [{ id: 12, label: 'Donnerstag 20 Uhr' }, { id: 11, label: 'Mittwoch' }, { id: 0, label: 'Samstag' }] }])
})

test('install hint picks the browser prompt, iPhone steps, or nothing (FR-28.5)', () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const { installHint } = s.load('installHint.ts')
  const base = { isStandalone: false, isDismissed: false, hasPrompt: false, isIOS: false }
  assert.equal(installHint(base), 'none', 'no prompt and no iPhone: nothing to offer')
  assert.equal(installHint({ ...base, hasPrompt: true }), 'prompt')
  assert.equal(installHint({ ...base, isIOS: true }), 'ios')
  assert.equal(installHint({ ...base, hasPrompt: true, isStandalone: true }), 'none', 'already installed')
  assert.equal(installHint({ ...base, isIOS: true, isDismissed: true }), 'none', 'hidden for this session')
})

test('install hint shows the iPhone steps and can be hidden (FR-28.5)', () => {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  Object.defineProperty(globalThis, 'navigator', { configurable: true, writable: true, value: { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)', maxTouchPoints: 5 } })
  try {
    const s = setup({ id: 1, role: 'MEMBER' })
    const view = s.mount(s.load('components/PWAInstall.tsx').PWAInstall)
    view.render()
    assert.match(text(view.tree), /Zum Home-Bildschirm/)
    assert.equal(nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Installieren').length, 0, 'iPhone has no install button')
    nodes(view.tree, (n) => n.type === 'button' && n.props['aria-label'] === 'Hinweis für diese Sitzung ausblenden')[0].props.onClick()
    view.render()
    assert.equal(view.tree, null)
  } finally {
    if (previous) Object.defineProperty(globalThis, 'navigator', previous)
    else delete globalThis.navigator
  }
})

test('the service worker caches nothing and handles no requests (FR-28.5, SC-052)', () => {
  const worker = fs.readFileSync(path.resolve(__dirname, '../public/sw.js'), 'utf8')
  assert.doesNotMatch(worker, /addEventListener\(\s*['"]fetch['"]/)
  assert.doesNotMatch(worker, /caches\.|CacheStorage|importScripts/)
  const manifest = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../public/manifest.webmanifest'), 'utf8'))
  for (const size of ['192x192', '512x512']) {
    const icon = manifest.icons.find((entry) => entry.sizes === size && entry.type === 'image/png')
    assert.ok(icon && fs.existsSync(path.resolve(__dirname, '../public', icon.src.slice(1))), `PNG icon ${size}`)
  }
})

function runServiceWorker() {
  const listeners = {}
  const shown = []
  const opened = []
  const self = {
    location: { origin: 'https://raid.example' },
    addEventListener: (type, fn) => { listeners[type] = fn },
    skipWaiting: () => {},
    registration: { showNotification: (title, options) => { shown.push({ title, options }); return Promise.resolve() } },
    clients: { claim: () => Promise.resolve(), matchAll: () => Promise.resolve([]), openWindow: (url) => { opened.push(url); return Promise.resolve() } },
  }
  vm.runInNewContext(fs.readFileSync(path.resolve(__dirname, '../public/sw.js'), 'utf8'), { self, URL })
  const fire = (type, event) => {
    let pending = Promise.resolve()
    listeners[type]({ ...event, waitUntil: (p) => { pending = p } })
    return pending
  }
  return { listeners, shown, opened, fire }
}

test('a push shows only a fixed German text for its kind (FR-32, SC-053)', async () => {
  const sw = runServiceWorker()
  const data = (value) => ({ json: () => JSON.parse(value) })
  await sw.fire('push', { data: data('{"kind":"poll","title":"Geheimer Titel"}') })
  await sw.fire('push', { data: data('{"kind":"raid"}') })
  await sw.fire('push', { data: data('{"kind":"roster"}') })
  await sw.fire('push', { data: data('{"kind":"<b>unbekannt</b>"}') })
  await sw.fire('push', { data: { json: () => { throw new Error('not json') } } })
  await sw.fire('push', { data: null })
  assert.deepEqual(sw.shown.map((n) => n.options.body), [
    'Es gibt eine neue Umfrage.',
    'Es wurde ein neuer Raid geplant.',
    'Du stehst in einer Raid-Aufstellung.',
    'Es gibt Neuigkeiten in RAID.',
    'Es gibt Neuigkeiten in RAID.',
    'Es gibt Neuigkeiten in RAID.',
  ])
  assert.ok(sw.shown.every((n) => n.title === 'RAID' && !JSON.stringify(n).includes('Geheim') && !JSON.stringify(n).includes('<b>')))
  await sw.fire('notificationclick', { notification: { close: () => {}, data: sw.shown[0].options.data } })
  assert.deepEqual(sw.opened, ['https://raid.example/#/polls'])
})

test('push opt-in states, including iPhone without installation (FR-32)', () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const { pushAvailability, base64URLToBytes, subscriptionBody } = s.load('pushNotifications.ts')
  const base = { hasPushAPI: true, isIOS: false, isStandalone: false, permission: 'default', serverEnabled: true }
  assert.equal(pushAvailability(base), 'available')
  assert.equal(pushAvailability({ ...base, serverEnabled: false }), 'server-off')
  assert.equal(pushAvailability({ ...base, isIOS: true }), 'ios-install')
  assert.equal(pushAvailability({ ...base, isIOS: true, isStandalone: true }), 'available')
  assert.equal(pushAvailability({ ...base, hasPushAPI: false, permission: 'unsupported' }), 'unsupported')
  assert.equal(pushAvailability({ ...base, permission: 'denied' }), 'denied')
  assert.deepEqual(Array.from(base64URLToBytes('-_8')), [0xfb, 0xff])
  assert.deepEqual(subscriptionBody({ endpoint: 'https://fcm.googleapis.com/x', expirationTime: null, keys: { p256dh: 'p', auth: 'a' } }), { endpoint: 'https://fcm.googleapis.com/x', p256dh: 'p', auth: 'a' })
})

test("a visit keeps the own push subscription and drops another player's (FR-32.3, ADR 103)", () => {
  const s = setup({ id: 1, role: 'MEMBER' })
  const { pushSyncAction } = s.load('pushNotifications.ts')
  const base = { hasSubscription: true, owner: '7', playerID: 7, permission: 'granted' }
  assert.equal(pushSyncAction(base), 'resubscribe')
  assert.equal(pushSyncAction({ ...base, owner: null }), 'resubscribe')
  assert.equal(pushSyncAction({ ...base, owner: '8' }), 'drop')
  assert.equal(pushSyncAction({ ...base, owner: '8', permission: 'default' }), 'drop')
  assert.equal(pushSyncAction({ ...base, permission: 'default' }), 'none')
  assert.equal(pushSyncAction({ ...base, hasSubscription: false }), 'none')
  const auth = fs.readFileSync(path.resolve(__dirname, '..', 'src/auth.tsx'), 'utf8')
  assert.match(auth, /if \(playerID !== undefined && browserHasPushAPI\(\)\) void syncPushForPlayer\(playerID\)/)
})

test('the browser is asked for notification permission only after a click (FR-32)', () => {
  const sources = ['src/pushNotifications.ts', 'src/components/PushNotificationSettings.tsx', 'src/auth.tsx', 'src/App.tsx', 'src/components/PWAInstall.tsx']
    .map((file) => fs.readFileSync(path.resolve(__dirname, '..', file), 'utf8'))
  const calls = sources.join('\n').match(/requestPermission\(/g) ?? []
  assert.equal(calls.length, 1)
  assert.match(sources[0], /export async function enablePush[\s\S]*requestPermission\(/)
  assert.match(sources[2], /if \(browserHasPushAPI\(\)\) await disablePushOnLogout\(\)\s*\n\s*try \{\s*\n\s*await api\.post<void>\('\/api\/v1\/auth\/logout'\)/)
})

const statisticsFixture = {
  raids_by_status: [{ status: 'PLANNED', count: 1 }, { status: 'OPEN', count: 0 }, { status: 'COMPLETED', count: 3 }, { status: 'CANCELLED', count: 0 }],
  class_distribution: [{ class: 'Magier', count: 2 }],
  role_history: [{ role: 'TANK', count: 4 }],
  loot_by_class: [{ class: 'Magier', count: 5 }],
  top_attendance: [{ player_id: 3, player_name: 'Anna', count: 3 }],
  top_loot: [{ player_id: 3, player_name: 'Anna', count: 5 }],
}

test('TestReqStats011_AC01 statistics render from the endpoint without a session', async () => {
  const s = setup(null)
  const view = s.mount(s.load('pages/Statistics.tsx').StatisticsPage)
  s.requests[0].resolve(statisticsFixture)
  await flush(view)
  const cards = nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'StatCard').map((n) => [n.props.label, n.props.value])
  assert.deepEqual(cards, [['Raids', 4], ['Archivierte Raids', 3], ['Aktive Charaktere', 2], ['Loot-Einträge', 5]])
  const tables = nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'LeaderTable')
  assert.deepEqual(tables.map((n) => n.props.rows[0].player_name), ['Anna', 'Anna'])
  const app = fs.readFileSync(path.resolve(__dirname, '../src/App.tsx'), 'utf8')
  assert.match(app, /path !== '\/addons' && !path\.startsWith\('\/addons\/'\) && path !== '\/statistics'/)
})

test('TestReqStats011_AC02 the page calls only the statistics endpoint', async () => {
  const s = setup(null)
  const view = s.mount(s.load('pages/Statistics.tsx').StatisticsPage)
  s.requests[0].resolve(statisticsFixture)
  await flush(view)
  assert.deepEqual(s.requests.map((r) => `${r.method} ${r.url}`), ['get /api/v1/statistics'])
})

test('TestReqStats011_AC03 a failing endpoint shows the shared error component', async () => {
  const s = setup(null)
  const view = s.mount(s.load('pages/Statistics.tsx').StatisticsPage)
  s.requests[0].reject(new Error('down'))
  await flush(view)
  const errors = nodes(view.tree, (n) => typeof n.type === 'function' && n.type.name === 'ErrorMessage')
  assert.equal(errors.length, 1)
  assert.match(text(errors[0]), /Statistiken konnten nicht geladen werden/)
})
