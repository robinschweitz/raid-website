import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Player, type Role } from '../types'

const ROLES: Role[] = ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']

export function PlayersPage() {
  const { player: me } = useAuth()
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [role, setRole] = useState<Role>('GUEST')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false

  const load = () => {
    api
      .get<Player[]>('/api/v1/players')
      .then(setPlayers)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<Player>('/api/v1/players', { name, role })
      setName('')
      setRole('GUEST')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
    }
  }

  return (
    <div>
      <h1>Spieler</h1>
      {error && <p className="error">{error}</p>}
      <table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Rolle</th>
            <th>Login</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {players.map((p) => (
            <tr key={p.id}>
              <td>
                <Link to={`/players/${p.id}`}>{p.name}</Link>
              </td>
              <td>
                <span className="badge">{p.role}</span>
              </td>
              <td>{p.username ?? '—'}</td>
              <td>{p.active ? 'aktiv' : 'inaktiv'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {isAdmin && (
        <form className="card inline-form" onSubmit={onCreate}>
          <h2>Neuer Spieler</h2>
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
          <button type="submit">Anlegen</button>
        </form>
      )}
    </div>
  )
}
