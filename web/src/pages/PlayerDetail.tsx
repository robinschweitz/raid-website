import { useEffect, useState, type FormEvent } from 'react'
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
  type Role,
  type RollResult,
  type TokensResponse,
} from '../types'

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
  const { player: me } = useAuth()
  const [player, setPlayer] = useState<Player | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [tokens, setTokens] = useState<TokensResponse | null>(null)
  const [locks, setLocks] = useState<ItemLock[]>([])
  const [history, setHistory] = useState<NameHistory[] | null>(null)
  const [historyChar, setHistoryChar] = useState<number | null>(null)
  const [roll, setRoll] = useState<RollResult | null>(null)
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
  const isMember = me ? atLeast(me.role, 'MEMBER') : false

  const load = () => {
    setError('')
    Promise.all([
      api.get<Player>(`/api/v1/players/${playerID}`),
      api.get<Character[]>(`/api/v1/players/${playerID}/characters`),
      api.get<PointsResponse>(`/api/v1/players/${playerID}/points`),
      api.get<TokensResponse>(`/api/v1/players/${playerID}/tokens`),
      api.get<ItemLock[]>(`/api/v1/players/${playerID}/locks`),
    ])
      .then(([p, cs, pts, tks, lk]) => {
        setPlayer(p)
        setCharacters(cs)
        setPoints(pts)
        setTokens(tks)
        setLocks(lk)
        setAccUser(p.username ?? '')
        setAccRole(p.role)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [playerID])

  const createCharacter = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<Character>(`/api/v1/players/${playerID}/characters`, {
        name: charName,
        class: charClass,
        main_spec: charMainSpec,
        is_main: charIsMain,
      })
      setCharName('')
      setCharMainSpec('')
      setCharIsMain(false)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Charakter anlegen fehlgeschlagen')
    }
  }

  const toggleMain = async (c: Character) => {
    setError('')
    try {
      await api.patch<Character>(`/api/v1/characters/${c.id}`, { is_main: !c.is_main, reason: 'main flag change' })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
    }
  }

  const renameCharacter = async (c: Character) => {
    const next = window.prompt('Neuer Name', c.name)
    if (!next || next === c.name) return
    const reason = window.prompt('Grund', '')
    if (reason === null) return
    setError('')
    try {
      await api.patch<Character>(`/api/v1/characters/${c.id}`, { name: next, reason })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Umbenennen fehlgeschlagen')
    }
  }

  const showHistory = async (c: Character) => {
    setError('')
    try {
      const hs = await api.get<NameHistory[]>(`/api/v1/characters/${c.id}/name-history`)
      setHistory(hs)
      setHistoryChar(c.id)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Historie fehlgeschlagen')
    }
  }

  const doRoll = async () => {
    setError('')
    try {
      setRoll(await api.post<RollResult>(`/api/v1/players/${playerID}/points/roll`))
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Würfeln fehlgeschlagen')
    }
  }

  const createLock = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<ItemLock>(`/api/v1/players/${playerID}/locks`, {
        raid_id: Number(lockRaid),
        item_id: lockItemID,
        item_name: lockItem,
        boss: lockBoss,
      })
      setLockRaid('')
      setLockItem('')
      setLockItemID('')
      setLockBoss('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Lock anlegen fehlgeschlagen')
    }
  }

  const cancelLock = async (lock: ItemLock) => {
    setError('')
    try {
      await api.post<ItemLock>(`/api/v1/players/${playerID}/locks/${lock.id}/cancel`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Lock stornieren fehlgeschlagen')
    }
  }

  const setAccount = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.put<Player>(`/api/v1/players/${playerID}/account`, {
        username: accUser,
        password: accPass,
        role: accRole,
      })
      setAccPass('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Konto speichern fehlgeschlagen')
    }
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
      <h1>
        {player.name} <span className="badge">{player.role}</span>
      </h1>
      {error && <p className="error">{error}</p>}

      <div className="grid">
        <section className="card">
          <h2>Punkte</h2>
          <p className="stat">{points?.balance ?? '—'}</p>
          {isMember && (
            <button onClick={doRoll} disabled={(points?.balance ?? 0) < 1}>
              Enhance würfeln
            </button>
          )}
          {roll && (
            <p className="muted">
              Kosten {roll.cost} · Wurf {roll.roll} ({roll.min}–{roll.max}) · neu {roll.balance_after}
            </p>
          )}
          <table className="table small">
            <tbody>
              {points?.transactions.map((t) => (
                <tr key={t.id}>
                  <td>{t.amount > 0 ? `+${t.amount}` : t.amount}</td>
                  <td>{t.reason || t.type}</td>
                  <td>{new Date(t.created_at).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <section className="card">
          <h2>Tokens</h2>
          <p className="stat">{tokens?.total ?? '—'}</p>
          <table className="table small">
            <tbody>
              {tokens?.transactions.map((t) => (
                <tr key={t.id}>
                  <td>{t.amount > 0 ? `+${t.amount}` : t.amount}</td>
                  <td>{t.reason || t.type}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      </div>

      <section className="card">
        <h2>Charaktere</h2>
        <table className="table">
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
            {characters.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td>{c.class}</td>
                <td>{c.main_spec || '—'}</td>
                <td>{c.is_main ? '★' : ''}</td>
                <td className="actions">
                  {isAdmin && (
                    <>
                      <button onClick={() => toggleMain(c)}>Main umschalten</button>
                      <button onClick={() => renameCharacter(c)}>Umbenennen</button>
                    </>
                  )}
                  <button onClick={() => showHistory(c)}>Historie</button>
                </td>
              </tr>
            ))}
            {characters.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Keine Charaktere.
                </td>
              </tr>
            )}
          </tbody>
        </table>
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
            <input placeholder="Name" value={charName} onChange={(e) => setCharName(e.target.value)} required />
            <select value={charClass} onChange={(e) => setCharClass(e.target.value)}>
              {CLASSES.map((cl) => (
                <option key={cl} value={cl}>
                  {cl}
                </option>
              ))}
            </select>
            <input placeholder="Main-Spec" value={charMainSpec} onChange={(e) => setCharMainSpec(e.target.value)} />
            <label className="check">
              <input type="checkbox" checked={charIsMain} onChange={(e) => setCharIsMain(e.target.checked)} /> Main
            </label>
            <button type="submit">Anlegen</button>
          </form>
        )}
      </section>

      <section className="card">
        <h2>Item-Locks</h2>
        <table className="table">
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
                <td>#{l.raid_id}</td>
                <td>{l.status}</td>
                <td>
                  {l.status === 'ACTIVE' && <button onClick={() => cancelLock(l)}>Stornieren</button>}
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
        </table>
        {isMember && (
          <form className="inline-form" onSubmit={createLock}>
            <input placeholder="Raid-ID" value={lockRaid} onChange={(e) => setLockRaid(e.target.value)} required />
            <input placeholder="Item" value={lockItem} onChange={(e) => setLockItem(e.target.value)} required />
            <input placeholder="Item-ID" value={lockItemID} onChange={(e) => setLockItemID(e.target.value)} />
            <input placeholder="Boss" value={lockBoss} onChange={(e) => setLockBoss(e.target.value)} />
            <button type="submit">Lock anlegen</button>
          </form>
        )}
      </section>

      {isAdmin && (
        <section className="card">
          <h2>Konto</h2>
          <form className="inline-form" onSubmit={setAccount}>
            <input placeholder="Benutzername" value={accUser} onChange={(e) => setAccUser(e.target.value)} required />
            <input
              type="password"
              placeholder="Passwort (leer = unverändert)"
              value={accPass}
              onChange={(e) => setAccPass(e.target.value)}
            />
            <select value={accRole} onChange={(e) => setAccRole(e.target.value as Role)}>
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
            <button type="submit">Speichern</button>
          </form>
        </section>
      )}
    </div>
  )
}
