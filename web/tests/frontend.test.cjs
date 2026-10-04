const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

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
  const api = Object.fromEntries(['get', 'post', 'put', 'patch'].map((method) => [method, (url, body) => {
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
      if (name.endsWith('/auth')) return { useAuth: () => auth }
      if (name.endsWith('/router')) return { Link: 'a', navigate: (url) => navigations.push(url) }
      const resolved = path.resolve(path.dirname(filename), name)
      const extension = fs.existsSync(`${resolved}.ts`) ? '.ts' : '.tsx'
      const exports = load(path.relative(path.resolve(__dirname, '../src'), resolved + extension))
      return name.endsWith('/api') ? { ...exports, api } : exports
    }
    vm.runInThisContext(`(function(require,module,exports,window){${source}\n})`, { filename })(localRequire, module, module.exports, { prompt: () => prompts.shift() })
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
      if (Component.name === 'RaidDetailPage' || Component.name === 'PlayerDetailPage') {
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
  return [...(predicate(tree) ? [tree] : []), ...nodes(tree.props?.children, predicate)]
}
function text(tree) {
  if (tree == null || typeof tree === 'boolean') return ''
  if (Array.isArray(tree)) return tree.map(text).join(' ')
  if (typeof tree !== 'object') return String(tree)
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
const player = (id, role = 'MEMBER') => ({ id, name: `Spieler ${id}`, role, username: `user${id}` })
const character = (id, playerID) => ({ id, player_id: playerID, name: `Char ${id}` })
const lock = (playerID) => ({ id: 9, player_id: playerID, status: 'ACTIVE' })

function resolveRaid(s, id, options = {}) {
  for (const r of s.requests.filter((r) => r.method === 'get' && !r.resolved)) {
    r.resolved = true
    r.resolve(r.url === `/api/v1/raids/${id}` ? { ...raid(id), ...options.raid }
      : r.url === '/api/v1/players' ? [player(1), player(2)]
      : r.url.endsWith('/attendance') ? options.attendance ?? []
      : r.url.endsWith('/loot') ? options.loot ?? [] : [])
  }
}
function resolvePlayer(s, id) {
  for (const r of s.requests.filter((r) => r.method === 'get' && !r.resolved)) {
    r.resolved = true
    if (r.url.endsWith('/characters')) r.resolve([character(id * 10, id), character(id * 10 + 1, id)])
    else if (r.url.endsWith('/points')) r.resolve({ balance: 200, transactions: [] })
    else if (r.url.endsWith('/tokens')) r.resolve({ total: 0, transactions: [] })
    else if (r.url.endsWith('/locks')) r.resolve([lock(id)])
    else r.resolve(player(id))
  }
}

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
  input(view, 'Item').props.onChange(change('old loot'))
  view.render()
  const oldSubmit = form(view, 1).props.onSubmit
  view.render({ raidID: 2 })
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 0)
  await oldSubmit(event)
  assert.equal(writes(s).length, 0)
  resolveRaid(s, 2)
  await flush(view)
  assert.match(text(view.tree), /Raid 2/)
  assert.equal(input(view, 'Item').props.value, '')
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

test('raid character selection clears synchronously and ignores obsolete success/failure', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  const selects = () => nodes(form(view), (n) => n.type === 'select')
  selects()[0].props.onChange(change('1'))
  view.render()
  const old = s.requests.at(-1)
  selects()[0].props.onChange(change('2'))
  view.render()
  const latest = s.requests.at(-1)
  assert.equal(selects()[1].props.value, '')
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  latest.resolve([character(20, 2)])
  await flush(view)
  old.resolve([character(10, 1)])
  await flush(view)
  assert.equal(selects()[1].props.value, '20')
  const submit = form(view).props.onSubmit
  const first = submit(event)
  await submit(event)
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].body.character_id, 20)
  writes(s)[0].reject(new Error('offline'))
  await first
  view.render()
  selects()[0].props.onChange(change('1'))
  view.render()
  const failure = s.requests.at(-1)
  selects()[0].props.onChange(change('2'))
  view.render()
  s.requests.at(-1).resolve([character(21, 2)])
  failure.reject(new Error('obsolete'))
  await flush(view)
  assert.equal(selects()[1].props.value, '21')
  assert.doesNotMatch(text(view.tree), /Charaktere laden fehlgeschlagen/)
})

