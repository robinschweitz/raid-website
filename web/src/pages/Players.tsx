import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Player, type PlayerSummary, type Role } from '../types'
import { useSubmission } from '../useSubmission'

const ROLES: Role[] = ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']

export function PlayersPage() {
  const { pending, submit, isActive } = useSubmission()
  const { player: me } = useAuth()
  const [players, setPlayers] = useState<PlayerSummary[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [search, setSearch] = useState('')
  const [role, setRole] = useState<Role>('GUEST')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false

  const load = () => {
    api
      .get<PlayerSummary[]>('/api/v1/players')
      .then(setPlayers)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Player>('/api/v1/players', { name, role })
        if (!isActive()) return
        setName('')
        setRole('GUEST')
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  const visiblePlayers = players
    .filter((p) => p.name.toLowerCase().includes(search.trim().toLowerCase()))
    .sort((a, b) => a.name.localeCompare(b.name, 'de', { numeric: true, sensitivity: 'base' }))

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Unsere Gemeinschaft</p><h1>Spieler</h1></div>{isAdmin && <Link to="/roster">Gildenkonfiguration →</Link>}</div>
      {error && <p className="error">{error}</p>}
      {isAdmin && (
        <details className="card editor-disclosure"><summary>Neuer Spieler</summary>
        <form className="card inline-form" onSubmit={onCreate}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Rolle
            <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={pending}>{pending ? 'Anlegen…' : 'Anlegen'}</button>
        </form>
        </details>
      )}
      {players.length > 0 && <div className="roster-filters player-search">
        <label className="filter-search">Spieler suchen<input type="search" value={search} placeholder="Name" onChange={(e) => setSearch(e.target.value)} /></label>
        <span className="muted" role="status">{visiblePlayers.length} von {players.length} Spielern</span>
      </div>}
      <div className="card table-scroll"><table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {visiblePlayers.map((p) => (
            <tr key={p.id}>
              <td>
                <Link to={`/players/${p.id}`}>{p.name}</Link>
              </td>
              <td>{p.active ? 'aktiv' : 'inaktiv'}</td>
            </tr>
          ))}
          {players.length === 0 && <tr><td colSpan={2} className="muted">Keine Spieler geladen.</td></tr>}
          {players.length > 0 && visiblePlayers.length === 0 && <tr><td colSpan={2} className="muted">Kein Spieler mit diesem Namen.</td></tr>}
        </tbody>
      </table></div>

    </div>
  )
}
