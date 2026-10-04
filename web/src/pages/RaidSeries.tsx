import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Player, type RaidSeries } from '../types'
import { useSubmission } from '../useSubmission'

const weekdays = ['Sonntag', 'Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag']
const emptySchedule = { name: '', raid_leader_id: '', weekdays: [] as number[], interval_weeks: '', start_time: '', timezone: '' }

export function RaidSeriesPage() {
  const { player } = useAuth()
  const { pending, submit, isActive, isPending } = useSubmission()
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [editingID, setEditingID] = useState<number | null>(null)
  const [schedule, setSchedule] = useState(emptySchedule)
  const [error, setError] = useState('')
  const isAdmin = !!player && atLeast(player.role, 'ADMIN')

  useEffect(() => {
    Promise.all([api.get<RaidSeries[]>('/api/v1/raid-series'), api.get<Player[]>('/api/v1/players')])
      .then(([s, p]) => { if (isActive()) { setSeries(s); setPlayers(p) } })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  const edit = (row: RaidSeries | null) => {
    if (!isAdmin || isPending() || !isActive()) return
    setEditingID(row?.id ?? null)
    setSchedule(row ? { name: row.name, raid_leader_id: String(row.raid_leader_id), weekdays: [...row.weekdays], interval_weeks: String(row.interval_weeks), start_time: row.start_time, timezone: row.timezone } : emptySchedule)
    setError('')
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !schedule.name.trim() || !schedule.raid_leader_id || !schedule.weekdays.length
      || !Number.isInteger(Number(schedule.interval_weeks)) || Number(schedule.interval_weeks) < 1
      || !schedule.start_time || !schedule.timezone.trim()) return
    await submit(async () => {
      setError('')
      try {
        const body = { ...schedule, name: schedule.name.trim(), raid_leader_id: Number(schedule.raid_leader_id), interval_weeks: Number(schedule.interval_weeks), timezone: schedule.timezone.trim() }
        const saved = editingID === null
          ? await api.post<RaidSeries>('/api/v1/raid-series', body)
          : await api.patch<RaidSeries>(`/api/v1/raid-series/${editingID}`, body)
        if (!isActive()) return
        setSeries((rows) => editingID === null ? [...rows, saved] : rows.map((r) => r.id === editingID ? saved : r))
        setEditingID(null)
        setSchedule(emptySchedule)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Speichern fehlgeschlagen')
      }
    })
  }

  return <div>
    <p><Link to="/roster">Roster</Link></p>
    <h1>Raidserien</h1>
    <p className="muted">Beim Archivieren entsteht der nächste zukünftige Termin nach dieser Konfiguration. Die Wiederholung verwendet die konfigurierte IANA-Zeitzone, nicht die Browser-Zeitzone.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <ul className="plain-list">{series.map((s) => <li key={s.id}>
      <span>{s.name}: {s.weekdays.map((day) => weekdays[day]).join(', ')}, alle {s.interval_weeks} Wochen, {s.start_time} ({s.timezone}); Raidleiter: {players.find((p) => p.id === s.raid_leader_id)?.name ?? 'Nicht verfügbar'}</span>
      {isAdmin && <button disabled={pending} onClick={() => edit(s)}>Serie bearbeiten</button>}
    </li>)}</ul>
    {series.length === 0 && <p className="muted">Keine Raidserien konfiguriert.</p>}
    {isAdmin && <form className="card inline-form" onSubmit={save}>
      <h2>{editingID === null ? 'Neue Raidserie' : 'Raidserie bearbeiten'}</h2>
      <label>Name<input disabled={pending} required placeholder="Serienname" value={schedule.name} onChange={(e) => setSchedule({ ...schedule, name: e.target.value })} /></label>
      <label>Raidleiter<select aria-label="Serien-Raidleiter" disabled={pending} required value={schedule.raid_leader_id} onChange={(e) => setSchedule({ ...schedule, raid_leader_id: e.target.value })}>
        <option value="">Raidleiter auswählen</option>{players.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
      </select></label>
      <fieldset disabled={pending}><legend>Wochentage (mindestens einer)</legend>{weekdays.map((day, index) => <label className="check" key={day}>
        <input type="checkbox" checked={schedule.weekdays.includes(index)} onChange={(e) => setSchedule({ ...schedule, weekdays: e.target.checked ? [...schedule.weekdays, index].sort((a, b) => a - b) : schedule.weekdays.filter((d) => d !== index) })} /> {day}
      </label>)}</fieldset>
      <label>Intervall in Wochen<input disabled={pending} required type="number" min="1" step="1" placeholder="Wochenintervall" value={schedule.interval_weeks} onChange={(e) => setSchedule({ ...schedule, interval_weeks: e.target.value })} /></label>
      <label>Startzeit in der Serien-Zeitzone<input disabled={pending} required type="time" value={schedule.start_time} onChange={(e) => setSchedule({ ...schedule, start_time: e.target.value })} /></label>
      <label>IANA-Zeitzone<input disabled={pending} required placeholder="IANA-Zeitzone" value={schedule.timezone} onChange={(e) => setSchedule({ ...schedule, timezone: e.target.value })} /></label>
      <button disabled={pending || schedule.weekdays.length === 0} type="submit">Serie speichern</button>
      {editingID !== null && <button disabled={pending} type="button" onClick={() => edit(null)}>Bearbeitung abbrechen</button>}
    </form>}
  </div>
}