test('old player roll cannot refresh or display a result under the new ID', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  input(view, 'Passwort (leer = unverändert)').props.onChange(change('secret'))
  const roll = button(view, 'Enhance würfeln').props.onClick()
  assert.equal(writes(s).length, 1)
  view.render({ playerID: 2 })
  resolvePlayer(s, 2)
  await flush(view)
  const count = s.requests.length
  writes(s)[0].resolve({ cost: 50, roll: 123, balance_after: 150 })
  await roll
  await flush(view)
  assert.equal(s.requests.length, count)
  assert.equal(input(view, 'Passwort (leer = unverändert)').props.value, '')
  assert.doesNotMatch(text(view.tree), /Kosten 50/)
  assert.equal(button(view, 'Enhance würfeln'), undefined)
})

test('character response is obsolete immediately on selection, before effect cleanup', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1)
  await flush(view)
  const select = () => nodes(form(view), (n) => n.type === 'select')[0]
  select().props.onChange(change('1'))
  view.render()
  const old = s.requests.at(-1)
  select().props.onChange(change('2'))
  old.resolve([character(10, 1)])
  await flush(view)
  assert.equal(nodes(form(view), (n) => n.type === 'select')[1].props.value, '')
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
})

for (const [label, formIndex] of [['Enhance würfeln', null], ['Lock anlegen', 1], ['Stornieren', null], ['Anlegen', 0], ['Speichern', 2]]) {
  test(`player mutation ${label} blocks same-tick duplicates`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    const invoke = formIndex === null ? button(view, label).props.onClick : () => form(view, formIndex).props.onSubmit(event)
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
  resolveRaid(s, 1)
  await flush(view)
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
  assert.equal(button(view, 'Vergeben').props.disabled, false)
})

test('player switch ignores late successful detail loads and old mutation handlers', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  const oldRoll = button(view, 'Enhance würfeln').props.onClick
  view.render({ playerID: 2 })
  const oldReads = s.requests.filter((r) => !r.resolved)
  view.render({ playerID: 3 })
  oldReads.forEach((r) => { r.resolved = true; r.resolve(r.url.endsWith('/2') ? player(2) : []) })
  await flush(view)
  assert.doesNotMatch(text(view.tree), /Spieler 2/)
  assert.equal(nodes(view.tree, (n) => n.type === 'form').length, 0)
  resolvePlayer(s, 3)
  await flush(view)
  await oldRoll()
  assert.equal(writes(s).length, 0)
  assert.match(text(view.tree), /Spieler 3/)
})

test('name history is latest-request-only and clears on entity switch', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  const historyButtons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Historie')
  const first = historyButtons[0].props.onClick()
  const old = s.requests.at(-1)
  const second = historyButtons[1].props.onClick()
  s.requests.at(-1).resolve([{ id: 2, old_name: 'new history', new_name: 'latest' }])
  await second
  old.resolve([{ id: 1, old_name: 'obsolete history', new_name: 'old' }])
  await first
  view.render()
  assert.match(text(view.tree), /new history/)
  assert.doesNotMatch(text(view.tree), /obsolete history/)
  view.render({ playerID: 2 })
  assert.doesNotMatch(text(view.tree), /new history/)
})

for (const [role, own, allowed] of [['GUEST', true, false], ['MEMBER', true, true], ['MEMBER', false, false], ['ADMIN', false, false], ['ADMIN', true, true]]) {
  test(`EP and lock controls: ${role}, own=${own}`, async () => {
    const s = setup({ id: own ? 1 : 2, role })
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    for (const label of ['Enhance würfeln', 'Lock anlegen', 'Stornieren']) {
      assert.equal(Boolean(button(view, label)), allowed, label)
    }
  })
}

