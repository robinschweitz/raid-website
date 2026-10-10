import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import {
  atLeast,
  type Character,
  type ExternalCharacter,
  type ItemLock,
  type NameHistory,
  type Player,
  type PointsResponse,
  type GuildRank,
  type LootAward,
  type Role,
} from '../types'
import { EnhancePointsCard } from '../components/EnhancePointsCard'
import { characterClassName } from '../characterTheme'
import { canonicalClass, useCharacterOptions } from '../characterOptions'
import { SpecSelect } from '../components/SpecSelect'

const CLASSES = [
  'Todesritter',
  'Druide',
  'Jäger',
  'Magier',
  'Paladin',
  'Priester',
  'Schamane',
  'Schurke',
  'Hexer',
  'Krieger',
]

const ROLES: Role[] = ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']
const LOOT_METHOD_LABELS = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }

interface Props {
  playerID: number
}

export function PlayerDetailPage({ playerID }: Props) {
  return <PlayerDetail key={playerID} playerID={playerID} />
}

function PlayerDetail({ playerID }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const historyVersion = useRef(0)
  const { player: me } = useAuth()
  const [player, setPlayer] = useState<Player | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [loot, setLoot] = useState<LootAward[]>([])
  const [locks, setLocks] = useState<ItemLock[]>([])
  const [history, setHistory] = useState<NameHistory[] | null>(null)
  const [historyChar, setHistoryChar] = useState<number | null>(null)
  const [ranks, setRanks] = useState<GuildRank[]>([])
  const [rankError, setRankError] = useState('')
  const [rankLoading, setRankLoading] = useState(true)
  const [error, setError] = useState('')
  const [view, setView] = useState<'points' | 'loot' | 'characters' | 'locks' | 'account'>('points')
  const [lootFilter, setLootFilter] = useState<'all' | 'tokens'>('all')

  const [charQuery, setCharQuery] = useState('')
  const [charResults, setCharResults] = useState<ExternalCharacter[]>([])
  const [charSelected, setCharSelected] = useState<ExternalCharacter | null>(null)
  const { options: classSpecs, error: optionsError, reload: reloadOptions } = useCharacterOptions()
  const [charIsMain, setCharIsMain] = useState(false)
  const [editingCharacter, setEditingCharacter] = useState<Character | null>(null)
  const [characterDraft, setCharacterDraft] = useState({ name: '', class: '', main_spec: '', second_spec: '', is_main: false, active: true })
  const characterEditVersion = useRef(0)
  const renderedCharacterEditVersion = characterEditVersion.current

  const [lockRaid, setLockRaid] = useState('')
  const [lockItem, setLockItem] = useState('')
  const [lockItemID, setLockItemID] = useState('')
  const [lockBoss, setLockBoss] = useState('')

  const [claimQuery, setClaimQuery] = useState('')
  const [claimResults, setClaimResults] = useState<ExternalCharacter[]>([])
  const [claimSelected, setClaimSelected] = useState<ExternalCharacter | null>(null)
  const [claimCode, setClaimCode] = useState('')
  const [claimStatus, setClaimStatus] = useState('')

  const [accUser, setAccUser] = useState('')
  const [accPass, setAccPass] = useState('')
  const [accRole, setAccRole] = useState<Role>('MEMBER')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false
  const isOwnProfile = me ? me.id === playerID : false
  const isOwnMember = me ? isOwnProfile && atLeast(me.role, 'MEMBER') : false

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [p, cs, pts, playerLoot, lk] = await Promise.all([
        api.get<Player>(`/api/v1/players/${playerID}`),
        api.get<Character[]>(`/api/v1/players/${playerID}/characters`),
        api.get<PointsResponse>(`/api/v1/players/${playerID}/points`),
        api.get<LootAward[]>(`/api/v1/players/${playerID}/loot`),
        api.get<ItemLock[]>(`/api/v1/players/${playerID}/locks`),
      ])
      if (isActive() && version === loadVersion.current) {
        setPlayer(p)
        setCharacters(cs)
        setPoints(pts)
        setLoot(playerLoot)
        setLocks(lk)
        setAccUser(p.username ?? '')
        if (p.role) setAccRole(p.role)
      }
    } catch (err) {
      if (isActive() && version === loadVersion.current) {
        setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen')
      }
    }
  }, [playerID, isActive])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    let active = true
    api.get<GuildRank[]>('/api/v1/guild-ranks').then((rows) => { if (active) setRanks(rows) })
      .catch(() => { if (active) setRankError('Gildenrang konnte nicht geladen werden.') })
      .finally(() => { if (active) setRankLoading(false) })
    return () => { active = false }
  }, [])

  const ownedCharacters = characters.filter((c) => c.player_id === playerID)
    .sort((a, b) => Number(b.is_main) - Number(a.is_main) || a.name.localeCompare(b.name, 'de') || a.id - b.id)
  const preferredCharacter = ownedCharacters[0]
  const visibleLoot = lootFilter === 'tokens' ? loot.filter((row) => row.token_award) : loot

  const createCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !charSelected) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Character>(`/api/v1/players/${playerID}/characters`, {
          profile_id: charSelected.profile_id,
          is_main: charIsMain,
        })
        if (!isActive()) return
        setCharQuery('')
        setCharResults([])
        setCharSelected(null)
        setCharIsMain(false)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter anlegen fehlgeschlagen')
      }
    })
  }

  const searchNewCharacters = async () => {
    if (!isAdmin || charQuery.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setCharSelected(null)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(charQuery.trim())}`)
        if (isActive()) setCharResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können angelegt werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen')
      }
    })
  }

  const toggleMain = async (c: Character) => {
    if (!isAdmin || c.player_id !== playerID) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${c.id}`, { is_main: !c.is_main })
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
      }
    })
  }

  const editCharacter = (c: Character | null) => {
    if (!isAdmin || !isActive() || isPending() || (c && c.player_id !== playerID)) return
    ++characterEditVersion.current
    setEditingCharacter(c)
    if (c) setCharacterDraft({ name: c.name, class: c.class, main_spec: c.main_spec, second_spec: c.second_spec, is_main: c.is_main, active: c.active })
  }

  const saveCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !editingCharacter || !characterDraft.name.trim() || renderedCharacterEditVersion !== characterEditVersion.current) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${editingCharacter.id}`, { ...characterDraft, name: characterDraft.name.trim() })
        if (!isActive()) return
        ++characterEditVersion.current
        setEditingCharacter(null)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter speichern fehlgeschlagen')
      }
    })
  }

  const changeCharacter = (patch: Partial<typeof characterDraft>) => {
    if (!isActive() || isPending() || renderedCharacterEditVersion !== characterEditVersion.current) return
    setCharacterDraft((previous) => ({ ...previous, ...patch }))
  }

  const showHistory = async (c: Character) => {
    const version = ++historyVersion.current
    setHistory(null)
    setHistoryChar(c.id)
    setError('')
    try {
      const hs = await api.get<NameHistory[]>(`/api/v1/characters/${c.id}/name-history`)
      if (!isActive() || version !== historyVersion.current) return
      setHistory(hs)
      setHistoryChar(c.id)
    } catch (err) {
      if (isActive() && version === historyVersion.current) setError(err instanceof ApiError ? err.message : 'Historie fehlgeschlagen')
    }
  }

  const createLock = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnMember) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ItemLock>(`/api/v1/players/${playerID}/locks`, {
          raid_id: Number(lockRaid),
          item_id: lockItemID,
          item_name: lockItem,
          boss: lockBoss,
        })
        if (!isActive()) return
        setLockRaid('')
        setLockItem('')
        setLockItemID('')
        setLockBoss('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Lock anlegen fehlgeschlagen')
      }
    })
  }

  const cancelLock = async (lock: ItemLock) => {
    if (!isOwnMember || lock.player_id !== playerID) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ItemLock>(`/api/v1/players/${playerID}/locks/${lock.id}/cancel`)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Lock stornieren fehlgeschlagen')
      }
    })
  }

  const searchClaimCharacters = async () => {
    if (!isOwnProfile || claimQuery.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setClaimStatus('')
      setClaimSelected(null)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(claimQuery.trim())}`)
        if (isActive()) setClaimResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können beansprucht werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen')
      }
    })
  }

  const requestClaim = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !claimSelected) return
    await submit(async () => {
      setError('')
      setClaimStatus('')
      try {
        await api.post('/api/v1/characters/claim-requests', { profile_id: claimSelected.profile_id })
        if (isActive()) setClaimStatus(`Anfrage für ${claimSelected.name} erstellt. Ein Admin sendet dir den Code ingame an diesen Charakter.`)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfrage fehlgeschlagen')
      }
    })
  }

  const redeemClaim = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !claimCode.trim()) return
    await submit(async () => {
      setError('')
      setClaimStatus('')
      try {
        await api.post('/api/v1/characters/claim-with-code', { code: claimCode })
        if (!isActive()) return
        setClaimCode('')
        setClaimStatus('Charakter wurde beansprucht.')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht eingelöst werden')
      }
    })
  }

  const setAccount = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.put<Player>(`/api/v1/players/${playerID}/account`, {
          username: accUser,
          password: accPass,
          role: accRole,
        })
        if (!isActive()) return
        setAccPass('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Konto speichern fehlgeschlagen')
      }
    })
  }

  if (!player) {
    return (
      <div>
        <p>
          <Link to="/players">← Spieler</Link>
        </p>
        {error ? <p className="error">{error}</p> : <p className="muted">Lade…</p>}
      </div>
    )
  }

  return (
    <div>
      <p>
        <Link to="/players">← Spieler</Link>
      </p>
      <header className={`card profile-header ${characterClassName(preferredCharacter?.class ?? '')}`}>
        <div className="profile-sigil" aria-hidden="true">{preferredCharacter?.name.slice(0, 1) ?? player.name.slice(0, 1)}</div>
        <div><p className="eyebrow">Spielerprofil</p><h1>{player.name} {player.role && <span className="badge">{player.role}</span>}</h1>
          {preferredCharacter ? <p><strong className="character-name">{preferredCharacter.name}</strong> · {preferredCharacter.class}{preferredCharacter.main_spec && ` · ${preferredCharacter.main_spec}`} {preferredCharacter.is_main && <span className="badge">Main</span>}</p> : <p className="muted">Noch keine eigenen Charaktere.</p>}
          {player.guild_rank_id != null && <p className="muted">Gildenrang: {ranks.find((r) => r.id === player.guild_rank_id)?.name ?? (rankError || (rankLoading ? 'Wird geladen…' : 'Nicht verfügbar'))}</p>}
        </div>
      </header>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}

      <nav className="workspace-tabs" aria-label="Profilbereiche">
        <button type="button" aria-pressed={view === 'points'} aria-controls="profile-points" onClick={() => setView('points')}>EP</button>
        <button type="button" aria-pressed={view === 'loot'} aria-controls="profile-loot" onClick={() => setView('loot')}>Loot ({loot.length})</button>
        <button type="button" aria-pressed={view === 'characters'} aria-controls="profile-characters" onClick={() => setView('characters')}>Charaktere ({ownedCharacters.length})</button>
        <button type="button" aria-pressed={view === 'locks'} aria-controls="profile-locks" onClick={() => setView('locks')}>Item-Locks</button>
        {isAdmin && <button type="button" aria-pressed={view === 'account'} aria-controls="profile-account" onClick={() => setView('account')}>Konto</button>}
      </nav>
      <div id="profile-points" hidden={view !== 'points'}>
        <EnhancePointsCard points={points} showHistory />
      </div>

      <section id="profile-loot" hidden={view !== 'loot'} className="card">
        <div className="section-heading">
          <div><h2>Loot</h2><p className="muted">Tokens sind normale Loot-Einträge und können hier gefiltert werden.</p></div>
          <label>Filter<select value={lootFilter} onChange={(event) => setLootFilter(event.target.value as 'all' | 'tokens')}>
            <option value="all">Alles</option>
            <option value="tokens">Nur Tokens</option>
          </select></label>
        </div>
        <div className="table-scroll"><table className="table">
          <thead><tr><th>Item</th><th>Boss</th><th>Charakter</th><th>Methode</th><th>Raid</th><th>Datum</th></tr></thead>
          <tbody>
            {visibleLoot.map((row) => {
              const method = row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
              return <tr key={row.id}>
                <td>{row.item_name}{row.token_award && <span className="badge">Token</span>}</td>
                <td>{row.boss || '—'}</td>
                <td className="character-name">{row.awarded_to_character_name || '—'}</td>
                <td>{LOOT_METHOD_LABELS[method]}</td>
                <td><Link to={`/raids/${row.raid_id}`}>Raid #{row.raid_id}</Link></td>
                <td>{new Date(row.awarded_at).toLocaleDateString('de-DE')}</td>
              </tr>
            })}
            {visibleLoot.length === 0 && <tr><td colSpan={6} className="muted">{lootFilter === 'tokens' ? 'Keine Tokens gefunden.' : 'Noch kein Loot.'}</td></tr>}
          </tbody>
        </table></div>
      </section>

      <section id="profile-characters" hidden={view !== 'characters'} className="card">
        <h2>Charaktere</h2>
        <div className="table-scroll"><table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Klasse</th>
              <th>Main-Spec</th>
              <th>Rising Gods</th>
              <th>Main</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {ownedCharacters.map((c) => (
              <tr key={c.id} className={characterClassName(c.class)}>
                {editingCharacter?.id === c.id ? <td colSpan={6}><form className="inline-form in-place-editor" onSubmit={saveCharacter}>
                  <label>Name<input autoFocus aria-label="Charakter bearbeiten: Name" disabled={pending} required value={characterDraft.name} onChange={(e) => changeCharacter({ name: e.target.value })} /></label>
                  <label>Klasse<select disabled={pending} value={characterDraft.class} onChange={(e) => changeCharacter({ class: e.target.value, main_spec: '', second_spec: '' })}>
                    {!CLASSES.includes(characterDraft.class) && <option value={characterDraft.class}>{characterDraft.class}</option>}
                    {CLASSES.map((cl) => <option key={cl} value={cl}>{cl}</option>)}
                  </select></label>
                  <SpecSelect label="Main-Spec" disabled={pending} value={characterDraft.main_spec} specs={classSpecs[canonicalClass(characterDraft.class)] ?? []} onChange={(value) => changeCharacter({ main_spec: value })} />
                  <SpecSelect label="Second-Spec" disabled={pending} value={characterDraft.second_spec} specs={classSpecs[canonicalClass(characterDraft.class)] ?? []} onChange={(value) => changeCharacter({ second_spec: value })} />
                  <label className="check"><input type="checkbox" disabled={pending} checked={characterDraft.is_main} onChange={(e) => changeCharacter({ is_main: e.target.checked })} />Main</label>
                  <label className="check"><input type="checkbox" disabled={pending} checked={characterDraft.active} onChange={(e) => changeCharacter({ active: e.target.checked })} />Aktiv</label>
                  <button type="submit" disabled={pending || !characterDraft.name.trim()}>Charakter speichern</button>
                  <button type="button" disabled={pending} onClick={() => editCharacter(null)}>Bearbeitung abbrechen</button>
                  {error && <p className="error" role="alert">{error}</p>}
                </form></td> : <>
                <td className="character-name">{c.name}</td>
                <td>{c.class}</td>
                <td>{c.main_spec || '—'}</td>
                <td>{c.external_profile_url ? <a href={c.external_profile_url} target="_blank" rel="noreferrer">verknüpft</a> : <span className="muted">—</span>}</td>
                <td>{c.is_main ? '★' : ''}</td>
                <td className="actions">
                  {isAdmin && (
                    <>
                      <button disabled={pending} onClick={() => toggleMain(c)}>Main umschalten</button>
                      <button disabled={pending} onClick={() => editCharacter(c)}>Bearbeiten</button>
                    </>
                  )}
                  <button onClick={() => showHistory(c)}>Historie</button>
                </td>
                </>}
              </tr>
            ))}
            {ownedCharacters.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  Keine Charaktere.
                </td>
              </tr>
            )}
          </tbody>
        </table></div>
        {history && (
          <div className="subcard">
            <h3>Namenshistorie #{historyChar}</h3>
            <ul>
              {history.map((h) => (
                <li key={h.id}>
                  {h.old_name} → {h.new_name} ({new Date(h.created_at).toLocaleDateString()})
                </li>
              ))}
              {history.length === 0 && <li className="muted">Keine Änderungen.</li>}
            </ul>
          </div>
        )}
        {isAdmin && (
          <details className="editor-disclosure"><summary>Charakter hinzufügen</summary>
          {optionsError && <p className="error">{optionsError} <button type="button" onClick={reloadOptions}>Erneut laden</button></p>}
          <form className="inline-form" onSubmit={createCharacter}>
            <label>Rising-Gods-Charakter<input value={charQuery} onChange={(e) => setCharQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={pending || charQuery.trim().length < 3} onClick={() => void searchNewCharacters()}>Suchen</button>
            <label className="check">
              <input type="checkbox" checked={charIsMain} onChange={(e) => setCharIsMain(e.target.checked)} /> Main
            </label>
            <button type="submit" disabled={pending || !charSelected}>Anlegen</button>
            {charSelected && <p className="muted">Ausgewählt: {charSelected.name} · {charSelected.class} · {charSelected.main_spec}</p>}
          </form>
          {charResults.length > 0 && <div className="plain-list">{charResults.map((row) => <button key={row.profile_id} type="button" className={charSelected?.profile_id === row.profile_id ? 'wishlist-toggle active' : 'wishlist-toggle'} onClick={() => setCharSelected(row)}>{row.name} · {row.class} · {row.main_spec || 'Spec unbekannt'} · Stufe {row.level || '—'}</button>)}</div>}
          </details>
        )}
        {isOwnProfile && (
          <details className="editor-disclosure"><summary>Charakter beanspruchen</summary>
          <p className="muted">Suche deinen Rising-Gods-Charakter. Ein Admin sendet dir den Code ingame an diesen Charakter.</p>
          <div className="inline-form">
            <label>Charakter suchen<input value={claimQuery} onChange={(e) => setClaimQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={pending || claimQuery.trim().length < 3} onClick={() => void searchClaimCharacters()}>Suchen</button>
          </div>
          {claimResults.length > 0 && <div className="plain-list">{claimResults.map((row) => <button key={row.profile_id} type="button" className={claimSelected?.profile_id === row.profile_id ? 'wishlist-toggle active' : 'wishlist-toggle'} onClick={() => setClaimSelected(row)}>{row.name} · {row.class} · Stufe {row.level || '—'}</button>)}</div>}
          <form className="inline-form" onSubmit={requestClaim}>
            <button type="submit" disabled={pending || !claimSelected}>Anfrage erstellen</button>
            {claimSelected && <p className="muted">Ausgewählt: {claimSelected.name} · {claimSelected.class}</p>}
          </form>
          <form className="inline-form" onSubmit={redeemClaim}>
            <label>Code<input value={claimCode} onChange={(e) => setClaimCode(e.target.value)} /></label>
            <button type="submit" disabled={pending || !claimCode.trim()}>Code einlösen</button>
          </form>
          {claimStatus && <p className="muted" role="status">{claimStatus}</p>}
          </details>
        )}
      </section>

      <section id="profile-locks" hidden={view !== 'locks'} className="card">
        <h2>Item-Locks</h2>
        <div className="table-scroll"><table className="table">
          <thead>
            <tr>
              <th>Item</th>
              <th>Boss</th>
              <th>Raid</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {locks.map((l) => (
              <tr key={l.id}>
                <td>{l.item_name || l.item_id}</td>
                <td>{l.boss || '—'}</td>
                <td><Link to={`/raids/${l.raid_id}`}>Raid #{l.raid_id}</Link></td>
                <td>{l.status}</td>
                <td>
                  {isOwnMember && l.status === 'ACTIVE' && <button disabled={pending} onClick={() => cancelLock(l)}>Stornieren</button>}
                </td>
              </tr>
            ))}
            {locks.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Keine Locks.
                </td>
              </tr>
            )}
          </tbody>
        </table></div>
        {isOwnMember && (
          <form className="inline-form" onSubmit={createLock}>
            <input aria-label="Raid-ID" placeholder="Raid-ID" value={lockRaid} onChange={(e) => setLockRaid(e.target.value)} required />
            <input aria-label="Item" placeholder="Item" value={lockItem} onChange={(e) => setLockItem(e.target.value)} required />
            <input aria-label="Item-ID" placeholder="Item-ID" value={lockItemID} onChange={(e) => setLockItemID(e.target.value)} />
            <input aria-label="Boss" placeholder="Boss" value={lockBoss} onChange={(e) => setLockBoss(e.target.value)} />
            <button type="submit" disabled={pending}>Lock anlegen</button>
          </form>
        )}
      </section>

      {isAdmin && (
        <section id="profile-account" hidden={view !== 'account'} className="card">
          <h2>Konto</h2>
          <form className="inline-form" onSubmit={setAccount}>
            <input aria-label="Benutzername" placeholder="Benutzername" value={accUser} onChange={(e) => setAccUser(e.target.value)} required />
            <input
              type="password"
              aria-label="Passwort (leer = unverändert)"
              placeholder="Passwort (leer = unverändert)"
              value={accPass}
              onChange={(e) => setAccPass(e.target.value)}
            />
            <select aria-label="Kontorolle" value={accRole} onChange={(e) => setAccRole(e.target.value as Role)}>
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
            <button type="submit" disabled={pending}>Speichern</button>
          </form>
        </section>
      )}
    </div>
  )
}
