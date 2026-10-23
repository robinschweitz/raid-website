import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { Link } from '../router'
import { characterClassName } from '../characterTheme'
import { useAuth } from '../auth'
import { useSubmission } from '../useSubmission'
import { CharacterWishlist } from '../components/CharacterWishlist'
import { atLeast, type Character, type LootAward, type NameHistory, type Player } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'

const LOOT_METHOD_LABELS = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const LOOT_METHOD_CLASSES = { MAIN: 'main', SECOND: 'second', ENHANCED: 'enhanced', BOE: 'boe' }

interface Props {
  characterID: number
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleDateString('de-DE')
}

export function CharacterDetailPage({ characterID }: Props) {
  return <CharacterDetail key={characterID} characterID={characterID} />
}

function CharacterDetail({ characterID }: Props) {
  const { player: me } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [character, setCharacter] = useState<Character | null>(null)
  const [owner, setOwner] = useState<Player | null>(null)
  const [loot, setLoot] = useState<LootAward[]>([])
  const [history, setHistory] = useState<NameHistory[]>([])
  const [namesOpen, setNamesOpen] = useState(false)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')

  useEffect(() => {
    let active = true
    void (async () => {
      setError('')
      setStatus('')
      setNamesOpen(false)
      try {
        const loadedCharacter = await api.get<Character>(`/api/v1/characters/${characterID}`)
        if (!active) return
        setCharacter(loadedCharacter)
        const [loadedOwner, playerLoot, loadedHistory] = await Promise.all([
          api.get<Player>(`/api/v1/players/${loadedCharacter.player_id}`),
          api.get<LootAward[]>(`/api/v1/players/${loadedCharacter.player_id}/loot`),
          api.get<NameHistory[]>(`/api/v1/characters/${loadedCharacter.id}/name-history`),
        ])
        if (!active) return
        setOwner(loadedOwner)
        setLoot(playerLoot)
        setHistory(loadedHistory)
      } catch (err) {
        if (active) setError(err instanceof ApiError ? err.message : 'Charakter konnte nicht geladen werden')
      }
    })()
    return () => { active = false }
  }, [characterID])

  if (!character) {
    return <div><p><Link to="/players">← Spieler</Link></p>{error ? <ErrorMessage>{error}</ErrorMessage> : <p className="muted">Lade Charakter…</p>}</div>
  }

  const exactLoot = loot.filter((row) => row.awarded_to_character_id === character.id)
  const nameFallbackLoot = exactLoot.length === 0
    ? loot.filter((row) => !row.awarded_to_character_id && row.awarded_to_character_name?.trim() === character.name)
    : []
  const visibleLoot = exactLoot.length > 0 ? exactLoot : nameFallbackLoot
  const hasNameHistory = history.length > 0
  const canReloadProfile = Boolean(me && atLeast(me.role, 'ADMIN') && character.external_source === 'rising-gods' && character.external_profile_id)

  // The server lets owners and staff read a wishlist; only the owner (MEMBER and up) edits it.
  const isOwner = Boolean(me && me.id === character.player_id)
  const canReadWishlist = Boolean(me && (isOwner || atLeast(me.role, 'RAIDASSISTENT')))
  const canEditWishlist = Boolean(me && isOwner && atLeast(me.role, 'MEMBER'))

  const canRetryLink = Boolean(me && atLeast(me.role, 'ADMIN') && !character.external_profile_id && !character.external_source)

  const retryLink = async () => {
    if (!canRetryLink) return
    await submit(async () => {
      setError('')
      setStatus('')
      try {
        const linked = await api.post<Character>(`/api/v1/characters/${character.id}/retry-link`)
        if (!isActive()) return
        setCharacter(linked)
        setStatus('Rising-Gods-Profil wurde erfolgreich verknüpft.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Rising-Gods-Verknüpfung fehlgeschlagen')
      }
    })
  }

  const reloadProfile = async () => {
    if (!canReloadProfile) return
    await submit(async () => {
      setError('')
      setStatus('')
      try {
        const reloaded = await api.post<Character>(`/api/v1/characters/${character.id}/reload-profile`)
        if (!isActive()) return
        setCharacter(reloaded)
        setStatus('Rising-Gods-Profil wurde neu geladen.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Profil konnte nicht neu geladen werden')
      }
    })
  }

  return <div className="character-profile-page">
    <p><Link to={owner ? `/players/${owner.id}` : '/players'}>← {owner ? owner.name : 'Spieler'}</Link></p>
    <header className={`card character-hero ${characterClassName(character.class)}`}>
      <div className="profile-sigil" aria-hidden="true">{character.name.slice(0, 1)}</div>
      <div className="character-hero-main">
        <p className="eyebrow">Charakterprofil</p>
        <div className="character-title-row">
          <h1>{character.name}</h1>
          {hasNameHistory && <button type="button" className="name-history-toggle" aria-expanded={namesOpen} aria-label="Frühere Namen anzeigen" onClick={() => setNamesOpen((open) => !open)}><span aria-hidden="true" /></button>}
        </div>
        {hasNameHistory && namesOpen && <div className="name-history-popover" role="region" aria-label="Frühere Namen">
          <strong>Frühere Namen</strong>
          <ul className="plain-list">
            {history.map((row) => <li key={row.id}>{row.old_name} → {row.new_name} <span className="muted">· {formatDate(row.created_at)}</span></li>)}
          </ul>
        </div>}
        <p><strong>{character.class}</strong>{character.main_spec && ` · ${character.main_spec}`}{character.second_spec && ` / ${character.second_spec}`} {character.is_main && <span className="badge">Main</span>} {!character.active && <span className="badge bad">Inaktiv</span>}</p>
        {owner && <p className="muted">Gehört zu <Link to={`/players/${owner.id}`}>{owner.name}</Link>.</p>}
        {canRetryLink && <div className="actions character-profile-actions"><button type="button" disabled={pending} onClick={() => void retryLink()}>{pending ? 'Suche Profil…' : 'Rising-Gods-Verknüpfung erneut versuchen'}</button></div>}
        {canReloadProfile && <div className="actions character-profile-actions"><button type="button" disabled={pending} onClick={() => void reloadProfile()}>{pending ? 'Lade neu…' : 'Rising-Gods-Profil neu laden'}</button></div>}
      </div>
    </header>

    {error && <ErrorMessage>{error}</ErrorMessage>}
    {status && <p className="muted" role="status">{status}</p>}

    <section className="card character-facts">
      <h2>Details</h2>
      <dl>
        <div><dt>Main-Spec</dt><dd>{character.main_spec || '—'}</dd></div>
        <div><dt>Second-Spec</dt><dd>{character.second_spec || '—'}</dd></div>
        <div><dt>GearScore</dt><dd>{character.gear_score ?? '—'}</dd></div>
        <div><dt>Status</dt><dd>{character.active ? 'Aktiv' : 'Inaktiv'}</dd></div>
        <div><dt>Angelegt</dt><dd>{formatDate(character.created_at)}</dd></div>
        <div><dt>Letzte Namensänderung</dt><dd>{formatDate(character.name_changed_at)}</dd></div>
        <div><dt>Rising Gods</dt><dd>{character.external_profile_url ? <a href={character.external_profile_url} target="_blank" rel="noreferrer">Profil öffnen</a> : '—'}</dd></div>
      </dl>
    </section>

    {canReadWishlist && <CharacterWishlist characterID={character.id} canEdit={canEditWishlist} />}

    <section className="card character-loot-history">
      <div className="section-heading"><div><h2>Loot-Historie</h2><p className="muted">Direkte Charaktertreffer über gespeicherte Charakter-ID.</p></div><span className="badge info">{visibleLoot.length}</span></div>
      {nameFallbackLoot.length > 0 && <p className="muted">Diese Einträge wurden über den gespeicherten Namen gefunden, weil historische Loot-Zeilen keine Charakter-ID enthalten.</p>}
      <div className="table-scroll"><table className="table loot-table">
        <thead><tr><th>Item</th><th>Boss</th><th>Methode</th><th>Raid</th><th>Datum</th></tr></thead>
        <tbody>{visibleLoot.map((row) => {
          const method = row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
          return <tr key={row.id}>
            <td><strong className="loot-item-name">{row.item_name}</strong>{row.token_award && <span className="badge">Token</span>}</td>
            <td>{row.boss || '—'}</td>
            <td><span className={`loot-method ${LOOT_METHOD_CLASSES[method]}`}>{LOOT_METHOD_LABELS[method]}</span>{method === 'ENHANCED' && <span className="loot-cost">{row.enhanced_cost ?? 0} EP</span>}</td>
            <td><Link to={`/raids/${row.raid_id}`}>Raid #{row.raid_id}</Link></td>
            <td>{formatDate(row.awarded_at)}</td>
          </tr>
        })}{visibleLoot.length === 0 && <tr><td colSpan={5} className="muted">Kein Loot für diesen Charakter.</td></tr>}</tbody>
      </table></div>
    </section>
  </div>
}
