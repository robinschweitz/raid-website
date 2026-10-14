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
  const api = Object.fromEntries(['get', 'post', 'postEnvelope', 'put', 'patch'].map((method) => [method, (url, body) => {
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
    vm.runInThisContext(`(function(require,module,exports,window,crypto){${source}\n})`, { filename })(localRequire, module, module.exports, { prompt: () => prompts.shift(), setInterval: () => 0, clearInterval: () => {} }, crypto)
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

  return { load, mount, requests, api, auth, navigations, prompts }
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
const select = (view, label) => nodes(view.tree, (n) => n.type === 'select' && n.props['aria-label'] === label)[0]
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
  const option = nodes(select(view, 'Loot-Item'), (node) => node.type === 'option' && node.props.value)[0]
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
      : r.url === '/api/v1/loot-catalog' ? options.catalog ?? emptyCatalog() : [])
  }
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

test('character search preserves selected identity and displays the matching owner', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  select(view, 'Teilnahme-Charakter').props.onChange(change('10'))
  view.render()
  input(view, 'Charaktername oder Spieler').props.onChange(change('Spieler 2'))
  view.render()
  assert.equal(select(view, 'Teilnahme-Charakter').props.value, '10')
  assert.match(text(select(view, 'Teilnahme-Charakter')), /Char 20.*Spieler 2/)
  assert.doesNotMatch(text(select(view, 'Teilnahme-Charakter')), /Char 11/)
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

const report = (errors = []) => ({ mode: 'dry_run', raw_hash: 'preview-hash', created: { players: 2, characters: 3, raids: 4, attendance: 5, loot: 6, tokens: 7, points: 8 }, errors, warnings: [], rows_read: 10 })
test('import merge requires error-free preview of unchanged source and shows counts/errors', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  await button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).length, 0)
  const first = form(view).props.onSubmit(event)
  // The form handler intentionally starts work without returning its promise.
  await first
  form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 1)
  writes(s)[0].resolve(report(['invalid date']))
  await flush(view)
  assert.match(text(view.tree), /invalid date/)
  assert.match(text(view.tree), /Geplante neue Einträge/)
  assert.match(text(view.tree), /Spieler:\s+2/)
  assert.match(text(view.tree), /EP-Transaktionen \(einschließlich Korrekturen\):\s+8/)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  form(view).props.onSubmit(event)
  writes(s).at(-1).resolve(report())
  await flush(view)
  assert.equal(button(view, 'Zusammenführen').props.disabled, false)
  const sheet = nodes(view.tree, (n) => n.type === 'input')[0]
  const source = sheet.props.value
  sheet.props.onChange(change('different-source'))
  view.render()
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  assert.doesNotMatch(text(view.tree), /Geplante neue Einträge/)
  nodes(view.tree, (n) => n.type === 'input')[0].props.onChange(change(source))
  view.render()
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
})

test('import ignores preview response after source edit and guards merge duplicates', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  nodes(view.tree, (n) => n.type === 'input')[0].props.onChange(change('new-source'))
  writes(s)[0].resolve(report())
  await flush(view)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  assert.doesNotMatch(text(view.tree), /Geplante neue Einträge/)
  form(view).props.onSubmit(event)
  writes(s).at(-1).resolve(report())
  await flush(view)
  const merge = button(view, 'Zusammenführen').props.onClick
  const first = merge()
  await merge()
  assert.equal(writes(s).filter((r) => r.url === '/api/v1/imports').length, 1)
  assert.equal(writes(s).at(-1).body.sheet_id, 'new-source')
  assert.equal(writes(s).at(-1).body.raw_hash, 'preview-hash')
  writes(s).at(-1).reject(new Error('offline'))
  await first
  view.render()
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
})

