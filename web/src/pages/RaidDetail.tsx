import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Attendance, type AttendanceRole, type Character, type LootAward, type Player, type Raid } from '../types'

interface Props {
  raidID: number
}

export function RaidDetailPage({ raidID }: Props) {
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<LootAward[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')

  const [attPlayer, setAttPlayer] = useState('')
  const [attCharacter, setAttCharacter] = useState('')
  const [attRole, setAttRole] = useState<AttendanceRole>('DD')
  const [characters, setCharacters] = useState<Character[]>([])

  const [lootItem, setLootItem] = useState('')
  const [lootItemID, setLootItemID] = useState('')
  const [lootBoss, setLootBoss] = useState('')
  const [lootPlayer, setLootPlayer] = useState('')
  const [lootMain, setLootMain] = useState(true)

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false

  const load = () => {
    setError('')
    Promise.all([
      api.get<Raid>(`/api/v1/raids/${raidID}`),
      api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
      api.get<LootAward[]>(`/api/v1/raids/${raidID}/loot`),
      api.get<Player[]>('/api/v1/players'),
    ])
      .then(([r, a, l, p]) => {
        setRaid(r)
        setAttendance(a)
        setLoot(l)
        setPlayers(p)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [raidID])

  useEffect(() => {
    if (!attPlayer) {
      setCharacters([])
      return
    }
    api
      .get<Character[]>(`/api/v1/players/${attPlayer}/characters`)
      .then((cs) => {
        setCharacters(cs)
        setAttCharacter(cs.length > 0 ? String(cs[0].id) : '')
      })
      .catch(() => setCharacters([]))
  }, [attPlayer])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`

  const addAttendance = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<Attendance>(`/api/v1/raids/${raidID}/attendance`, {
        player_id: Number(attPlayer),
        character_id: Number(attCharacter),
        role: attRole,
      })
      setAttPlayer('')
      setAttCharacter('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Teilnahme fehlgeschlagen')
    }
  }

  const addLoot = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<LootAward>(`/api/v1/raids/${raidID}/loot`, {
        boss: lootBoss,
        item_id: lootItemID,
        item_name: lootItem,
        awarded_to_player_id: Number(lootPlayer),
        is_main_spec: lootMain,
      })
      setLootItem('')
      setLootItemID('')
      setLootBoss('')
      setLootPlayer('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Loot fehlgeschlagen')
    }
  }

  const action = async (name: 'complete' | 'cancel' | 'archive') => {
    setError('')
    try {
      await api.post<Raid>(`/api/v1/raids/${raidID}/${name}`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Aktion fehlgeschlagen')
    }
  }

  if (!raid) {
    return (
      <div>
        <p>
          <Link to="/">← Raids</Link>
        </p>
        {error ? <p className="error">{error}</p> : <p className="muted">Lade…</p>}
      </div>
    )
  }

  return (
    <div>
      <p>
        <Link to="/">← Raids</Link>
      </p>
      <h1>
        {raid.name} <span className="badge">{raid.status}</span>
      </h1>
      {error && <p className="error">{error}</p>}

      {canManage && (
        <div className="actions">
          {raid.status !== 'COMPLETED' && (
            <button onClick={() => action('complete')}>Abschließen</button>
          )}
          {raid.status !== 'CANCELLED' && <button onClick={() => action('cancel')}>Absagen</button>}
          {!raid.archived_at && <button onClick={() => action('archive')}>Archivieren</button>}
        </div>
      )}

      <section className="card">
        <h2>Teilnahme ({attendance.length})</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Spieler</th>
              <th>Charakter</th>
              <th>Rolle</th>
            </tr>
          </thead>
          <tbody>
            {attendance.map((a) => (
              <tr key={a.id}>
                <td>{playerName(a.player_id)}</td>
                <td>#{a.character_id}</td>
                <td>{a.role}</td>
              </tr>
            ))}
            {attendance.length === 0 && (
              <tr>
                <td colSpan={3} className="muted">
                  Keine Einträge.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {canManage && (
          <form className="inline-form" onSubmit={addAttendance}>
            <select value={attPlayer} onChange={(e) => setAttPlayer(e.target.value)} required>
              <option value="">Spieler…</option>
              {players.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <select value={attCharacter} onChange={(e) => setAttCharacter(e.target.value)} required>
              <option value="">Charakter…</option>
              {characters.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
            <select value={attRole} onChange={(e) => setAttRole(e.target.value as AttendanceRole)}>
              <option value="TANK">Tank</option>
              <option value="HEAL">Heal</option>
              <option value="DD">DD</option>
            </select>
            <button type="submit">Hinzufügen</button>
          </form>
        )}
      </section>

      <section className="card">
        <h2>Loot ({loot.length})</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Item</th>
              <th>Boss</th>
              <th>Spieler</th>
              <th>Spec</th>
            </tr>
          </thead>
          <tbody>
            {loot.map((l) => (
              <tr key={l.id}>
                <td>{l.item_name}</td>
                <td>{l.boss || '—'}</td>
                <td>{playerName(l.awarded_to_player_id)}</td>
                <td>{l.is_main_spec ? 'Main' : 'Second'}</td>
              </tr>
            ))}
            {loot.length === 0 && (
              <tr>
                <td colSpan={4} className="muted">
                  Kein Loot.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {canManage && (
          <form className="inline-form" onSubmit={addLoot}>
            <input placeholder="Item" value={lootItem} onChange={(e) => setLootItem(e.target.value)} required />
            <input placeholder="Item-ID" value={lootItemID} onChange={(e) => setLootItemID(e.target.value)} />
            <input placeholder="Boss" value={lootBoss} onChange={(e) => setLootBoss(e.target.value)} />
            <select value={lootPlayer} onChange={(e) => setLootPlayer(e.target.value)} required>
              <option value="">Spieler…</option>
              {players.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            <label className="check">
              <input type="checkbox" checked={lootMain} onChange={(e) => setLootMain(e.target.checked)} /> Main-Spec
            </label>
            <button type="submit">Vergeben</button>
          </form>
        )}
      </section>
    </div>
  )
}