for (const [file, component, label] of [
  ['Dashboard', 'DashboardPage', 'Anlegen'],
  ['Players', 'PlayersPage', 'Anlegen'],
  ['Polls', 'PollsPage', 'Anlegen'],
  ['Roster', 'RosterPage', 'Anlegen'],
]) {
  test(`${file} creation is synchronously guarded and shows pending UX`, async () => {
    const s = setup()
    const view = s.mount(s.load(`pages/${file}.tsx`)[component])
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
  assert.equal(button(view, 'Anmelden').props.disabled, false)
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
const completed = { raid: { status: 'COMPLETED' }, attendance: [attendanceRow], loot: [lootRow] }

test('completed attendance additions require trimmed reason and send all form fields', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  select(view, 'Teilnahme-Spieler').props.onChange(change('2'))
  view.render()
  s.requests.at(-1).resolve([character(20, 2)])
  await flush(view)
  input(view, 'Teilnahme-Begründung').props.onChange(change('   '))
  view.render()
  await form(view).props.onSubmit(event)
  view.render()
  assert.equal(writes(s).length, 0)
  assert.equal(button(view, 'Hinzufügen').props.disabled, true)
  assert.match(text(view.tree), /nichtleere Begründung/)
  input(view, 'Teilnahme-Begründung').props.onChange(change('  missed participant  '))
  checkbox(view, 'Bank').props.onChange({ target: { checked: true } })
  input(view, 'Teilnahme-Notizen').props.onChange(change('bench note'))
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
  assert.equal(write.body.notes, 'bench note')
  write.reject(new Error('offline'))
  await first
})

test('completed attendance edit prefills before values and PATCHes with an explicit reason', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  button(view, 'Teilnahme korrigieren').props.onClick()
  view.render()
  assert.match(text(view.tree), /Vorher:.*Spieler 1.*Charakter #.*11.*HEAL/)
  assert.match(text(view.tree), /old attendance note/)
  assert.equal(select(view, 'Teilnahme-Rolle').props.value, 'HEAL')
  assert.equal(checkbox(view, 'Ersatz').props.checked, true)
  assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
  s.requests.at(-1).resolve([character(10, 1), character(11, 1)])
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
  assert.equal(write.body.is_substitute, true)
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

test('loot corrections edit item, recipient, character, spec and notes without destructive controls', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  button(view, 'Loot korrigieren').props.onClick()
  view.render()
  assert.match(text(view.tree), /Vorher:.*Old item.*123.*Old boss.*Spieler 1/)
  assert.equal(input(view, 'Item').props.value, 'Old item')
  assert.equal(input(view, 'Loot-Charakter-ID').props.value, '11')
  assert.equal(input(view, 'Loot-Notizen').props.value, 'old loot note')
  await form(view, 1).props.onSubmit(event)
  assert.equal(writes(s).length, 0)
  input(view, 'Item').props.onChange(change('New item'))
  input(view, 'Item-ID').props.onChange(change('456'))
  input(view, 'Boss').props.onChange(change('New boss'))
  select(view, 'Loot-Empfänger').props.onChange(change('2'))
  checkbox(view, 'Main-Spec').props.onChange({ target: { checked: false } })
  input(view, 'Loot-Notizen').props.onChange(change('new loot note'))
  input(view, 'Loot-Begründung').props.onChange(change(' wrong recipient '))
  view.render()
  assert.equal(input(view, 'Loot-Charakter-ID').props.value, '')
  const submit = form(view, 1).props.onSubmit
  const first = submit(event)
  await submit(event)
  const write = writes(s)[0]
  assert.equal(writes(s).length, 1)
  assert.equal(write.method, 'patch')
  assert.equal(write.url, '/api/v1/raids/1/loot/8')
  assert.equal(write.body.item_name, 'New item')
  assert.equal(write.body.item_id, '456')
  assert.equal(write.body.boss, 'New boss')
  assert.equal(write.body.awarded_to_player_id, 2)
  assert.equal(write.body.awarded_to_character_id, null)
  assert.equal(write.body.is_main_spec, false)
  assert.equal(write.body.notes, 'new loot note')
  assert.equal(write.body.reason, 'wrong recipient')
  assert.equal('item_lock_id' in write.body, false)
  assert.equal(nodes(view.tree, (n) => n.type === 'button' && /löschen|delete/i.test(text(n))).length, 0)
  write.reject(new Error('offline'))
  await first
})

test('completed loot addition requires reason and sends optional character ID', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, completed)
  await flush(view)
  input(view, 'Item').props.onChange(change('Missed item'))
  select(view, 'Loot-Empfänger').props.onChange(change('2'))
  input(view, 'Loot-Charakter-ID').props.onChange(change('20'))
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
  assert.equal(input(view, 'Item').props.value, 'Other item')
  assert.equal(input(view, 'Loot-Begründung').props.value, '')
  button(view, 'Bearbeitung abbrechen').props.onClick()
  view.render()
  assert.equal(input(view, 'Item').props.value, '')
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
  assert.equal(input(view, 'Item').props.value, 'Old item')
  assert.equal(input(view, 'Loot-Begründung').props.value, 'reason')
  writes(s)[0].reject(new Error('offline'))
  await pending
})

test('attendance correction row switch invalidates character requests and old submits', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
  resolveRaid(s, 1, { ...completed, attendance: [attendanceRow, { ...attendanceRow, id: 9, player_id: 2, character_id: 20 }] })
  await flush(view)
  const buttons = nodes(view.tree, (n) => n.type === 'button' && text(n) === 'Teilnahme korrigieren')
  buttons[0].props.onClick()
  view.render()
  const oldCharacters = s.requests.at(-1)
  input(view, 'Teilnahme-Begründung').props.onChange(change('old reason'))
  view.render()
  const oldSubmit = form(view).props.onSubmit
  buttons[1].props.onClick()
  oldCharacters.resolve([character(11, 1)])
  await oldSubmit(event)
  await flush(view)
  assert.equal(writes(s).length, 0)
  assert.equal(input(view, 'Teilnahme-Begründung').props.value, '')
  assert.equal(button(view, 'Teilnahme-Korrektur speichern').props.disabled, true)
  s.requests.at(-1).resolve([character(20, 2)])
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
      s.requests.at(-1).resolve([character(11, 1)])
      await flush(view)
    }
    input(view, kind === 'attendance' ? 'Teilnahme-Begründung' : 'Loot-Begründung').props.onChange(change('reason for old raid'))
    view.render()
    const oldSubmit = form(view, kind === 'attendance' ? 0 : 1).props.onSubmit
    const pending = oldSubmit(event)
    assert.equal(writes(s)[0].method, 'patch')
    view.render({ raidID: 2 })
    resolveRaid(s, 2, { raid: { status: 'COMPLETED' } })
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
  ['CANCELLED', false, false, false], ['OPEN', true, false, false], ['COMPLETED', true, false, false],
]) {
  test(`raid lifecycle controls: ${status}, archived=${archived}`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, { ...completed, raid: { status, archived_at: archived ? '2026-10-04T00:00:00Z' : null } })
    await flush(view)
    assert.equal(Boolean(button(view, 'Abschließen')), active)
    assert.equal(Boolean(button(view, 'Absagen')), active)
    assert.equal(Boolean(button(view, 'Archivieren')), !archived)
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
  assert.equal(input(view, 'Item').props.value, 'Old item')
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

for (const status of ['PLANNED', 'OPEN']) {
  for (const kind of ['attendance', 'loot']) {
    test(`${status} ${kind} edit requires reason and guards pending PATCH`, async () => {
      const s = setup()
      const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
      resolveRaid(s, 1, { ...completed, raid: { status } })
      await flush(view)
      button(view, kind === 'attendance' ? 'Teilnahme korrigieren' : 'Loot korrigieren').props.onClick()
      view.render()
      if (kind === 'attendance') {
        s.requests.at(-1).resolve([character(11, 1)])
        await flush(view)
      }
      const index = kind === 'attendance' ? 0 : 1
      const reasonField = kind === 'attendance' ? 'Teilnahme-Begründung' : 'Loot-Begründung'
      const saveLabel = kind === 'attendance' ? 'Teilnahme-Korrektur speichern' : 'Loot-Korrektur speichern'
      assert.equal(input(view, reasonField).props.required, true)
      assert.match(text(view.tree), /Vorher:/)
      // Editing one form must not turn the other active-raid addition into a correction.
      assert.equal(input(view, kind === 'attendance' ? 'Loot-Begründung' : 'Teilnahme-Begründung'), undefined)
      input(view, reasonField).props.onChange(change('   '))
      view.render()
      await form(view, index).props.onSubmit(event)
      assert.equal(writes(s).length, 0)
      view.render()
      assert.equal(button(view, saveLabel).props.disabled, true)
      input(view, reasonField).props.onChange(change(' audited edit '))
      if (kind === 'attendance') {
        select(view, 'Teilnahme-Rolle').props.onChange(change('TANK'))
      } else {
        input(view, 'Item').props.onChange(change('Weiheabzeichen des Beschützers'))
        checkbox(view, 'Main-Spec').props.onChange({ target: { checked: false } })
      }
      view.render()
      const submit = form(view, index).props.onSubmit
      const first = submit(event)
      await submit(event)
      const write = writes(s)[0]
      assert.equal(writes(s).length, 1)
      assert.equal(write.method, 'patch')
      assert.equal(write.url, `/api/v1/raids/1/${kind}/${kind === 'attendance' ? 7 : 8}`)
      assert.equal(write.body.reason, 'audited edit')
      if (kind === 'attendance') assert.equal(write.body.role, 'TANK')
      else {
        assert.equal(write.body.item_name, 'Weiheabzeichen des Beschützers')
        assert.equal(write.body.is_main_spec, false)
      }
      view.render()
      assert.equal(button(view, saveLabel).props.disabled, true)
      write.reject(new Error('offline'))
      await first
      view.render()
      assert.equal(button(view, saveLabel).props.disabled, false)
      assert.equal(input(view, reasonField).props.value, ' audited edit ')
      button(view, 'Bearbeitung abbrechen').props.onClick()
      view.render()
      assert.equal(input(view, reasonField), undefined)
    })
  }

  test(`${status} additions do not require or send correction reasons`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/RaidDetail.tsx').RaidDetailPage, { raidID: 1 })
    resolveRaid(s, 1, { raid: { status } })
    await flush(view)
    assert.equal(input(view, 'Teilnahme-Begründung'), undefined)
    assert.equal(input(view, 'Loot-Begründung'), undefined)
    select(view, 'Teilnahme-Spieler').props.onChange(change('1'))
    view.render()
    s.requests.at(-1).resolve([character(10, 1)])
    await flush(view)
    const attendance = form(view).props.onSubmit(event)
    assert.equal(writes(s)[0].method, 'post')
    assert.equal('reason' in writes(s)[0].body, false)
    writes(s)[0].reject(new Error('offline'))
    await attendance
    view.render()
    const loot = form(view, 1).props.onSubmit(event)
    assert.equal(writes(s)[1].method, 'post')
    assert.equal('reason' in writes(s)[1].body, false)
    writes(s)[1].reject(new Error('offline'))
    await loot
  })
}

