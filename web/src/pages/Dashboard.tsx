import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import type { Attendance, Player, PointsResponse, Poll, Raid, Vote } from '../types'
import { Citadel } from '../components/Citadel'
import { EnhancePointsCard } from '../components/EnhancePointsCard'

export function DashboardPage() {
  const { player } = useAuth()
  const [raids, setRaids] = useState<Raid[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [polls, setPolls] = useState<Poll[]>([])
  const [votes, setVotes] = useState<Record<number, Vote[]>>({})
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [attendance, setAttendance] = useState<Attendance[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [attendanceError, setAttendanceError] = useState('')
  const [pointsError, setPointsError] = useState('')
  const [votesError, setVotesError] = useState('')
  const [now] = useState(() => Date.now())
  const upcoming = raids.filter((r) => !r.archived_at && (r.status === 'PLANNED' || r.status === 'OPEN') && r.scheduled_start && Date.parse(r.scheduled_start) > now)
    .sort((a, b) => Date.parse(a.scheduled_start!) - Date.parse(b.scheduled_start!) || a.id - b.id)
  const nextRaid = upcoming[0]
  const history = raids.filter((r) => r.archived_at).sort((a, b) => Date.parse(b.archived_at!) - Date.parse(a.archived_at!) || b.id - a.id).slice(0, 5)

  useEffect(() => {
    let active = true
    Promise.all([api.get<Raid[]>('/api/v1/raids?include_archived=true'), api.get<Player[]>('/api/v1/players'), api.get<Poll[]>('/api/v1/polls')])
      .then(([rs, ps, polls]) => { if (active) { setRaids(rs); setPlayers(ps); setPolls(polls.filter((p) => p.status === 'OPEN')) } })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Übersicht konnte nicht geladen werden.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  useEffect(() => {
    let active = true
    if (player) api.get<PointsResponse>(`/api/v1/players/${player.id}/points`)
      .then((pts) => { if (active) setPoints(pts) })
      .catch((err) => { if (active) setPointsError(err instanceof ApiError ? err.message : 'EP konnten nicht geladen werden.') })
    return () => { active = false }
  }, [player])

  useEffect(() => {
    let active = true
    setAttendance(null)
    setAttendanceError('')
    if (nextRaid) api.get<Attendance[]>(`/api/v1/raids/${nextRaid.id}/attendance`)
      .then((rows) => { if (active) setAttendance(rows) })
      .catch((err) => { if (active) setAttendanceError(err instanceof ApiError ? err.message : 'Aufstellung konnte nicht geladen werden.') })
    return () => { active = false }
  }, [nextRaid])

  useEffect(() => {
    let active = true
    Promise.all(polls.map(async (poll) => [poll.id, await api.get<Vote[]>(`/api/v1/polls/${poll.id}/votes`)] as const))
      .then((rows) => { if (active) setVotes(Object.fromEntries(rows)) })
      .catch((err) => { if (active) setVotesError(err instanceof ApiError ? err.message : 'Stimmen konnten nicht geladen werden.') })
    return () => { active = false }
  }, [polls])

  return <div className="dashboard">
    <div className="page-heading"><div><h1>Gildenübersicht</h1></div><Link className="text-link" to="/raids">Alle Raids ansehen →</Link></div>
    {error && <p className="error" role="alert">{error}</p>}
    <section className="raid-banner">
      <Citadel />
      <div className="raid-banner-content"><p className="eyebrow">Nächster geplanter Raid</p>
        {loading ? <p role="status" className="muted">Raidplanung wird geladen…</p> : nextRaid ? <>
          <div className="actions"><span className="badge info">{nextRaid.status === 'OPEN' ? 'Offen' : 'Geplant'}</span>{nextRaid.raid_size && <span className="badge">{nextRaid.raid_size} Spieler</span>}<span className="muted">{new Date(nextRaid.scheduled_start!).toLocaleString('de-DE', { weekday: 'long', day: '2-digit', month: 'long', hour: '2-digit', minute: '2-digit' })}</span></div>
          <h2>{nextRaid.name}</h2><p className="muted">Raidleitung: {players.find((p) => p.id === nextRaid.raid_leader_id)?.name ?? `Spieler #${nextRaid.raid_leader_id}`}</p>
          {attendanceError ? <p className="error" role="alert">{attendanceError}</p> : attendance ? <div className="composition" aria-label="Gespeicherte Raidaufstellung"><span><strong>{attendance.length}</strong> Aufgestellt</span><span><strong>{attendance.filter((a) => a.role === 'TANK').length}</strong> Tanks</span><span><strong>{attendance.filter((a) => a.role === 'HEAL').length}</strong> Heiler</span><span><strong>{attendance.filter((a) => a.role === 'DD').length}</strong> DD</span></div> : <p className="muted" role="status">Aufstellung wird geladen…</p>}
          <Link className="button-link primary" to={`/raids/${nextRaid.id}`}>Raid öffnen →</Link>
        </> : <><h2>{error ? 'Raidplanung nicht verfügbar' : 'Kein zukünftiger Raid geplant'}</h2><p className="muted">{error ? 'Bitte lade die Seite erneut.' : 'Hier erscheint der nächste gespeicherte Termin. Offene Raids und die Historie findest du in der Raidübersicht.'}</p><Link className="button-link" to="/raids">Zur Raidplanung →</Link></>}
      </div>
    </section>
    <div className="dashboard-columns">
      <div className="dashboard-points">
        {pointsError && <p className="error" role="alert">{pointsError}</p>}<EnhancePointsCard points={points} />
        {player && <p className="profile-link"><Link to={`/players/${player.id}`}>Mein Profil & EP-Historie →</Link></p>}
      </div>
      <section className="card dashboard-agenda"><div className="section-heading"><h2>Raidagenda</h2><span className="eyebrow">Gespeicherte Termine</span></div>
        {upcoming.slice(0, 5).map((r) => <div className="agenda-row" key={r.id}><time dateTime={r.scheduled_start!}><strong>{new Date(r.scheduled_start!).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' })}</strong><span>{new Date(r.scheduled_start!).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })}</span></time><Link to={`/raids/${r.id}`}>{r.name}</Link><span className="badge">{r.status === 'OPEN' ? 'Offen' : 'Geplant'}</span></div>)}
        {!loading && !error && upcoming.length === 0 && <p className="muted">Keine zukünftigen Termine gespeichert.</p>}
      </section>
      <section className="card dashboard-polls"><div className="section-heading"><h2>Offene Umfragen</h2><Link to="/polls">Alle →</Link></div>
        {votesError && <p className="error" role="alert">{votesError}</p>}
        {polls.map((p) => <div className="poll-preview" key={p.id}><h3>{p.title}</h3>{p.description && <p className="muted">{p.description}</p>}<div className="section-heading"><span className="muted">{votes[p.id] ? `${votes[p.id].length} abgegebene Stimmen` : 'Stimmen werden geladen…'}</span><Link to="/polls">Zur Abstimmung →</Link></div></div>)}
        {!loading && !error && polls.length === 0 && <p className="muted">Aktuell keine offenen Umfragen.</p>}
      </section>
      <section className="card dashboard-history"><div className="section-heading"><h2>Zuletzt archiviert</h2><Link to="/raids">Historie →</Link></div>
        {history.map((r) => <div className="archive-row" key={r.id}><Link to={`/raids/${r.id}`}>{r.name}</Link><time dateTime={r.archived_at!}>{new Date(r.archived_at!).toLocaleDateString('de-DE')}</time><span className="badge ok">Archiviert</span></div>)}
        {!loading && !error && history.length === 0 && <p className="muted">Noch keine archivierten Raids.</p>}
      </section>
    </div>
  </div>
}
