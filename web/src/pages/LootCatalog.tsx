import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { Link } from '../router'
import { atLeast, catalogModeLabels, type CatalogItemDrop, type LootCatalog, type Character, type WishlistWish } from '../types'
import { matchesCatalogItem } from '../catalog'
import { useSubmission } from '../useSubmission'
import { useAuth } from '../auth'
import { NavIcon } from '../components/NavIcon'
import { BossOrderEditor } from '../components/BossOrderEditor'

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
  return values
}

function itemTooltipText(item: CatalogItemDrop): string {
  return item.locales?.deDE?.tooltip_text ?? item.locales?.enUS?.tooltip_text ?? ''
}

function itemTooltipDamage(item: CatalogItemDrop): string {
  const text = itemTooltipText(item)
  const match = text.match(/\d[\d.,]*\s*-\s*\d[\d.,]*\s+[^\n]*Schaden/i)
    ?? text.match(/\d[\d.,]*\s*-\s*\d[\d.,]*\s+[^\n]*Damage/i)
  return match?.[0] ?? ''
}

function wowheadItemURL(itemID: string): string {
  return `https://www.wowhead.com/wotlk/item=${encodeURIComponent(itemID)}`
}

function itemIconURL(item: CatalogItemDrop): string {
  const icon = item.icon ?? item.icon_name
  return icon ? `https://wow.zamimg.com/images/wow/icons/large/${icon}.jpg` : ''
}

function itemWeaponDetails(item: CatalogItemDrop): { kind: string; damage: string; speed: string; dps: string } | null {
  const min = item.weapon?.min_damage ?? item.min_damage
  const max = item.weapon?.max_damage ?? item.max_damage
  const speed = item.weapon?.speed
  const dps = item.weapon?.dps ?? item.dps
  if (min == null && max == null && speed == null && dps == null) return null
  return {
    kind: [item.slot, item.type].filter(Boolean).join(' · '),
    damage: min != null && max != null ? `${min.toFixed(0)} - ${max.toFixed(0)} Schaden` : itemTooltipDamage(item),
    speed: speed != null ? `Tempo ${speed.toFixed(2)}` : '',
    dps: dps != null ? `(${dps.toFixed(2)} Schaden pro Sekunde)` : '',
  }
}

function CatalogWeaponLines({ details }: { details: NonNullable<ReturnType<typeof itemWeaponDetails>> }) {
  return <div className="catalog-weapon-lines">
    {details.kind && <p className="catalog-weapon-kind">{details.kind}</p>}
    {(details.damage || details.speed) && <p className="catalog-weapon-damage"><span>{details.damage}</span><span>{details.speed}</span></p>}
    {details.dps && <p className="catalog-weapon-dps">{details.dps}</p>}
  </div>
}