for (const reason of ['', ' \t ', null]) {
  test(`rename rejects blank or cancelled reason ${JSON.stringify(reason)} without request`, async () => {
    const s = setup()
    const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
    resolvePlayer(s, 1)
    await flush(view)
    s.prompts.push('New name', reason)
    await button(view, 'Umbenennen').props.onClick()
    view.render()
    assert.equal(writes(s).length, 0)
    if (reason !== null) assert.match(text(view.tree), /Umbenennen erfordert eine nichtleere Begründung/)
    else assert.doesNotMatch(text(view.tree), /Umbenennen erfordert/)
  })
}

test('rename sends trimmed nonblank reason and retains duplicate guard', async () => {
  const s = setup()
  const view = s.mount(s.load('pages/PlayerDetail.tsx').PlayerDetailPage, { playerID: 1 })
  resolvePlayer(s, 1)
  await flush(view)
  const rename = button(view, 'Umbenennen').props.onClick
  s.prompts.push('New name', ' in-game rename ', 'New name', ' duplicate ')
  const first = rename()
  await rename()
  assert.equal(writes(s).length, 1)
  assert.equal(writes(s)[0].method, 'patch')
  assert.equal(writes(s)[0].body.name, 'New name')
  assert.equal(writes(s)[0].body.reason, 'in-game rename')
  view.render()
  assert.equal(button(view, 'Umbenennen').props.disabled, true)
  writes(s)[0].reject(new Error('offline'))
  await first
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
