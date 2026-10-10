const assert = require('node:assert/strict')
const { test } = require('node:test')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const ts = require('typescript')

// Render real TSX with deterministic hooks and deferred API reads; no DOM dependency.
function setup(player = { id: 7, name: 'Real player', role: 'MEMBER' }) {
  const cache = new Map(), requests = [], slots = []
  let index = 0, effects = [], active = true, tree, component
  let fetchMock = () => { throw new Error('Unexpected fetch') }
  let now = Date.parse('2026-10-04T12:00:00Z')
  const timers = new Map()
  const windowMock = {
    setInterval(fn) { const id = timers.size + 1; timers.set(id, fn); return id },
    clearInterval(id) { timers.delete(id) },
  }
  const jsx = (type, props) => ({ type, props: props ?? {} })
  const react = {
    useState(initial) {
      const slot = slots[index++] ?? (slots[index - 1] = { value: typeof initial === 'function' ? initial() : initial })
      return [slot.value, (value) => { if (active) slot.value = typeof value === 'function' ? value(slot.value) : value }]
    },
    useEffect(effect, deps) {
      const slot = slots[index++] ?? (slots[index - 1] = {})
      if (!slot.deps || deps.some((value, i) => !Object.is(value, slot.deps[i]))) {
        slot.deps = deps
        effects.push(() => { slot.cleanup?.(); slot.cleanup = effect() })
      }
    },
    useRef(initial) { return slots[index++] ?? (slots[index - 1] = { current: initial }) },
    useCallback(fn, deps) {
      const slot = slots[index++] ?? (slots[index - 1] = {})
      if (!slot.deps || deps.some((value, i) => !Object.is(value, slot.deps[i]))) { slot.deps = deps; slot.fn = fn }
      return slot.fn
    },
  }
  react.useLayoutEffect = react.useEffect
  class FixedDate extends Date { static now() { return now } }
  function load(relative) {
    const filename = path.resolve(__dirname, '../src', relative)
    if (cache.has(filename)) return cache.get(filename)
    const module = { exports: {} }
    const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      fileName: filename,
      compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
    }).outputText
    const requireLocal = (name) => {
      if (name === 'react') return react
      if (name === 'react/jsx-runtime') return { jsx, jsxs: jsx }
      if (name.endsWith('/router')) return { Link: 'a' }
      if (name.endsWith('/auth')) return { useAuth: () => ({ player }) }
      if (name.endsWith('/api')) return { ...load('api.ts'), api: Object.fromEntries(['get', 'post', 'patch'].map((method) => [method, (url, body) => new Promise((resolve, reject) => requests.push({ method, url, body, resolve, reject }))])) }
      const resolved = path.resolve(path.dirname(filename), name)
      return load(path.relative(path.resolve(__dirname, '../src'), resolved + (fs.existsSync(resolved + '.ts') ? '.ts' : '.tsx')))
    }
    vm.runInThisContext(`(function(require,module,exports,Date,fetch,window){${source}\n})`, { filename })(requireLocal, module, module.exports, FixedDate, (...args) => fetchMock(...args), windowMock)
    cache.set(filename, module.exports)
    return module.exports
  }
  function render() {
    index = 0
    tree = component()
    const pending = effects
    effects = []
    pending.forEach((effect) => effect())
    return tree
  }
  return {
    load, requests, render,
    mount(fn) { component = fn; return render() },
    get tree() { return tree },
    setFetch(fn) { fetchMock = fn },
    advanceTo(date) { now = Date.parse(date); timers.forEach((fn) => fn()); render() },
    timers,
    unmount() { slots.forEach((slot) => slot.cleanup?.()); active = false },
  }
}
function text(tree) {
  if (tree == null || typeof tree === 'boolean') return ''
  if (Array.isArray(tree)) return tree.map(text).join(' ')
  return typeof tree === 'object' ? text(typeof tree.type === 'function' ? tree.type(tree.props) : tree.props?.children) : String(tree)
}
function nodes(tree, predicate) {
  if (!tree || typeof tree !== 'object') return []
  if (Array.isArray(tree)) return tree.flatMap((child) => nodes(child, predicate))
  return [...(predicate(tree) ? [tree] : []), ...nodes(tree.props?.children, predicate)]
}
async function flush(s) { for (let i = 0; i < 12; i++) await Promise.resolve(); s.render() }
function resolveOverview(s, raids = [], polls = []) {
  for (const request of s.requests) {
    if (request.url.includes('include_archived')) request.resolve(raids)
    else if (request.url === '/api/v1/players') request.resolve([{ id: 2, name: 'Actual leader' }])
    else if (request.url === '/api/v1/polls') request.resolve(polls)
    else if (request.url.endsWith('/points')) request.resolve({ balance: 30, transactions: [] })
    else if (request.url.endsWith('/characters')) request.resolve([])
  }
}