function localizedTooltipLines(item: CatalogItemDrop): string[] {
  const text = itemTooltipText(item)
  if (!text) return item.effects ?? []
  return text.split('\n').map((line) => line.trim()).filter((line) => {
    if (!line || line === item.name || line.startsWith('Item Level') || line.startsWith('Gegenstandsstufe')) return false
    if (line.startsWith('Benötigt Stufe') || line.startsWith('Requires Level')) return false
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
      if (isActive() && version === loadVersion.current) setError(err instanceof ApiError ? err.message : 'Beute konnte nicht geladen werden. Bitte erneut versuchen.')
    } finally {
      if (isActive() && version === loadVersion.current) setLoading(false)
    }
  }, [isActive])
  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (!me || !atLeast(me.role, 'MEMBER')) { setOwnChars([]); setCharWishes({}); return; }
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

  const [wishNotice, setWishNotice] = useState('')
  const wishCount = Object.values(charWishes).reduce((total, set) => total + set.size, 0)

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
        setWishNotice(`„${item.name}“ von der Wunschliste von ${char.name} entfernt.`)
      } catch (err) {
        setWishNotice(err instanceof ApiError ? err.message : 'Wunschliste konnte nicht geändert werden.')
      }
    } else {
      try {
        await api.post(`/api/v1/characters/${char.id}/wishlist/items`, { item_id: item.item_id, item_name: item.name, priority: 0, notes: '' })
        const next = new Set(set)
        next.add(item.item_id)
        setCharWishes((s) => ({ ...s, [key]: next }))
        setWishNotice(`„${item.name}“ steht jetzt auf der Wunschliste von ${char.name}.`)
      } catch (err) {
        setWishNotice(err instanceof ApiError ? err.message : 'Wunschliste konnte nicht geändert werden.')
      }
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
    <div className="page-heading"><div><p className="eyebrow">Bosse · Begegnungen · Truhen</p><h1>Beute</h1></div></div>
    {me && atLeast(me.role, 'MEMBER') && ownChars.length > 0 && <div className="catalog-wishlist-banner" role="status">
      <span>{wishNotice || <>Klick bei einem Item auf deinen Charakter, um es auf die Wunschliste zu setzen. {wishCount > 0 && <>Aktuell <strong>{wishCount === 1 ? '1 Item' : `${wishCount} Items`}</strong> auf deiner Wunschliste.</>}</>}</span>
      <Link className="button-link" to="/wishlist">Meine Wunschliste &amp; Addon-Export &rarr;</Link>
    </div>}
    <section className="card"><h2>Beute finden</h2>
      <p className="muted">Truhenbeute ist direkt der Begegnung zugeordnet, etwa dem Luftschiffkampf oder Valithria. Suche nach Gegenstand oder Item-ID.</p>
      {error && <p className="error" role="alert">{error}</p>}
      {loading ? <p className="muted" role="status">Beute wird geladen…</p> : catalog && <>
        <div className="catalog-filters">
          <label>Instanz<select value={instanceID} onChange={(e) => { setInstanceID(e.target.value); setBossID(''); setPage(0) }}><option value="">Alle Instanzen</option>{catalog.instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}</select></label>
          <label>Boss / Begegnung<select value={bossID} onChange={(e) => { setBossID(e.target.value); setPage(0) }}><option value="">Alle Begegnungen</option>{bosses.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}</select></label>
          <label>Raidgröße<select value={size} onChange={(e) => { setSize(e.target.value); setPage(0) }}><option value="">10 und 25 Spieler</option><option value="10">10 Spieler</option><option value="25">25 Spieler</option></select></label>
          <label>Modus<select value={mode} onChange={(e) => { setMode(e.target.value); setPage(0) }}><option value="">Alle Modi</option>{Object.entries(catalogModeLabels).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
          <label>Gegenstand suchen<input type="search" value={query} placeholder="Name oder Item-ID" onChange={(e) => { setQuery(e.target.value); setPage(0) }} /></label>
        </div>
        <p role="status">{results.length} Beutevarianten gefunden</p>
        {results.length === 0 && <p className="muted">Keine passende Beute gefunden. Filter ändern oder später erneut versuchen.</p>}
        <div className="catalog-results">{results.slice(currentPage * pageSize, (currentPage + 1) * pageSize).map((item) => {
          const weaponDetails = itemWeaponDetails(item)
          return <article className="subcard catalog-item-card" key={JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.mode])}>
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
            {(item.binding_type || item.unique || weaponDetails) && <div className="catalog-tooltip-lines">
              {item.binding_type && <p>{bindingLabels[item.binding_type] ?? item.binding_type}</p>}
              {item.unique && <p>Einzigartig</p>}
              {weaponDetails && <CatalogWeaponLines details={weaponDetails} />}
            </div>}
            {item.armor != null && <p className="catalog-armor-line"><strong>{item.armor}</strong> Rüstung</p>}
            {itemStats(item).length > 0 && <dl className="catalog-stat-grid">{itemStats(item).map((stat) => <div key={stat.label}><dt>{stat.value}</dt><dd>{stat.label}</dd></div>)}</dl>}
            {(item.sockets?.length || item.socket_bonus) && <div className="catalog-sockets">
              {item.sockets?.map((socket, index) => <span key={`${socket}-${index}`} className={`socket-${socket}`}>{socket}</span>)}
              {item.socket_bonus && <strong>Sockelbonus: {item.socket_bonus.text}</strong>}
            </div>}
            {localizedTooltipLines(item).length > 0 && <ul className="catalog-effects">{localizedTooltipLines(item).map((line) => <li key={line}>{line}</li>)}</ul>}
          </div>
          {me && atLeast(me.role, 'MEMBER') && ownChars.length > 0 && <div className="catalog-wishlist"><strong>Wunschliste</strong><div>
            {ownChars.map((char) => {
              const wished = charWishes[char.id]?.has(item.item_id) ?? false
              const disabled = pendingWish === `${char.id}:${item.item_id}`
              return <button key={char.id} type="button" className={wished ? 'wishlist-toggle active' : 'wishlist-toggle'} disabled={Boolean(pendingWish) && !disabled} onClick={() => toggleWish(char, item)}>
                <NavIcon name={wished ? 'heart' : 'plus'} />{char.name}
              </button>
            })}
          </div></div>}
        </article>})}</div>
        {results.length > pageSize && <div className="catalog-pagination"><button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>Zurück</button><span>Seite {currentPage + 1} / {lastPage + 1}</span><button type="button" disabled={currentPage === lastPage} onClick={() => setPage(currentPage + 1)}>Weiter</button></div>}
      </>}
      <button type="button" disabled={loading} onClick={() => void load()}>Beute neu laden</button>
    </section>
    {me && atLeast(me.role, 'ADMIN') && catalog && <BossOrderEditor catalog={catalog} onSaved={() => void load()} />}
  </div>
}
