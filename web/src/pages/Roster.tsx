import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { atLeast, type GuildRank, type RaidGroup } from '../types'
import { useSubmission } from '../useSubmission'
import { Link } from '../router'

export function RosterPage() {
  const { pending, submit, isActive } = useSubmission()
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
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.post<GuildRank>('/api/v1/guild-ranks', { name: rankName })
        if (!isActive()) return
        setRankName('')
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  const createGroup = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.post<RaidGroup>('/api/v1/raid-groups', { name: groupName, color: groupColor })
        if (!isActive()) return
        setGroupName('')
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  const toggleRank = async (r: GuildRank) => {
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<GuildRank>(`/api/v1/guild-ranks/${r.id}`, { active: !r.active })
        if (!isActive()) return
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
      }
    })
  }

  const toggleGroup = async (g: RaidGroup) => {
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<RaidGroup>(`/api/v1/raid-groups/${g.id}`, { active: !g.active })
        if (!isActive()) return
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
      }
    })
  }

  return (
    <div>
      <p className="eyebrow">Administration</p><h1>Gildenkonfiguration</h1>
      {isAdmin && <div className="actions"><Link to="/raid-series">Raidserien konfigurieren</Link><Link to="/loot-catalog">Beutekatalog verwalten</Link></div>}
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}

      <section className="card">
        <h2>Gildenränge</h2>
        <ul className="plain-list">
          {ranks.map((r) => (
            <li key={r.id}>
              <span className={r.active ? '' : 'muted'}>
                {r.name} (#{r.sort_order})
              </span>
              {isAdmin && <button disabled={pending} onClick={() => toggleRank(r)}>{r.active ? 'deaktivieren' : 'aktivieren'}</button>}
            </li>
          ))}
          {ranks.length === 0 && <li className="muted">Keine Ränge.</li>}
        </ul>
        {isAdmin && (
          <form className="inline-form" onSubmit={createRank}>
            <input aria-label="Rangname" placeholder="Rangname" value={rankName} onChange={(e) => setRankName(e.target.value)} required />
            <button type="submit" disabled={pending}>Anlegen</button>
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
              {isAdmin && <button disabled={pending} onClick={() => toggleGroup(g)}>{g.active ? 'deaktivieren' : 'aktivieren'}</button>}
            </li>
          ))}
          {groups.length === 0 && <li className="muted">Keine Gruppen.</li>}
        </ul>
        {isAdmin && (
          <form className="inline-form" onSubmit={createGroup}>
            <input aria-label="Gruppenname" placeholder="Gruppenname" value={groupName} onChange={(e) => setGroupName(e.target.value)} required />
            <input aria-label="Gruppenfarbe" type="color" value={groupColor} onChange={(e) => setGroupColor(e.target.value)} />
            <button type="submit" disabled={pending}>Anlegen</button>
          </form>
        )}
      </section>
    </div>
  )
}