test('import setup sends explicit owner settings and schedule edits invalidate preview and stale handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  const submitted = JSON.parse(JSON.stringify(writes(s)[0].body.setup))
  assert.match(submitted.first_date, /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/)
  const nextDate = new Date(submitted.first_date + 'T00:00:00Z')
  assert.equal(nextDate.getUTCDay(), 1, 'suggested ICC25 date must be a Monday')
  const todayParts = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Europe/Berlin', year: 'numeric', month: '2-digit', day: '2-digit',
  }).formatToParts(new Date())
  const berlinDate = (field) => Number(todayParts.find((p) => p.type === field)?.value)
  const today = new Date(Date.UTC(berlinDate('year'), berlinDate('month') - 1, berlinDate('day')))
  assert.ok(nextDate > today, 'suggested ICC25 date must be in the future')
  assert.deepEqual({ ...submitted, first_date: 'future Monday' }, {
    name: 'ICC25', leader: 'Oxora', first_date: 'future Monday', start_time: '19:00',
    timezone: 'Europe/Berlin', weekdays: [1], interval_weeks: 1,
  })
  writes(s)[0].resolve(report())
  await flush(view)
  const staleMerge = button(view, 'Zusammenführen').props.onClick
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'time')[0].props.onChange(change('20:00'))
  await staleMerge()
  view.render()
  assert.equal(writes(s).length, 1)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  form(view).props.onSubmit(event)
  assert.equal(writes(s).at(-1).body.setup.start_time, '20:00')
  nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'checkbox')[0].props.onChange({ target: { checked: false } })
  writes(s).at(-1).resolve(report())
  await flush(view)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  form(view).props.onSubmit(event)
  assert.equal('setup' in writes(s).at(-1).body, false)
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

test('auth gate shows retry instead of login when session lookup fails', () => {
  const s = setup(null)
  s.auth.error = 'Sitzung konnte nicht geladen werden.'
  s.auth.loading = false
  const app = s.load('App.tsx').default()
  const gate = s.mount(app.props.children.type)
  assert.ok(button(gate, 'Erneut versuchen'))
  assert.match(text(gate.tree), /Sitzung konnte nicht geladen/)
  s.auth.error = ''
  gate.render()
  assert.equal(gate.tree.type.name, 'LoginPage')
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
  assert.equal(select(view, 'Teilnahme-Charakter').props.value, '11')
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
  assert.equal(select(view, 'Loot-Empfänger').props.value, '11')
  assert.equal(input(view, 'Loot-Notizen').props.value, 'old loot note')
  await form(view, 1).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  await chooseSourceDrop(s, view)
  select(view, 'Loot-Empfänger').props.onChange(change('20'))
  select(view, 'Loot-Vergabemethode').props.onChange(change('SECOND'))
  input(view, 'Loot-Notizen').props.onChange(change('new loot note'))
  input(view, 'Loot-Begründung').props.onChange(change(' wrong recipient '))
  view.render()
  assert.equal(select(view, 'Loot-Empfänger').props.value, '20')
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

test('import hash is mandatory and stale merge handlers cannot bypass source edits', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  writes(s)[0].resolve({ ...report(), raw_hash: '' })
  await flush(view)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  await button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).length, 1)
  form(view).props.onSubmit(event)
  writes(s).at(-1).resolve({ ...report(), raw_hash: 'latest-preview-hash' })
  await flush(view)
  const oldMerge = button(view, 'Zusammenführen').props.onClick
  nodes(view.tree, (n) => n.type === 'input')[0].props.onChange(change('changed-sheet'))
  await oldMerge()
  assert.equal(writes(s).filter((r) => r.url === '/api/v1/imports').length, 0)
})

test('source_changed clears merge permission and next preview provides the new hash', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  assert.equal('raw_hash' in writes(s)[0].body, false)
  writes(s)[0].resolve(report())
  await flush(view)
  const first = button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).at(-1).body.raw_hash, 'preview-hash')
  const { ApiError } = s.load('api.ts')
  writes(s).at(-1).reject(new ApiError(409, 'source_changed', 'source changed'))
  await first
  view.render()
  assert.match(text(view.tree), /neue Vorschau/)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  form(view).props.onSubmit(event)
  writes(s).at(-1).resolve({ ...report(), raw_hash: 'new-hash' })
  await flush(view)
  const next = button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).at(-1).body.raw_hash, 'new-hash')
  const failedReport = { ...report(['conflicting row']), mode: 'merge', created: { players: 0, characters: 0, raids: 0, attendance: 0, loot: 0, tokens: 0, points: 0 } }
  writes(s).at(-1).resolve(failedReport)
  await next
  view.render()
  assert.match(text(view.tree), /conflicting row/)
  assert.match(text(view.tree), /keine Einträge geschrieben/)
  assert.match(text(view.tree), /Import ID/)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
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

