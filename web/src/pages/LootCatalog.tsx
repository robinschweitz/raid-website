import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { catalogModeLabels, type CatalogItemDrop, type LootCatalog, type Character, type WishlistWish } from '../types'
import { matchesCatalogItem } from '../catalog'
import { useSubmission } from '../useSubmission'
import { useAuth } from '../auth'
import { NavIcon } from '../components/NavIcon'

const qualityClasses: Record<number, string> = {
  2: 'uncommon',
  3: 'rare',
  4: 'epic',
  5: 'legendary',
}

const qualityLabels: Record<number, string> = {
  0: 'Schlecht',
  1: 'Gewöhnlich',
  2: 'Ungewöhnlich',
  3: 'Selten',
  4: 'Episch',
  5: 'Legendär',
}

function itemMeta(item: CatalogItemDrop): string[] {
  const values: string[] = []
  if (item.item_level) values.push(`iLvl ${item.item_level}`)
  if (item.required_level) values.push(`Stufe ${item.required_level}`)
  if (item.slot) values.push(item.slot)
  if (item.type) values.push(item.type)
  if (item.set_name) values.push(`Set: ${item.set_name}`)
  if (item.conditional) values.push('Bedingte Beute')
  return values
}

const statLabels: Record<string, string> = {
  stamina: 'Ausdauer',
  strength: 'Stärke',
  agility: 'Beweglichkeit',
  intellect: 'Intelligenz',
  spirit: 'Willenskraft',
  spell_power: 'Zaubermacht',
  attack_power: 'Angriffskraft',
  hit_rating: 'Trefferwertung',
  haste_rating: 'Tempowertung',
  critical_strike_rating: 'Kritische Trefferwertung',
  expertise_rating: 'Waffenkundewertung',
  armor_penetration_rating: 'Rüstungsdurchschlag',
  defense_rating: 'Verteidigungswertung',
  dodge_rating: 'Ausweichwertung',
  parry_rating: 'Parierwertung',
  block_rating: 'Blockwertung',
  mana_per_5: 'Mana pro 5 Sek.',
  spell_penetration: 'Zauberdurchschlag',
  shield_block_value: 'Blockwert',
}

const bindingLabels: Record<string, string> = {
  bind_on_pickup: 'Wird beim Aufheben gebunden',
  bind_on_equip: 'Wird beim Anlegen gebunden',
  bind_to_account: 'Accountgebunden',
  bind_to_bnet_account: 'Battle.net-accountgebunden',
}

function itemStats(item: CatalogItemDrop): { label: string; value: string }[] {
  const values: { label: string; value: string }[] = []
  if (item.armor) values.push({ label: 'Rüstung', value: String(item.armor) })
  const stats = item.stats ?? {}
  for (const [key, value] of Object.entries(stats)) {
    values.push({ label: statLabels[key] ?? key.replace(/_/g, ' '), value: `+${value}` })
  }
  if (!item.stats) {
    if (item.stamina) values.push({ label: 'Ausdauer', value: `+${item.stamina}` })
    if (item.strength) values.push({ label: 'Stärke', value: `+${item.strength}` })
    if (item.agility) values.push({ label: 'Beweglichkeit', value: `+${item.agility}` })
    if (item.intellect) values.push({ label: 'Intelligenz', value: `+${item.intellect}` })
    if (item.spirit) values.push({ label: 'Willenskraft', value: `+${item.spirit}` })
    if (item.spell_power) values.push({ label: 'Zaubermacht', value: `+${item.spell_power}` })
  }
  if (item.dps != null) values.push({ label: 'DPS', value: item.dps.toFixed(2) })
  if (item.dps == null && item.min_damage != null && item.max_damage != null) {
    values.push({ label: 'Schaden', value: `${item.min_damage.toFixed(1)}-${item.max_damage.toFixed(1)}` })
  }
  return values
}

function wowheadItemURL(itemID: string): string {
  return `https://www.wowhead.com/wotlk/item=${encodeURIComponent(itemID)}`
}

function itemIconURL(item: CatalogItemDrop): string {
  const icon = item.icon ?? item.icon_name
  return icon ? `https://wow.zamimg.com/images/wow/icons/large/${icon}.jpg` : ''
}

function itemWeaponLine(item: CatalogItemDrop): string {
  const weapon = item.weapon
  const min = weapon?.min_damage ?? item.min_damage
  const max = weapon?.max_damage ?? item.max_damage
  const speed = weapon?.speed
  const dps = weapon?.dps ?? item.dps
  const parts = []
  if (min != null && max != null) parts.push(`${min.toFixed(0)}-${max.toFixed(0)} Schaden`)
  if (speed != null) parts.push(`Tempo ${speed.toFixed(2)}`)
  if (dps != null) parts.push(`${dps.toFixed(2)} DPS`)
  return parts.join(' · ')
}

