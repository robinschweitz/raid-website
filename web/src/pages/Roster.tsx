import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { atLeast, type GuildRank, type RaidGroup } from '../types'

export function RosterPage() {
  const { player: me } = useAuth()
  const [ranks, setRanks] = useState<GuildRank[]>([])
  const [groups, setGroups] = useState<RaidGroup[]>([])
  const [error, setError] = useState('')
  const [rankName, setRankName] = useState('')
  const [groupName, setGroupName] = useState('')
  const [groupColor, setGroupColor] = useState('#888888')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false

  const load = () => {
    setError('')
    Promise.all([api.get<GuildRank[]>('/api/v1/guild-ranks'), api.get<RaidGroup[]>('/api/v1/raid-groups')])
      .then(([r, g]) => {
        setRanks(r)
        setGroups(g)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const createRank = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<GuildRank>('/api/v1/guild-ranks', { name: rankName })
      setRankName('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
    }
  }

  const createGroup = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    try {
      await api.post<RaidGroup>('/api/v1/raid-groups', { name: groupName, color: groupColor })
      setGroupName('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
    }
  }

  const toggleRank = async (r: GuildRank) => {
    setError('')
    try {
      await api.patch<GuildRank>(`/api/v1/guild-ranks/${r.id}`, { active: !r.active })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
    }
  }

  const toggleGroup = async (g: RaidGroup) => {
    setError('')
    try {
      await api.patch<RaidGroup>(`/api/v1/raid-groups/${g.id}`, { active: !g.active })
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
    }
  }

  return (
    <div>
      <h1>Roster</h1>
      {error && <p className="error">{error}</p>}

      <section className="card">
        <h2>Gildenränge</h2>
        <ul className="plain-list">
          {ranks.map((r) => (
            <li key={r.id}>
              <span className={r.active ? '' : 'muted'}>
                {r.name} (#{r.sort_order})
              </span>
              {isAdmin && <button onClick={() => toggleRank(r)}>{r.active ? 'deaktivieren' : 'aktivieren'}</button>}
            </li>
          ))}
          {ranks.length === 0 && <li className="muted">Keine Ränge.</li>}
        </ul>
        {isAdmin && (
          <form className="inline-form" onSubmit={createRank}>
            <input placeholder="Rangname" value={rankName} onChange={(e) => setRankName(e.target.value)} required />
            <button type="submit">Anlegen</button>
          </form>
        )}
      </section>

      <section className="card">
        <h2>Raidgruppen</h2>
        <ul className="plain-list">
          {groups.map((g) => (
            <li key={g.id}>
              <span className="swatch" style={{ background: g.color }} />
              <span className={g.active ? '' : 'muted'}>
                {g.name} (#{g.sort_order})
              </span>
              {isAdmin && <button onClick={() => toggleGroup(g)}>{g.active ? 'deaktivieren' : 'aktivieren'}</button>}
            </li>
          ))}
          {groups.length === 0 && <li className="muted">Keine Gruppen.</li>}
        </ul>
        {isAdmin && (
          <form className="inline-form" onSubmit={createGroup}>
            <input placeholder="Gruppenname" value={groupName} onChange={(e) => setGroupName(e.target.value)} required />
            <input type="color" value={groupColor} onChange={(e) => setGroupColor(e.target.value)} />
            <button type="submit">Anlegen</button>
          </form>
        )}
      </section>
    </div>
  )
}