const historicalPreview = () => ({ ...report(), warnings: ['raid "ICC25-01": reason_required for audited additions to existing imported raid'] })

test('historical preview remains valid but merge requires trimmed nonblank reason', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  assert.equal('reason' in writes(s)[0].body, false)
  writes(s)[0].resolve(historicalPreview())
  await flush(view)
  assert.match(text(view.tree), /Die Vorschau ist gültig/)
  assert.doesNotMatch(text(view.tree), /Der Bericht enthält Fehler/)
  assert.equal(input(view, 'Import-Begründung').props['aria-required'], true)
  assert.equal(input(view, 'Import-Begründung').props.required, undefined)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  await button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).length, 1)
  input(view, 'Import-Begründung').props.onChange(change(' \t '))
  view.render()
  await button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).length, 1)
  input(view, 'Import-Begründung').props.onChange(change('  fehlendes historisches Loot  '))
  view.render()
  assert.equal(button(view, 'Zusammenführen').props.disabled, false)
  const merge = button(view, 'Zusammenführen').props.onClick
  const first = merge()
  await merge()
  assert.equal(writes(s).length, 2)
  const write = writes(s).at(-1)
  assert.equal(write.body.reason, 'fehlendes historisches Loot')
  assert.equal(write.body.raw_hash, 'preview-hash')
  assert.equal(write.body.sheet_id, nodes(view.tree, (n) => n.type === 'input')[0].props.value)
  view.render()
  assert.equal(input(view, 'Import-Begründung').props.disabled, true)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  write.reject(new Error('offline'))
  await first
  view.render()
  assert.equal(input(view, 'Import-Begründung').props.disabled, false)
  assert.equal(input(view, 'Import-Begründung').props.value, '  fehlendes historisches Loot  ')
})

test('initial import permits blank reason and unrelated preview warnings', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  input(view, 'Import-Begründung').props.onChange(change('  '))
  view.render()
  form(view).props.onSubmit(event)
  writes(s)[0].resolve({ ...report(), warnings: ['optional column missing'] })
  await flush(view)
  assert.equal(input(view, 'Import-Begründung').props['aria-required'], false)
  assert.equal(button(view, 'Zusammenführen').props.disabled, false)
  const first = button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).at(-1).body.reason, '')
  assert.equal(writes(s).at(-1).body.raw_hash, 'preview-hash')
  writes(s).at(-1).resolve({ ...report(), mode: 'merge' })
  await first
  view.render()
  assert.match(text(view.tree), /EP-Transaktionen \(einschließlich Korrekturen\):\s+8/)
})

test('historical reason draft survives source change without reusing old preview or handler', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  input(view, 'Import-Begründung').props.onChange(change('  historical additions  '))
  view.render()
  form(view).props.onSubmit(event)
  writes(s)[0].resolve(historicalPreview())
  await flush(view)
  const oldMerge = button(view, 'Zusammenführen').props.onClick
  nodes(view.tree, (n) => n.type === 'input')[0].props.onChange(change('new-sheet'))
  await oldMerge()
  assert.equal(writes(s).length, 1)
  view.render()
  assert.equal(input(view, 'Import-Begründung').props.value, '  historical additions  ')
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  form(view).props.onSubmit(event)
  writes(s).at(-1).resolve({ ...historicalPreview(), raw_hash: 'new-sheet-hash' })
  await flush(view)
  const first = button(view, 'Zusammenführen').props.onClick()
  const write = writes(s).at(-1)
  assert.equal(write.body.sheet_id, 'new-sheet')
  assert.equal(write.body.raw_hash, 'new-sheet-hash')
  assert.equal(write.body.reason, 'historical additions')
  // Even an obsolete response arriving after a source change cannot render a report.
  nodes(view.tree, (n) => n.type === 'input')[0].props.onChange(change('third-sheet'))
  write.resolve({ ...report(), mode: 'merge' })
  await first
  view.render()
  assert.doesNotMatch(text(view.tree), /Neu angelegte Einträge/)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
  assert.equal(input(view, 'Import-Begründung').props.value, '  historical additions  ')
})