function localizedTooltipLines(item: CatalogItemDrop): string[] {
  const text = item.locales?.deDE?.tooltip_text ?? item.locales?.enUS?.tooltip_text ?? ''
  if (!text) return item.effects ?? []
  return text.split('\n').map((line) => line.trim()).filter((line) => {
    if (!line || line === item.name || line.startsWith('Item Level') || line.startsWith('Gegenstandsstufe')) return false
    if (line.startsWith('Verkaufspreis') || line.startsWith('Sell Price') || line.startsWith('Dropchance') || line.startsWith('Drop Chance')) return false
    return /^(Anlegen:|Benutzen:|Chance|Erhöht|Verbessert|Benötigt|Equip:|Use:|Requires)/.test(line)
  }).slice(0, 5)
}

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
  const { player: me } = useAuth()
  const [ownChars, setOwnChars] = useState<Character[]>([])
  const [charWishes, setCharWishes] = useState<Record<number, Set<string>>>({})
  const [pendingWish, setPendingWish] = useState('')
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
  useEffect(() => {
    if (!me) { setOwnChars([]); setCharWishes({}); return; }
    api.get<Character[]>(`/api/v1/players/${me.id}/characters`).catch(() => []).then(async (chars) => {
      const nextChars = chars ?? []
      setOwnChars(nextChars)
      const wishLists = await Promise.all(nextChars.map(async (char) => {
        const wishes = await api.get<WishlistWish[]>(`/api/v1/characters/${char.id}/wishlist`).catch(() => [])
        return [char.id, new Set((wishes ?? []).map((wish) => wish.item_id))] as const
      }))
      if (isActive()) setCharWishes(Object.fromEntries(wishLists))
    })
  }, [isActive, me])

  async function toggleWish(char: Character, item: CatalogItemDrop) {
    const key = char.id
    const pendingKey = `${char.id}:${item.item_id}`
    if (pendingWish) return
    setPendingWish(pendingKey)
    const set = charWishes[key] ?? new Set<string>()
    if (set.has(item.item_id)) {
      try {
        await api.del(`/api/v1/characters/${char.id}/wishlist/items/${item.item_id}`)
        const next = new Set(set)
        next.delete(item.item_id)
        setCharWishes((s) => ({ ...s, [key]: next }))
      } catch { /* ignore */ }
    } else {
      try {
        await api.post(`/api/v1/characters/${char.id}/wishlist/items`, { item_id: item.item_id, item_name: item.name, priority: 0, notes: '' })
        const next = new Set(set)
        next.add(item.item_id)
        setCharWishes((s) => ({ ...s, [key]: next }))
      } catch { /* ignore */ }
    }
    setPendingWish('')
  }

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
        <div className="catalog-results">{results.slice(currentPage * pageSize, (currentPage + 1) * pageSize).map((item) => <article className="subcard catalog-item-card" key={JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.mode])}>
          <div className="catalog-tooltip-card">
            <div className="catalog-item-main">
              <a className="catalog-item-art" href={wowheadItemURL(item.item_id)} target="_blank" rel="noreferrer" aria-label={`${item.name} bei Wowhead öffnen`}>{itemIconURL(item) ? <img src={itemIconURL(item)} alt="" /> : <NavIcon name="package" />}</a>
              <div>
                <h3 className={item.quality ? `item-quality-${qualityClasses[item.quality] ?? 'common'}` : undefined}>{item.name}</h3>
                <p>{names.get(item.boss_id)} · {item.raid_size} Spieler · {catalogModeLabels[item.mode]}</p>
                <div className="catalog-pills">
                  {item.quality != null && <span>{qualityLabels[item.quality] ?? `Qualität ${item.quality}`}</span>}
                  {itemMeta(item).map((value) => <span key={value}>{value}</span>)}
                </div>
              </div>
            </div>
            {(item.binding_type || item.unique || itemWeaponLine(item)) && <div className="catalog-tooltip-lines">
              {item.binding_type && <p>{bindingLabels[item.binding_type] ?? item.binding_type}</p>}
              {item.unique && <p>Einzigartig</p>}
              {itemWeaponLine(item) && <p>{itemWeaponLine(item)}</p>}
            </div>}
            {itemStats(item).length > 0 && <dl className="catalog-stat-grid">{itemStats(item).map((stat) => <div key={stat.label}><dt>{stat.label}</dt><dd>{stat.value}</dd></div>)}</dl>}
            {(item.sockets?.length || item.socket_bonus) && <div className="catalog-sockets">
              {item.sockets?.map((socket, index) => <span key={`${socket}-${index}`} className={`socket-${socket}`}>{socket}</span>)}
              {item.socket_bonus && <strong>Sockelbonus: {item.socket_bonus.text}</strong>}
            </div>}
            {localizedTooltipLines(item).length > 0 && <ul className="catalog-effects">{localizedTooltipLines(item).map((line) => <li key={line}>{line}</li>)}</ul>}
          </div>
          {me && ownChars.length > 0 && <div className="catalog-wishlist"><strong>Wunschliste</strong><div>
            {ownChars.map((char) => {
              const wished = charWishes[char.id]?.has(item.item_id) ?? false
              const disabled = pendingWish === `${char.id}:${item.item_id}`
              return <button key={char.id} type="button" className={wished ? 'wishlist-toggle active' : 'wishlist-toggle'} disabled={Boolean(pendingWish) && !disabled} onClick={() => toggleWish(char, item)}>
                <NavIcon name={wished ? 'heart' : 'plus'} />{char.name}
              </button>
            })}
          </div></div>}
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
