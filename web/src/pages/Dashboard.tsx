import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link, navigate } from '../router'
import { atLeast, type Raid } from '../types'
import { useSubmission } from '../useSubmission'

function statusClass(status: Raid['status']): string {
  switch (status) {
    case 'COMPLETED':
      return 'badge ok'
    case 'CANCELLED':
      return 'badge bad'
    case 'OPEN':
      return 'badge info'
    default:
      return 'badge'
  }
}

export function DashboardPage() {
  const { pending, submit, isActive } = useSubmission()
  const { player } = useAuth()
  const [raids, setRaids] = useState<Raid[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [scheduled, setScheduled] = useState('')

  const canManage = player ? atLeast(player.role, 'RAIDASSISTENT') : false

  const load = () => {
    api
      .get<Raid[]>('/api/v1/raids')
      .then(setRaids)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!player || !canManage) return
    await submit(async () => {
      setError('')
      try {
        const created = await api.post<Raid>('/api/v1/raids', {
          name,
          raid_leader_id: player.id,
          scheduled_start: scheduled ? new Date(scheduled).toISOString() : null,
        })
        if (!isActive()) return
        setName('')
        setScheduled('')
        navigate(`/raids/${created.id}`)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  return (
    <div>
      <h1>Raids</h1>
      {error && <p className="error">{error}</p>}
      <table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Start</th>
            <th>Notizen</th>
          </tr>
        </thead>
        <tbody>
          {raids.map((raid) => (
            <tr key={raid.id}>
              <td>
                <Link to={`/raids/${raid.id}`}>{raid.name}</Link>
              </td>
              <td>
                <span className={statusClass(raid.status)}>{raid.status}</span>
              </td>
              <td>{raid.scheduled_start ? new Date(raid.scheduled_start).toLocaleString() : '—'}</td>
              <td>{raid.notes}</td>
            </tr>
          ))}
          {raids.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                Noch keine Raids.
              </td>
            </tr>
          )}
        </tbody>
      </table>

      {canManage && (
        <form className="card inline-form" onSubmit={onCreate}>
          <h2>Neuer Raid</h2>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Start
            <input type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
          </label>
          <button type="submit" disabled={pending}>{pending ? 'Anlegen…' : 'Anlegen'}</button>
        </form>
      )}
    </div>
  )
}