test('reason warning does not prevent a new preview with a blank reason', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  writes(s)[0].resolve(historicalPreview())
  await flush(view)
  assert.equal(input(view, 'Import-Begründung').props.required, undefined)
  form(view).props.onSubmit(event)
  assert.equal(writes(s).length, 2)
  assert.equal(writes(s)[1].url, '/api/v1/imports/dry-run')
  assert.equal('reason' in writes(s)[1].body, false)
  writes(s)[1].resolve(historicalPreview())
  await flush(view)
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
})

test('missing historical reason merge report shows no writes and requires a fresh preview', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  form(view).props.onSubmit(event)
  writes(s)[0].resolve(report())
  await flush(view)
  const first = button(view, 'Zusammenführen').props.onClick()
  assert.equal(writes(s).at(-1).body.reason, '')
  writes(s).at(-1).resolve({
    ...historicalPreview(), mode: 'merge', errors: ['reason_required: additions to existing imported raids require an explicit correction reason'],
    created: { players: 0, characters: 0, raids: 0, attendance: 0, loot: 0, tokens: 0, points: 0 },
  })
  await first
  view.render()
  assert.match(text(view.tree), /keine Einträge geschrieben/)
  assert.match(text(view.tree), /Bitte eine Begründung eingeben/)
  assert.match(text(view.tree), /EP-Transaktionen \(einschließlich Korrekturen\):\s+0/)
  assert.doesNotMatch(text(view.tree), /Die Vorschau ist gültig/)
  input(view, 'Import-Begründung').props.onChange(change('now explained'))
  view.render()
  assert.equal(button(view, 'Zusammenführen').props.disabled, true)
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
  const first = button(view, 'Archivieren').props.onClick()
  await button(view, 'Archivieren').props.onClick()
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
  const archive = button(view, 'Archivieren').props.onClick()
  const { ApiError } = s.load('api.ts')
  writes(s)[1].reject(new ApiError(409, 'local_time_ambiguous', 'English backend message'))
  await archive
  view.render()
  assert.match(text(view.tree), /Zeitumstellung mehrdeutig/)
  assert.equal(button(view, 'Archivieren').props.disabled, false)
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
  assert.deepEqual(JSON.parse(JSON.stringify(writes(s)[0].body)), { name: 'Explicit name', raid_leader_id: 2, weekdays: [0], interval_weeks: 3, start_time: '19:45', timezone: 'Europe/Berlin', catalog_instance_id: 'map:631', raid_size: 25 })
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
  const first = button(view, 'Archivieren').props.onClick()
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
    const recipients = nodes(select(view, 'Loot-Empfänger'), (n) => n.type === 'option')
    assert.match(text(recipients), /Saved raid character/)
    assert.equal(recipients.some((n) => Number(n.props.value) === 99), false)
    assert.equal(recipients.some((n) => Number(n.props.value) === 1), false)
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
  assert.match(text(select(view, 'Loot-Item')), /Katalogitem.*25\s+Spieler.*Normal/)
  const variants = nodes(select(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)
  assert.equal(variants.length, 1)
  assert.equal(select(view, 'Loot-Modus').props.value, 'NORMAL')
  assert.deepEqual(nodes(select(view, 'Loot-Modus'), (node) => node.type === 'option').map((node) => node.props.value), ['NORMAL', 'HEROIC'])
  assert.doesNotMatch(text(select(view, 'Loot-Modus')), /source:|Alle|Lootmodus|Difficulty/)
  select(view, 'Loot-Modus').props.onChange(change('HEROIC'))
  view.render()
  assert.equal(nodes(select(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value).length, 1)
  select(view, 'Loot-Item').props.onChange(change(nodes(select(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0].props.value))
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
  const drop = nodes(select(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0]
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
  select(view, 'Loot-Item').props.onChange(change(nodes(select(view, 'Loot-Item'), (n) => n.type === 'option' && n.props.value)[0].props.value))
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
  assert.doesNotMatch(text(select(view, 'Loot-Item')), /Katalogitem/)
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
    assert.match(text(form(view, 1)), /Katalogitem/)
    assert.notEqual(select(view, 'Loot-Item').props.value, '')
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
  s.requests[0].resolve({ text: 'RS1|INV|1|Raid 1|25\nShanna|HEAL|Priester|0' })
  await flush(view)
  const exported = nodes(view.tree, (n) => n.type === 'textarea' && n.props['aria-label'] === 'Einladungs-Export')[0]
  assert.equal(exported.props.value, 'RS1|INV|1|Raid 1|25\nShanna|HEAL|Priester|0')
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

test('Sheet preflight exposes archive and EP targets before enabling synchronization', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  s.requests.find((r) => r.url === '/api/v1/imports').resolve([])
  s.requests.find((r) => r.url === '/api/v1/sheet-sync').resolve({ enabled: true, automatic: false })
  await flush(view)
  assert.equal(button(view, 'Jetzt synchronisieren').props.disabled, true)
  button(view, 'Vorabprüfung ohne Änderungen').props.onClick()
  const request = writes(s)[0]
  assert.equal(request.url, '/api/v1/sheet-sync/preflight')
  assert.equal(writes(s).filter((r) => r.url.endsWith('/run')).length, 0)
  request.resolve({ ready: true, blockers: [], import: {
    archive_transfers: [{ raid_id: 35, raid_name: 'ICC25-35', archived_at: '2026-10-11T22:00:00Z' }],
    point_transfers: [{ player: 'Hero', before: 0, target: 10, adjustment: 10 }, { player: 'Other', before: 170, target: 120, adjustment: -50 }],
    warnings: ['Skopin: vorhandene Website-EP bleiben unverändert.'],
  } })
  await flush(view)
  assert.match(text(view.tree), /Geplante Archivübernahme/)
  assert.match(text(view.tree), /ICC25-35/)
  assert.match(text(view.tree), /Hero\s+0\s+10\s+\+\s*10/)
  assert.match(text(view.tree), /Other\s+170\s+120\s+-50/)
  assert.match(text(view.tree), /Skopin: vorhandene Website-EP bleiben unverändert/)
  assert.equal(button(view, 'Jetzt synchronisieren').props.disabled, false)
})

test('Sheet preflight conflict blocks synchronization and displays the cause', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  s.requests.find((r) => r.url === '/api/v1/imports').resolve([])
  s.requests.find((r) => r.url === '/api/v1/sheet-sync').resolve({ enabled: true })
  await flush(view)
  button(view, 'Vorabprüfung ohne Änderungen').props.onClick()
  writes(s)[0].resolve({ ready: false, blockers: ['Hero: EP-Konflikt; Website und Sheet geändert'] })
  await flush(view)
  assert.match(text(view.tree), /Hero: EP-Konflikt/)
  assert.equal(button(view, 'Jetzt synchronisieren').props.disabled, true)
  assert.equal(writes(s).filter((r) => r.url.endsWith('/run')).length, 0)
})

test('Sheet sync Admin settings can be saved without exposing credentials', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/Imports.tsx').ImportsPage)
  s.requests.find((r) => r.url === '/api/v1/sheet-sync/config').resolve({
    enabled: false, automatic: false,
    sheet_id: '10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8',
    interval: '10m', reason: '', credentials_configured: false,
  })
  await flush(view)
  assert.ok(button(view, 'Einstellungen speichern'))
  const credential = '{"client_email":"private@example.com","private_key":"FAKE"}'
  const upload = nodes(view.tree, (n) => n.type === 'input' && n.props.type === 'file')[0]
  assert.ok(upload)
  upload.props.onChange({ currentTarget: {
    files: [{ name: 'service-account.json', size: credential.length, text: async () => credential }],
    value: 'service-account.json',
  } })
  await flush(view)
  assert.doesNotMatch(text(view.tree), /private@example.com/)
  assert.match(text(view.tree), /service-account.json/)
  const promise = form(view).props.onSubmit(event)
  const request = writes(s).at(-1)
  assert.equal(request.method, 'put')
  assert.equal(request.url, '/api/v1/sheet-sync/config')
  assert.equal(request.body.service_account_json, credential)
  request.resolve({
    enabled: false, automatic: false,
    sheet_id: '10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8',
    interval: '10m', reason: '', credentials_configured: true,
  })
  await promise
  await flush(view)
  assert.match(text(view.tree), /Servicekonto vorhanden/)
  assert.doesNotMatch(text(view.tree), /private@example.com/)
})
