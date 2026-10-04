import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import {
  atLeast,
  type Character,
  type ItemLock,
  type NameHistory,
  type Player,
  type PointsResponse,
  type GuildRank,
  type Role,
  type TokensResponse,
} from '../types'
import { EnhancePointsCard } from '../components/EnhancePointsCard'
import { characterClassName } from '../characterTheme'

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

interface Props {
  playerID: number
}

export function PlayerDetailPage({ playerID }: Props) {
  return <PlayerDetail key={playerID} playerID={playerID} />
}

function PlayerDetail({ playerID }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const loadVersion = useRef(0)
  const historyVersion = useRef(0)
  const { player: me } = useAuth()
  const [player, setPlayer] = useState<Player | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [tokens, setTokens] = useState<TokensResponse | null>(null)
  const [locks, setLocks] = useState<ItemLock[]>([])
  const [history, setHistory] = useState<NameHistory[] | null>(null)
  const [historyChar, setHistoryChar] = useState<number | null>(null)
  const [ranks, setRanks] = useState<GuildRank[]>([])
  const [rankError, setRankError] = useState('')
  const [rankLoading, setRankLoading] = useState(true)
  const [error, setError] = useState('')

  const [charName, setCharName] = useState('')
  const [charClass, setCharClass] = useState(CLASSES[0])
  const [charMainSpec, setCharMainSpec] = useState('')
  const [charIsMain, setCharIsMain] = useState(false)

  const [lockRaid, setLockRaid] = useState('')
  const [lockItem, setLockItem] = useState('')
  const [lockItemID, setLockItemID] = useState('')
  const [lockBoss, setLockBoss] = useState('')

  const [accUser, setAccUser] = useState('')
  const [accPass, setAccPass] = useState('')
  const [accRole, setAccRole] = useState<Role>('MEMBER')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false
  const isOwnMember = me ? me.id === playerID && atLeast(me.role, 'MEMBER') : false

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [p, cs, pts, tks, lk] = await Promise.all([
        api.get<Player>(`/api/v1/players/${playerID}`),
        api.get<Character[]>(`/api/v1/players/${playerID}/characters`),
        api.get<PointsResponse>(`/api/v1/players/${playerID}/points`),
        api.get<TokensResponse>(`/api/v1/players/${playerID}/tokens`),
        api.get<ItemLock[]>(`/api/v1/players/${playerID}/locks`),
      ])
      if (isActive() && version === loadVersion.current) {
        setPlayer(p)
        setCharacters(cs)
        setPoints(pts)
        setTokens(tks)
        setLocks(lk)
        setAccUser(p.username ?? '')
        setAccRole(p.role)
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

  const createCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Character>(`/api/v1/players/${playerID}/characters`, {
          name: charName,
          class: charClass,
          main_spec: charMainSpec,
          is_main: charIsMain,
        })
        if (!isActive()) return
        setCharName('')
        setCharMainSpec('')
        setCharIsMain(false)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter anlegen fehlgeschlagen')
      }
    })
  }

  const toggleMain = async (c: Character) => {
    if (!isAdmin || c.player_id !== playerID) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${c.id}`, { is_main: !c.is_main, reason: 'main flag change' })
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
      }
    })
  }

  const renameCharacter = async (c: Character) => {
    if (!isAdmin || c.player_id !== playerID || pending) return
    const next = window.prompt('Neuer Name', c.name)
    if (!next || next === c.name) return
    const reason = window.prompt('Grund', '')
    if (reason === null) return
    if (!reason.trim()) {
      setError('Umbenennen erfordert eine nichtleere Begründung. Bitte erneut versuchen.')
      return
    }
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${c.id}`, { name: next, reason: reason.trim() })
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Umbenennen fehlgeschlagen')
      }
    })
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
        <div><p className="eyebrow">Spielerprofil</p><h1>{player.name} <span className="badge">{player.role}</span></h1>
          {preferredCharacter ? <p><strong className="character-name">{preferredCharacter.name}</strong> · {preferredCharacter.class}{preferredCharacter.main_spec && ` · ${preferredCharacter.main_spec}`} {preferredCharacter.is_main && <span className="badge">Main</span>}</p> : <p className="muted">Noch keine eigenen Charaktere.</p>}
          {player.guild_rank_id != null && <p className="muted">Gildenrang: {ranks.find((r) => r.id === player.guild_rank_id)?.name ?? (rankError || (rankLoading ? 'Wird geladen…' : 'Nicht verfügbar'))}</p>}
        </div>
      </header>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}

      <div className="grid">
        <EnhancePointsCard points={points} showHistory />

        <section className="card">
          <h2>Armor Tokens</h2>
          <p className="stat">{tokens?.total ?? '—'}</p>
          <div className="table-scroll"><table className="table small">
            <thead><tr><th>Tokens</th><th>Buchung / Begründung</th><th>Raid</th></tr></thead>
            <tbody>
              {tokens?.transactions.map((t) => (
                <tr key={t.id}>
                  <td className={t.amount > 0 ? 'positive' : t.amount < 0 ? 'negative' : 'muted'}>{t.amount > 0 ? `+${t.amount}` : t.amount}</td>
                  <td><strong>{t.amount < 0 ? 'Tokenkorrektur' : 'Tokenbuchung'}</strong><div className="muted">{t.reason}</div><small className="muted">Akteur #{t.actor_id} · {new Date(t.created_at).toLocaleString('de-DE')}</small></td>
                  <td>{t.raid_id && <Link to={`/raids/${t.raid_id}`}>Raid #{t.raid_id}</Link>}</td>
                </tr>
              ))}
              {tokens && tokens.transactions.length === 0 && <tr><td colSpan={3} className="muted">Noch keine Tokenbuchungen.</td></tr>}
            </tbody>
          </table></div>
        </section>
      </div>

      <section className="card">
        <h2>Charaktere</h2>
        <div className="table-scroll"><table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Klasse</th>
              <th>Main-Spec</th>
              <th>Main</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {ownedCharacters.map((c) => (
              <tr key={c.id} className={characterClassName(c.class)}>
                <td className="character-name">{c.name}</td>
                <td>{c.class}</td>
                <td>{c.main_spec || '—'}</td>
                <td>{c.is_main ? '★' : ''}</td>
                <td className="actions">
                  {isAdmin && (
                    <>
                      <button disabled={pending} onClick={() => toggleMain(c)}>Main umschalten</button>
                      <button disabled={pending} onClick={() => renameCharacter(c)}>Umbenennen</button>
                    </>
                  )}
                  <button onClick={() => showHistory(c)}>Historie</button>
                </td>
              </tr>
            ))}
            {ownedCharacters.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
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
          <form className="inline-form" onSubmit={createCharacter}>
            <input aria-label="Charaktername" placeholder="Name" value={charName} onChange={(e) => setCharName(e.target.value)} required />
            <select aria-label="Charakterklasse" value={charClass} onChange={(e) => setCharClass(e.target.value)}>
              {CLASSES.map((cl) => (
                <option key={cl} value={cl}>
                  {cl}
                </option>
              ))}
            </select>
            <input aria-label="Main-Spec" placeholder="Main-Spec" value={charMainSpec} onChange={(e) => setCharMainSpec(e.target.value)} />
            <label className="check">
              <input type="checkbox" checked={charIsMain} onChange={(e) => setCharIsMain(e.target.checked)} /> Main
            </label>
            <button type="submit" disabled={pending}>Anlegen</button>
          </form>
        )}
      </section>

      <section className="card">
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
        <section className="card">
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
