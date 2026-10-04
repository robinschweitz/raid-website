import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { catalogModeLabels, type CatalogItemDrop, type LootCatalog } from '../types'
import { catalogSourceLabels, matchesCatalogItem } from '../catalog'
import { useSubmission } from '../useSubmission'

export function LootCatalogPage() {
  const { isActive } = useSubmission()
  const loadVersion = useRef(0)
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [drops, setDrops] = useState<CatalogItemDrop[]>([])
  const [instanceID, setInstanceID] = useState('')
  const [bossID, setBossID] = useState('')
  const [size, setSize] = useState('')
  const [mode, setMode] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(0)
  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setLoading(true)
    setError('')
    try {
      const [value, items] = await Promise.all([
        api.get<LootCatalog>('/api/v1/loot-catalog'),
        api.get<CatalogItemDrop[]>('/api/v1/loot-catalog/items'),
      ])
      if (isActive() && version === loadVersion.current) { setCatalog(value); setDrops(items); setPage(0) }
    } catch (err) {
      if (isActive() && version === loadVersion.current) setError(err instanceof ApiError ? err.message : 'Beutekatalog konnte nicht geladen werden. Bitte erneut versuchen; kein Ersatzdatensatz wird verwendet.')
    } finally {
      if (isActive() && version === loadVersion.current) setLoading(false)
    }
  }, [isActive])
  useEffect(() => { void load() }, [load])

  const bosses = (catalog?.bosses ?? []).filter((boss) => !instanceID || boss.instance_id === instanceID)
  const eligibleBosses = new Set(bosses.map((boss) => boss.id))
  const results = drops.filter((item) => eligibleBosses.has(item.boss_id) && (!bossID || item.boss_id === bossID)
    && (!size || item.raid_size === Number(size)) && (!mode || item.mode === mode) && matchesCatalogItem(item, query))
    .sort((a, b) => a.name.localeCompare(b.name, 'de') || a.boss_id.localeCompare(b.boss_id) || a.raid_size - b.raid_size || a.mode.localeCompare(b.mode))
  const pageSize = 50
  const lastPage = Math.max(0, Math.ceil(results.length / pageSize) - 1)
  const currentPage = Math.min(page, lastPage)
  const names = new Map((catalog?.bosses ?? []).map((boss) => [boss.id, boss.name]))

  return <div>
    <div className="page-heading"><div><p className="eyebrow">Bosse · Begegnungen · Truhen</p><h1>Beutekatalog</h1></div></div>
    <section className="card"><h2>Beute finden</h2>
      <p className="muted">Truhenbeute ist direkt der Begegnung zugeordnet – etwa dem Luftschiffkampf oder Valithria. Suche nach Gegenstand oder Item-ID.</p>
      {error && <p className="error" role="alert">{error}</p>}
      {loading ? <p className="muted" role="status">Beutekatalog wird geladen…</p> : catalog && <>
        <div className="catalog-filters">
          <label>Instanz<select value={instanceID} onChange={(e) => { setInstanceID(e.target.value); setBossID(''); setPage(0) }}><option value="">Alle Instanzen</option>{catalog.instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}</select></label>
          <label>Boss / Begegnung<select value={bossID} onChange={(e) => { setBossID(e.target.value); setPage(0) }}><option value="">Alle Begegnungen</option>{bosses.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}</select></label>
          <label>Raidgröße<select value={size} onChange={(e) => { setSize(e.target.value); setPage(0) }}><option value="">10 und 25 Spieler</option><option value="10">10 Spieler</option><option value="25">25 Spieler</option></select></label>
          <label>Modus<select value={mode} onChange={(e) => { setMode(e.target.value); setPage(0) }}><option value="">Alle Modi</option>{Object.entries(catalogModeLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
          <label>Gegenstand suchen<input type="search" value={query} placeholder="Name oder Item-ID" onChange={(e) => { setQuery(e.target.value); setPage(0) }} /></label>
        </div>
        <p role="status">{results.length} Dropvarianten gefunden</p>
        {results.length === 0 && <p className="muted">Keine passenden Drops im gebündelten Bestand. Filter ändern oder Quellenabdeckung prüfen.</p>}
        <div className="catalog-results">{results.slice(currentPage * pageSize, (currentPage + 1) * pageSize).map((item) => <article className="subcard" key={JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.mode])}>
          <h3>{item.name}</h3>
          <p>{names.get(item.boss_id)} · {item.raid_size} Spieler · {catalogModeLabels[item.mode]}</p>
          <p className="muted">Item #{item.item_id}{item.conditional ? ' · Bedingte Beute' : ''}</p>
          <ul>{catalogSourceLabels(item).map((label) => <li key={label}>{label}</li>)}</ul>
          <details><summary>Quelldetails</summary>{item.source_variants.map((variant) => <p className="catalog-source-path" key={`${variant.difficulty}/${variant.loot_mode}`}>{variant.difficulty} · Lootmaske {variant.loot_mode}</p>)}</details>
        </article>)}</div>
        {results.length > pageSize && <div className="catalog-pagination"><button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>Zurück</button><span>Seite {currentPage + 1} / {lastPage + 1}</span><button type="button" disabled={currentPage === lastPage} onClick={() => setPage(currentPage + 1)}>Weiter</button></div>}
        <details className="subcard"><summary>Abdeckung und Quellen</summary>
        <p>{catalog.instances.length} Instanzen · {catalog.bosses.length} Begegnungs-/Teilnehmereinträge · {catalog.items.length} Items</p>
        <p className="muted">Teilbestand: Fehlende Drops bedeuten nicht, dass eine Begegnung keine Beute hat. Manche Kreaturen-Hardmodes sind nur als bedingte Quellpfade belegt.</p>
        {catalog.instances.length === 0 && <p className="muted">Keine geprüften Dropdaten verfügbar.</p>}
        {catalog.sources.map((entry) => <details className="subcard" key={`${entry.source.id}-${entry.raw_hash}`}>
          <summary>Quelle: {entry.source.id} · {entry.source.license}</summary>
          <dl style={{ overflowWrap: 'anywhere' }}>
            <dt>Quelldatei / Quelle</dt><dd>{entry.source.id}</dd>
            <dt>Version</dt><dd>{entry.source.revision}</dd>
            <dt>Quellenadresse</dt><dd>{entry.source.url}</dd>
            <dt>Lizenz</dt><dd>{entry.source.license}</dd>
            <dt>Attribution</dt><dd>{entry.source.attribution}</dd>
            <dt>JSON-Prüfsumme</dt><dd>{entry.raw_hash}</dd>
          </dl>
          <ul>{entry.notices.map((notice, index) => <li key={index} style={{ overflowWrap: 'anywhere' }}>{notice}</li>)}</ul>
        </details>)}
        </details>
      </>}
      <button type="button" disabled={loading} onClick={() => void load()}>Bestand neu laden</button>
    </section>
  </div>
}