for (const [balance, range] of [[30, '31–130'], [70, '51–150'], [1, '2–101'], [0, 'Keine Verstärkung'], [-15, 'Keine Verstärkung']]) {
  test(`EP card previews real balance ${balance}, never offers a spend control`, () => {
    const s = setup()
    const tree = s.load('components/EnhancePointsCard.tsx').EnhancePointsCard({ points: { balance, transactions: [] } })
    assert.match(text(tree), new RegExp(range))
    assert.match(text(tree), /Verlorene Würfe kosten keine EP/)
    assert.equal(nodes(tree, (n) => ['button', 'input', 'select', 'form'].includes(n.type)).length, 0)
    if (balance < 1) assert.match(text(tree), /Kosten 0 EP/)
    if (balance < 0) assert.match(text(tree), /-15/)
  })
}

test('EP ledger preserves reasons, signed amounts, actor and actual raid links', () => {
  const s = setup()
  const tree = s.load('components/EnhancePointsCard.tsx').EnhancePointsCard({ showHistory: true, points: { balance: 10, transactions: [
    { id: 1, amount: 10, type: 'award_no_mainspec_loot', reason: 'Archivierung', raid_id: 42, actor_id: 2, created_at: '2026-10-01T18:00:00Z' },
    { id: 2, amount: -30, type: 'enhanced_loot', reason: 'Recorded winner', raid_id: 43, actor_id: 3, created_at: '2026-10-02T18:00:00Z' },
  ] } })
  assert.match(text(tree), /Raidbonus ohne Main-Spec-Loot/)
  assert.match(text(tree), /Recorded winner/)
  assert.match(text(tree), /Akteur # 3/)
  assert.equal(nodes(tree, (n) => n.props.className === 'positive').length, 1)
  assert.equal(nodes(tree, (n) => n.props.className === 'negative').length, 1)
  assert.deepEqual(nodes(tree, (n) => n.type === 'a').map((n) => n.props.to), ['/raids/42', '/raids/43'])
})

test('dashboard chooses earliest actual future active raid and fetches its composition and real votes', async () => {
  const s = setup()
  s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
  const raid = (id, name, date, status = 'PLANNED', archived_at = null) => ({ id, name, scheduled_start: date, status, archived_at, raid_leader_id: 2 })
  resolveOverview(s, [
    raid(1, 'Old completed raid', '2026-10-03T18:00:00Z', 'COMPLETED'),
    raid(2, 'Later raid', '2026-10-09T18:00:00Z'),
    { ...raid(3, 'Real next raid', '2026-10-05T18:00:00Z', 'OPEN'), raid_size: 10 },
    raid(4, 'Cancelled raid', '2026-10-04T18:00:00Z', 'CANCELLED'),
    raid(5, 'Archived raid', '2026-10-04T17:00:00Z', 'COMPLETED', '2026-10-01T18:00:00Z'),
    raid(6, 'Unscheduled raid', null),
  ], [{ id: 8, title: 'Actual poll', status: 'OPEN', created_at: '2026-10-04T10:00:00Z' }, { id: 9, title: 'Closed poll', status: 'CLOSED' }])
  await flush(s)
  assert.deepEqual(s.requests.filter((r) => r.url.endsWith('/attendance')).map((r) => r.url), ['/api/v1/raids/3/attendance'])
  assert.match(text(s.tree), /Stimmen werden geladen…/)
  s.requests.find((r) => r.url.endsWith('/attendance')).resolve([{ role: 'TANK' }, { role: 'HEAL' }, { role: 'DD' }, { role: 'DD', is_bench: true }])
  s.requests.find((r) => r.url.endsWith('/votes')).resolve([{ id: 1, choice: 'Montag' }, { id: 2, choice: 'Montag' }])
  await flush(s)
  const banner = nodes(s.tree, (n) => typeof n.type === 'function' && n.type.name === 'RaidBanner')[0]
  assert.match(text(banner), /Real next raid/)
  assert.match(text(banner), /Actual leader/)
  assert.match(text(banner), /10\s+Spieler/)
  assert.match(text(banner), /4\s+Aufgestellt.*1\s+Tanks.*1\s+Heiler.*2\s+DD/)
  assert.match(text(s.tree), /2 abgegebene Stimmen/)
  assert.doesNotMatch(text(s.tree), /Closed poll|ICC25/)
  assert.match(text(s.tree), /Montag.*2\s+Stimmen/)
  assert.equal(nodes(s.tree, (n) => n.type === 'form').length, 0)
  for (const className of ['widget-composition', 'widget-attendance', 'widget-poll', 'widget-notifications', 'widget-history', 'widget-profile', 'widget-agenda']) {
    assert.equal(nodes(s.tree, (n) => n.props.className === `card ${className}`).length, 1)
  }
})

for (const status of ['PLANNED', 'OPEN']) {
  test(`dashboard retains ${status} raids at and after start until finalization`, async () => {
    const s = setup()
    s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
    resolveOverview(s, [
      { id: 2, name: 'Future raid', status: 'PLANNED', scheduled_start: '2026-10-05T18:00:00Z' },
      { id: 1, name: 'Current raid', status, scheduled_start: '2026-10-04T12:00:01Z' },
    ])
    await flush(s)
    const bannerText = () => text(nodes(s.tree, (n) => n.type?.name === 'RaidBanner')[0])
    assert.match(bannerText(), /Nächster geplanter Raid/)
    assert.match(bannerText(), /Current raid/)
    s.advanceTo('2026-10-04T12:00:01Z')
    assert.match(bannerText(), /Laufender Raid/)
    assert.match(bannerText(), /Läuft/)
    assert.match(bannerText(), /Current raid/)
    s.advanceTo('2026-10-06T12:00:00Z')
    assert.match(bannerText(), /Current raid/)
    assert.deepEqual(s.requests.filter((r) => r.url.endsWith('/attendance')).map((r) => r.url), ['/api/v1/raids/1/attendance'])
    assert.equal(s.requests.filter((r) => r.method !== 'get').length, 0)
    s.unmount()
    assert.equal(s.timers.size, 0)
  })

  test(`dashboard selects already started ${status} raids on initial load`, async () => {
    const s = setup()
    s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
    resolveOverview(s, [
      { id: 2, name: 'Future raid', status: 'PLANNED', scheduled_start: '2026-10-05T18:00:00Z' },
      { id: 1, name: 'Running raid', status, scheduled_start: '2026-10-03T18:00:00Z' },
    ])
    await flush(s)
    const banner = nodes(s.tree, (n) => n.type?.name === 'RaidBanner')[0]
    assert.match(text(banner), /Laufender Raid.*Läuft.*Running raid/)
    s.unmount()
  })
}

test('dashboard empty state invents no raid, capacity or statistics', async () => {
  const s = setup()
  s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
  resolveOverview(s)
  await flush(s)
  assert.match(text(s.tree), /Kein zukünftiger Raid geplant/)
  assert.match(text(s.tree), /Noch keine archivierten Raids/)
  assert.match(text(s.tree), /Keine Aufstellung verfügbar/)
  assert.match(text(s.tree), /Keine Benachrichtigungen/)
  assert.doesNotMatch(text(s.tree), /ICC25|25 Spieler|Fortschritt|%/)
})

test('dashboard network failure is explicit, not an empty success', async () => {
  const s = setup()
  s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
  s.requests.find((r) => r.url.includes('include_archived')).reject(new Error('offline'))
  await flush(s)
  assert.match(text(s.tree), /Übersicht konnte nicht geladen/)
  assert.match(text(s.tree), /Raidplanung nicht verfügbar/)
  assert.doesNotMatch(text(s.tree), /Kein zukünftiger Raid geplant/)
})

test('dashboard ignores late reads after leaving the page', async () => {
  const s = setup()
  s.mount(s.load('pages/Dashboard.tsx').DashboardPage)
  const count = s.requests.length
  s.unmount()
  resolveOverview(s, [{ id: 1, name: 'Late raid', status: 'OPEN', scheduled_start: '2026-10-05T18:00:00Z' }], [{ id: 2, title: 'Late poll', status: 'OPEN' }])
  await flush(s)
  assert.equal(s.requests.length, count)
  assert.doesNotMatch(text(s.tree), /Late raid|Late poll/)
})

test('profile contains no legacy roll endpoint, type or control', () => {
  const source = fs.readFileSync(path.resolve(__dirname, '../src/pages/PlayerDetail.tsx'), 'utf8')
  assert.doesNotMatch(source, /points\/roll|RollResult|doRoll|Enhance würfeln/)
  assert.match(source, /EnhancePointsCard points=\{points\} showHistory/)
  assert.match(source, /filter\(\(c\) => c.player_id === playerID\)/)
})

test('shell separates real dashboard from raid creation and provides accessible safe-area navigation', () => {
  const source = fs.readFileSync(path.resolve(__dirname, '../src/App.tsx'), 'utf8')
  const css = fs.readFileSync(path.resolve(__dirname, '../src/index.css'), 'utf8')
  assert.match(source, /parts\[0\] === 'raids'\) return <RaidsPage/)
  assert.match(source, /aria-label="Mobile Hauptnavigation"/)
  assert.match(source, /Gildenkonfiguration/)
  assert.match(css, /env\(safe-area-inset-bottom\)/)
  assert.match(css, /prefers-reduced-motion/)
  assert.doesNotMatch(css, /@import|https?:\/\//)
})

const emptyCatalog = { sources: [], instances: [], bosses: [], items: [], drops: [] }
const dataset = {
  source: { id: 'reviewed-source', revision: 'pinned-revision', url: 'https://example.test/source', license: 'Reviewed license', attribution: 'Source authors' },
  notices: ['Incomplete source-derived fixture; not a complete raid catalog.'],
  instances: [{ id: '631', name: 'Reviewed instance' }, { id: '603', name: 'Other instance' }],
  bosses: [{ id: '36612', instance_id: '631', name: 'Reviewed boss' }, { id: '1', instance_id: '603', name: 'Other boss' }],
  items: [{ item_id: '50759', name: 'Reviewed item', quality: 4 }],
  drops: [{ item_id: '50759', name: 'Reviewed item', quality: 4, boss_id: '36612', raid_size: 10, mode: 'NORMAL', conditional: false, source_variants: [{ difficulty: 'raid-difficulty:0', loot_mode: 1 }] }, { item_id: '50759', name: 'Reviewed item', quality: 4, boss_id: '1', raid_size: 25, mode: 'NORMAL', conditional: false, source_variants: [{ difficulty: 'raid-difficulty:0', loot_mode: 1 }] }],
}
const catalog = { ...dataset, sources: [{ source: dataset.source, raw_hash: 'a'.repeat(64), notices: dataset.notices }] }
const findButton = (s, label) => nodes(s.tree, (node) => node.type === 'button' && text(node) === label)[0]
const writes = (s) => s.requests.filter((request) => request.method !== 'get')
const fileInput = (s) => nodes(s.tree, (node) => node.type === 'input' && node.props.type === 'file')[0]
// The page also reads the viewer's characters and wishlists; only catalog reads matter here.
const catalogReads = (requests) => requests.filter((request) => request.url.startsWith('/api/v1/loot-catalog'))
function resolveCatalogLoad(requests, value, drops = value.drops) {
  requests = catalogReads(requests)
  assert.deepEqual(requests.map((request) => request.url), ['/api/v1/loot-catalog', '/api/v1/loot-catalog/items'])
  requests[0].resolve(value)
  requests[1].resolve(drops)
}
for (const role of ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']) {
  test(`${role} reads the file-authoritative catalog without upload or mutation controls`, async () => {
    const s = setup({ id: 1, role })
    s.mount(s.load('pages/LootCatalog.tsx').LootCatalogPage)
    resolveCatalogLoad(s.requests, catalog)
    await flush(s)
    assert.match(text(s.tree), /Reviewed item/)
    assert.match(text(s.tree), /2\s+Beutevarianten gefunden/)
    assert.doesNotMatch(text(s.tree), /Administration|nur von einem Admin/)
    assert.equal(fileInput(s), undefined)
    assert.equal(findButton(s, 'Katalog importieren'), undefined)
    assert.equal(findButton(s, 'Vorschau prüfen'), undefined)
    findButton(s, 'Beute neu laden').props.onClick()
    resolveCatalogLoad(s.requests.slice(-2), emptyCatalog)
    await flush(s)
    assert.match(text(s.tree), /Keine passende Beute gefunden/)
    assert.equal(writes(s).length, 0)
  })
}

for (const failedIndex of [0, 1]) {
  test(`catalog load reports failure of ${failedIndex === 0 ? 'metadata' : 'items'} and retries both reads`, async () => {
    const s = setup()
    s.mount(s.load('pages/LootCatalog.tsx').LootCatalogPage)
    const initial = catalogReads(s.requests)
    initial[failedIndex].reject(new Error('offline'))
    initial[1 - failedIndex].resolve(failedIndex === 0 ? catalog.drops : catalog)
    await flush(s)
    assert.match(text(s.tree), /erneut versuchen/)
    assert.doesNotMatch(text(s.tree), /Reviewed item|Beutevarianten gefunden/)
    assert.equal(findButton(s, 'Beute neu laden').props.disabled, false)
    findButton(s, 'Beute neu laden').props.onClick()
    const retry = catalogReads(s.requests).slice(2)
    retry[0].resolve(catalog)
    await flush(s)
    assert.match(text(s.tree), /Beute wird geladen/)
    assert.doesNotMatch(text(s.tree), /Beutevarianten gefunden/)
    retry[1].resolve(catalog.drops)
    await flush(s)
    assert.match(text(s.tree), /2\s+Beutevarianten gefunden/)
    assert.doesNotMatch(text(s.tree), /erneut versuchen/)
    assert.equal(writes(s).length, 0)
  })
}

test('catalog GET failure offers retry and late reads cannot overwrite newer data or unmounted state', async () => {
  const s = setup()
  s.mount(s.load('pages/LootCatalog.tsx').LootCatalogPage)
  s.requests[0].reject(new Error('offline'))
  await flush(s)
  assert.match(text(s.tree), /erneut versuchen/)
  const retry = findButton(s, 'Beute neu laden').props.onClick
  const retryStart = s.requests.length
  retry(); retry()
  resolveCatalogLoad(s.requests.slice(retryStart + 2), catalog)
  await flush(s)
  resolveCatalogLoad(s.requests.slice(retryStart, retryStart + 2), emptyCatalog)
  await flush(s)
  assert.match(text(s.tree), /Reviewed instance/)
  assert.match(text(s.tree), /Reviewed item/)
  const beforeUnmount = s.requests.length
  findButton(s, 'Beute neu laden').props.onClick()
  s.unmount()
  resolveCatalogLoad(s.requests.slice(-2), emptyCatalog)
  await flush(s)
  assert.doesNotMatch(text(s.tree), /Keine passende Beute gefunden/)
  assert.equal(s.requests.length, beforeUnmount + 2)
  assert.equal(writes(s).length, 0)
})

test('catalog GET sends session credentials and retains source metadata', async () => {
  const s = setup()
  s.setFetch(async (url, options) => {
    assert.equal(url, '/api/v1/loot-catalog')
    assert.equal(options.method, 'GET')
    assert.equal(options.credentials, 'include')
    return { ok: true, status: 200, text: async () => JSON.stringify({ data: catalog }) }
  })
  assert.deepEqual(await s.load('api.ts').api.get('/api/v1/loot-catalog'), catalog)
})

test('catalog association sizes come only from drops for bosses of the selected instance', () => {
  const s = setup()
  const { catalogRaidSizes } = s.load('types.ts')
  assert.deepEqual(catalogRaidSizes(catalog, '631'), [10])
  assert.deepEqual(catalogRaidSizes(catalog, '603'), [25])
  assert.deepEqual(catalogRaidSizes(catalog, '999'), [])
  const fields = s.load('components/CatalogAssociationFields.tsx').CatalogAssociationFields({ catalog, instanceID: '631', raidSize: '', disabled: false, onChange() {} })
  const sizeSelect = nodes(fields, (node) => node.type === 'select' && node.props['aria-label'] === 'Raidgröße')[0]
  assert.match(text(sizeSelect), /10\s+Spieler/)
  assert.doesNotMatch(text(sizeSelect), /25\s+Spieler/)
})

test('new raid inherits series catalog association and optional explicit selection is persisted', async () => {
  const s = setup({ id: 1, role: 'ADMIN' })
  s.mount(s.load('pages/Raids.tsx').RaidsPage)
  s.requests.forEach((request) => request.resolve(request.url === '/api/v1/loot-catalog' ? catalog : request.url === '/api/v1/raid-series' ? [{ id: 3, name: 'Actual series', raid_leader_id: 2, catalog_instance_id: '631', raid_size: 10 }] : []))
  await flush(s)
  nodes(s.tree, (node) => node.type === 'select' && node.props['aria-label'] === 'Neue Raidserie')[0].props.onChange({ target: { value: '3' } })
  nodes(s.tree, (node) => node.type === 'input' && node.props.type === 'datetime-local')[0].props.onChange({ target: { value: '2026-10-05T20:00' } })
  s.render()
  let fields = nodes(s.tree, (node) => typeof node.type === 'function' && node.type.name === 'CatalogAssociationFields')[0]
  assert.equal(fields.props.instanceID, '631')
  assert.equal(fields.props.raidSize, '10')
  fields.props.onChange('603', '25'); s.render()
  nodes(s.tree, (node) => node.type === 'form')[0].props.onSubmit({ preventDefault() {} })
  assert.equal(writes(s)[0].body.catalog_instance_id, '603')
  assert.equal(writes(s)[0].body.raid_size, 25)
  writes(s)[0].reject(new Error('offline'))
  await flush(s)
})

test('series editing preloads association, omits unchanged fields and validates changed size', async () => {
  const s = setup({ id: 1, role: 'ADMIN' })
  const series = { id: 3, name: 'Existing series', raid_leader_id: 2, weekdays: [1], interval_weeks: 1, start_time: '20:00', timezone: 'Europe/Berlin', catalog_instance_id: '631', raid_size: 10 }
  s.mount(s.load('pages/RaidSeries.tsx').RaidSeriesPage)
  s.requests.forEach((request) => request.resolve(request.url === '/api/v1/loot-catalog' ? catalog : request.url === '/api/v1/raid-series' ? [series] : []))
  await flush(s)
  findButton(s, 'Serie bearbeiten').props.onClick(); s.render()
  let fields = nodes(s.tree, (node) => typeof node.type === 'function' && node.type.name === 'CatalogAssociationFields')[0]
  assert.equal(fields.props.instanceID, '631')
  assert.equal(fields.props.raidSize, '10')
  nodes(s.tree, (node) => node.type === 'form')[0].props.onSubmit({ preventDefault() {} })
  assert.equal('catalog_instance_id' in writes(s)[0].body, false)
  assert.equal('raid_size' in writes(s)[0].body, false)
  writes(s)[0].reject(new Error('offline')); await flush(s)
  fields = nodes(s.tree, (node) => typeof node.type === 'function' && node.type.name === 'CatalogAssociationFields')[0]
  fields.props.onChange('631', '25'); s.render()
  await nodes(s.tree, (node) => node.type === 'form')[0].props.onSubmit({ preventDefault() {} })
  assert.equal(writes(s).length, 1)
  fields = nodes(s.tree, (node) => typeof node.type === 'function' && node.type.name === 'CatalogAssociationFields')[0]
  fields.props.onChange('603', '25'); s.render()
  nodes(s.tree, (node) => node.type === 'form')[0].props.onSubmit({ preventDefault() {} })
  assert.equal(writes(s)[1].body.catalog_instance_id, '603')
  assert.equal(writes(s)[1].body.raid_size, 25)
  writes(s)[1].reject(new Error('offline')); await flush(s)
})
