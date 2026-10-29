import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import type { Attendance, BossKill, Character, Player, PointsResponse, Poll, Raid, Vote } from '../types'
import { RaidBanner } from '../components/RaidBanner'
// import { EnhancePointsCard } from '../components/EnhancePointsCard'
import { AttendanceRing } from '../components/AttendanceRing'
import { BossProgression } from '../components/BossProgression'
import { RaidCompositionCard } from '../components/RaidCompositionCard'
import { GuildNotifications } from '../components/GuildNotifications'
import { RaidNews } from '../components/RaidNews'
import { PlayerProfilePanel } from '../components/PlayerProfilePanel'
import { MyRaidsPanel } from '../components/MyRaidsPanel'
import { atLeast } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'


export function DashboardPage() {
  const { player } = useAuth()
  const [raids, setRaids] = useState<Raid[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [polls, setPolls] = useState<Poll[]>([])
  const [votes, setVotes] = useState<Record<number, Vote[]>>({})
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [attendance, setAttendance] = useState<Attendance[] | null>(null)
  const [kills, setKills] = useState<BossKill[] | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [attendanceError, setAttendanceError] = useState('')
  const [pointsError, setPointsError] = useState('')
  const [votesError, setVotesError] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const scheduledRaids = raids.filter((r) => !r.archived_at && (r.status === 'PLANNED' || r.status === 'OPEN') && r.scheduled_start && Number.isFinite(Date.parse(r.scheduled_start)))
    .sort((a, b) => Date.parse(a.scheduled_start!) - Date.parse(b.scheduled_start!) || a.id - b.id)
  const nextRaid = scheduledRaids[0]
  const isRunning = !!nextRaid && Date.parse(nextRaid.scheduled_start!) <= now
  const history = raids.filter((r) => r.archived_at).sort((a, b) => Date.parse(b.archived_at!) - Date.parse(a.archived_at!) || b.id - a.id).slice(0, 10)

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

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
    if (player) {
      api.get<PointsResponse>(`/api/v1/players/${player.id}/points`)
        .then((pts) => { if (active) setPoints(pts) })
        .catch((err) => { if (active) setPointsError(err instanceof ApiError ? err.message : 'EP konnten nicht geladen werden.') })
      api.get<Character[]>(`/api/v1/players/${player.id}/characters`)
        .then((chars) => { if (active) setCharacters(chars) })
        .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Charaktere konnten nicht geladen werden.') })
    }
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

  // Kills are readable only when signed in; without them the progress shows no status.
  const nextRaidID = nextRaid?.id
  useEffect(() => {
    let active = true
    setKills(null)
    if (player && nextRaidID !== undefined) api.get<BossKill[]>(`/api/v1/raids/${nextRaidID}/kills`)
      .then((rows) => { if (active) setKills(rows) })
      .catch(() => undefined)
    return () => { active = false }
  }, [player, nextRaidID])

  useEffect(() => {
    let active = true
    if (!player) return
    Promise.all(polls.map(async (poll) => [poll.id, await api.get<Vote[]>(`/api/v1/polls/${poll.id}/votes`)] as const))
      .then((rows) => { if (active) setVotes(Object.fromEntries(rows)) })
      .catch((err) => { if (active) setVotesError(err instanceof ApiError ? err.message : 'Stimmen konnten nicht geladen werden.') })
    return () => { active = false }
  }, [polls, player])

  const confirmedCount = attendance?.filter((a) => !a.is_bench).length ?? 0
  const requiredPlayers = nextRaid?.raid_size ?? 0
  const openSlots = Math.max(0, requiredPlayers - confirmedCount)
  const overCapacity = Math.max(0, confirmedCount - requiredPlayers)

  return <div className="dashboard">
    <div className="page-heading">
      <div>
        <p className="eybrw">Gilden&uuml;bersicht</p>
        <h1>Bratwurst des Schicksals</h1>
      </div>
      <Link to="/raids">Alle Raids ansehen &rarr;</Link>
    </div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <RaidNews player={player} />

    {player && atLeast(player.role, 'MEMBER') && !loading && <MyRaidsPanel playerID={player.id} raids={raids} characters={characters} />}

    <RaidBanner catalogInstanceId={nextRaid?.catalog_instance_id}>
      <div className="raid-banner-content">
        <p className="eyebrow">{isRunning ? 'Laufender Raid' : 'Nächster geplanter Raid'}</p>
        {loading ? <p role="status" className="muted">Raidplanung wird geladen&hellip;</p> : nextRaid ? <>
          <div className="actions">
            <span className="badge info">{isRunning ? 'Läuft' : nextRaid.status === 'OPEN' ? 'Offen' : 'Geplant'}</span>
            {nextRaid.raid_size && <span className="badge">{nextRaid.raid_size} Spieler</span>}
            <span className="muted">{new Date(nextRaid.scheduled_start!).toLocaleString('de-DE', { weekday: 'long', day: '2-digit', month: 'long', hour: '2-digit', minute: '2-digit' })}</span>
          </div>
          <h2>{nextRaid.name}</h2>
          <p className="muted">Raidleitung: {players.find((p) => p.id === nextRaid.raid_leader_id)?.name ?? `Spieler #${nextRaid.raid_leader_id}`}</p>
          {attendanceError ? <ErrorMessage>{attendanceError}</ErrorMessage> : attendance ? <>
            <div className="composition" aria-label="Gespeicherte Raidaufstellung">
              <span><strong>{attendance.length}</strong> Aufgestellt</span>
              <span><strong>{attendance.filter((a) => a.role === 'TANK').length}</strong> Tanks</span>
              <span><strong>{attendance.filter((a) => a.role === 'HEAL').length}</strong> Heiler</span>
              <span><strong>{attendance.filter((a) => a.role === 'DD').length}</strong> DD</span>
            </div>
          </> : <p className="muted" role="status">Aufstellung wird geladen&hellip;</p>}
          <BossProgression catalogInstanceId={nextRaid.catalog_instance_id} kills={kills} />
          <Link className="button-link primary" to={`/raids/${nextRaid.id}`}>Raid &ouml;ffnen &rarr;</Link>
        </> : <><h2>{error ? 'Raidplanung nicht verfügbar' : 'Kein zukünftiger Raid geplant'}</h2><p className="muted">{error ? 'Bitte lade die Seite erneut.' : 'Hier erscheint der nächste gespeicherte Termin.'}</p><Link className="button-link" to="/raids">Zur Raidplanung &rarr;</Link></>}
      </div>
    </RaidBanner>

    <div className="dashboard-widgets">
      <section className="card widget-composition">
        <div className="section-heading"><h2>Raidaufstellung</h2><span className="eyebrow">{nextRaid?.name ?? 'Kein Raid'}</span></div>
        {attendance ? <RaidCompositionCard attendance={attendance} /> : <p className="muted">Keine Aufstellung verf&uuml;gbar.</p>}
      </section>

      <section className="card widget-attendance">
        <div className="section-heading"><h2>Raid-Auslastung</h2><span className="eyebrow">Aufstellung</span></div>
        {attendance ? <div className="attendance-ring-container">
          <AttendanceRing confirmed={confirmedCount} requiredPlayers={requiredPlayers} />
          <div className="attendance-details">
            <div className="attendance-detail-row"><span className="muted">Aufgestellt</span><strong>{confirmedCount} / {requiredPlayers || "–"}</strong></div>
            <div className="attendance-detail-row"><span className="muted">Ersatzbank</span><strong>{attendance.filter((a) => a.is_bench).length}</strong></div>
            <div className="attendance-detail-row"><span className="muted">Freie Plätze</span><strong>{requiredPlayers ? openSlots : "–"}</strong></div>
            {overCapacity > 0 && <div className="attendance-detail-row"><span className="muted">Überbelegung</span><strong>+{overCapacity}</strong></div>}
          </div>
        </div> : <p className="muted">Keine Daten verf&uuml;gbar.</p>}
      </section>

      <section className="card widget-poll">
        <div className="section-heading"><h2>Aktive Umfrage</h2><Link to="/polls">Alle &rarr;</Link></div>
        {votesError && <ErrorMessage>{votesError}</ErrorMessage>}
        {polls.map((p) => {
          const pollVotes = votes[p.id]
          const choiceCounts: Record<string, number> = {}
          if (pollVotes) pollVotes.forEach((v) => {
            const labels = v.choices?.length ? v.choices : [v.choice]
            labels.forEach((label) => { choiceCounts[label] = (choiceCounts[label] ?? 0) + 1 })
          })
          const totalVotes = pollVotes?.length ?? 0
          return <div className="poll-preview" key={p.id}>
            <h3>{p.title}</h3>
            {p.description && <p className="muted">{p.description}</p>}
            {pollVotes && totalVotes > 0 && Object.entries(choiceCounts).map(([choice, count]) => <div className="poll-option-bar" key={choice}>
              <div className="option-label"><span>{choice}</span><span className="votes">{count} Stimmen</span></div>
              <div className="bar-track"><div className="bar-fill" style={{ width: `${(count / totalVotes) * 100}%` }} /></div>
            </div>)}
            <div className="section-heading"><span className="muted">{!player ? 'Zum Abstimmen anmelden' : pollVotes ? `${totalVotes} abgegebene Stimmen` : 'Stimmen werden geladen…'}</span><Link to="/polls">Zur Abstimmung &rarr;</Link></div>
          </div>
        })}
        {!loading && !error && polls.length === 0 && <p className="muted">Aktuell keine offenen Umfragen.</p>}
      </section>

      <section className="card widget-notifications">
        <div className="section-heading"><h2>Benachrichtigungen</h2><span className="eyebrow">Aktivit&auml;t</span></div>
        <GuildNotifications polls={polls} raids={raids} />
      </section>

      <section className="card widget-history">
        <div className="section-heading"><h2>Raid Historie</h2><Link to="/raids">Alle &rarr;</Link></div>
        <div className="table-scroll"><table className="table small">
          <thead><tr><th>Datum</th><th>Raid</th><th>Gr&ouml;&szlig;e</th><th>Status</th></tr></thead>
          <tbody>
            {history.map((r) => <tr key={r.id}>
              <td>{r.archived_at ? new Date(r.archived_at).toLocaleDateString('de-DE') : '—'}</td>
              <td><Link to={`/raids/${r.id}`}>{r.name}</Link></td>
              <td>{r.raid_size ?? '—'}</td>
              <td><span className="badge ok">Archiviert</span></td>
            </tr>)}
            {history.length === 0 && <tr><td colSpan={4} className="muted">Noch keine archivierten Raids.</td></tr>}
          </tbody>
        </table></div>
      </section>

      {player && <section className="card widget-profile">
        <div className="section-heading"><h2>Mein Profil</h2></div>
        {player && <PlayerProfilePanel player={player} characters={characters} points={points} />}
        {player && atLeast(player.role, 'MEMBER') && <p className="profile-link"><Link to="/wishlist">Meine Wunschliste &amp; Addon-Export &rarr;</Link></p>}
        {pointsError && <ErrorMessage>{pointsError}</ErrorMessage>}
      </section>}

      <section className="card widget-agenda">
        <div className="section-heading"><h2>Raidagenda</h2><span className="eyebrow">Gespeicherte Termine</span></div>
        {scheduledRaids.slice(0, 5).map((r) => <div className="agenda-row" key={r.id}>
          <time dateTime={r.scheduled_start!}><strong>{new Date(r.scheduled_start!).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' })}</strong><span>{new Date(r.scheduled_start!).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })}</span></time>
          <Link to={`/raids/${r.id}`}>{r.name}</Link>
          <span className="badge">{Date.parse(r.scheduled_start!) <= now ? 'Läuft' : r.status === 'OPEN' ? 'Offen' : 'Geplant'}</span>
        </div>)}
        {!loading && !error && scheduledRaids.length === 0 && <p className="muted">Keine laufenden oder zuk&uuml;nftigen Termine gespeichert.</p>}
      </section>
    </div>

    {/* <div className="dashboard-columns">
      <div className="dashboard-points">
        {pointsError && <ErrorMessage>{pointsError}</ErrorMessage>}
        <EnhancePointsCard points={points} />
        {player && <p className="profile-link"><Link to={`/players/${player.id}`}>Mein Profil &amp; EP-Historie &rarr;</Link></p>}
      </div>
      <section className="card widget-agenda">
        <div className="section-heading"><h2>Raidagenda</h2><span className="eyebrow">Gespeicherte Termine</span></div>
        {upcoming.slice(0, 5).map((r) => <div className="agenda-row" key={r.id}>
          <time dateTime={r.scheduled_start!}><strong>{new Date(r.scheduled_start!).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' })}</strong><span>{new Date(r.scheduled_start!).toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit' })}</span></time>
          <Link to={`/raids/${r.id}`}>{r.name}</Link>
          <span className="badge">{r.status === 'OPEN' ? 'Offen' : 'Geplant'}</span>
        </div>)}
        {!loading && !error && upcoming.length === 0 && <p className="muted">Keine zuk&uuml;nftigen Termine gespeichert.</p>}
      </section>
      <section className="card dashboard-polls">
        <div className="section-heading"><h2>Offene Umfragen</h2><Link to="/polls">Alle &rarr;</Link></div>
        {votesError && <ErrorMessage>{votesError}</ErrorMessage>}
        {polls.map((p) => <div className="poll-preview" key={p.id}>
          <h3>{p.title}</h3>
          {p.description && <p className="muted">{p.description}</p>}
          <div className="section-heading"><span className="muted">{votes[p.id] ? `${votes[p.id].length} abgegebene Stimmen` : 'Stimmen werden geladen&hellip;'}</span><Link to="/polls">Zur Abstimmung &rarr;</Link></div>
        </div>)}
        {!loading && !error && polls.length === 0 && <p className="muted">Aktuell keine offenen Umfragen.</p>}
      </section>
      <section className="card dashboard-history">
        <div className="section-heading"><h2>Zuletzt archiviert</h2><Link to="/raids">Historie &rarr;</Link></div>
        {history.slice(0, 5).map((r) => <div className="archive-row" key={r.id}>
          <Link to={`/raids/${r.id}`}>{r.name}</Link>
          <time dateTime={r.archived_at!}>{new Date(r.archived_at!).toLocaleDateString('de-DE')}</time>
          <span className="badge ok">Archiviert</span>
        </div>)}
        {!loading && !error && history.length === 0 && <p className="muted">Noch keine archivierten Raids.</p>}
      </section>
    </div> */}
  </div>
}
