import { useEffect, useRef, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, catalogRaidSizes, type LootCatalog, type Player, type RaidSeries } from '../types'
import { useSubmission } from '../useSubmission'
import { CatalogAssociationFields } from '../components/CatalogAssociationFields'

const weekdays = ['Sonntag', 'Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag']
const emptySchedule = { name: '', raid_leader_id: '', weekdays: [] as number[], interval_weeks: '', start_time: '', timezone: '', catalog_instance_id: '', raid_size: '' }

export function RaidSeriesPage() {
  const { player } = useAuth()
  const { pending, submit, isActive, isPending } = useSubmission()
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [editingID, setEditingID] = useState<number | null>(null)
  const editor = useRef<HTMLDetailsElement>(null)
  const [schedule, setSchedule] = useState(emptySchedule)
  const [error, setError] = useState('')
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogError, setCatalogError] = useState('')
  const existing = series.find((row) => row.id === editingID)
  const associationUnchanged = schedule.catalog_instance_id === (existing?.catalog_instance_id ?? '') && schedule.raid_size === (existing?.raid_size == null ? '' : String(existing.raid_size))
  const associationValid = (!!existing && associationUnchanged) || (!!catalog?.sources.length && !!catalog.instances.some((instance) => instance.id === schedule.catalog_instance_id) && !!schedule.raid_size && catalogRaidSizes(catalog, schedule.catalog_instance_id).includes(Number(schedule.raid_size)))
  const isAdmin = !!player && atLeast(player.role, 'ADMIN')

  useEffect(() => {
    Promise.all([api.get<RaidSeries[]>('/api/v1/raid-series'), api.get<Player[]>('/api/v1/players')])
      .then(([s, p]) => { if (isActive()) { setSeries(s); setPlayers(p) } })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(() => {
    api.get<LootCatalog>('/api/v1/loot-catalog')
      .then((value) => { if (isActive()) setCatalog(value) })
      .catch((err) => { if (isActive()) setCatalogError(err instanceof ApiError ? err.message : 'Beutekatalog konnte nicht geladen werden. Bestehende Zuordnungen bleiben unverändert.') })
  }, [isActive])

  const edit = (row: RaidSeries | null) => {
    if (!isAdmin || isPending() || !isActive()) return
    setEditingID(row?.id ?? null)
    if (row && editor.current) {
      editor.current.open = true
      editor.current.scrollIntoView({ block: 'nearest' })
    }
    setSchedule(row ? { name: row.name, raid_leader_id: String(row.raid_leader_id), weekdays: [...row.weekdays], interval_weeks: String(row.interval_weeks), start_time: row.start_time, timezone: row.timezone, catalog_instance_id: row.catalog_instance_id ?? '', raid_size: row.raid_size == null ? '' : String(row.raid_size) } : emptySchedule)
    setError('')
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !schedule.name.trim() || !schedule.raid_leader_id || !schedule.weekdays.length
      || !Number.isInteger(Number(schedule.interval_weeks)) || Number(schedule.interval_weeks) < 1
      || !schedule.start_time || !schedule.timezone.trim() || !associationValid
      || (!!existing?.catalog_instance_id && !schedule.catalog_instance_id) || (existing?.raid_size != null && !schedule.raid_size)) return
    await submit(async () => {
      setError('')
      try {
        const { catalog_instance_id, raid_size, ...fields } = schedule
        const body = { ...fields, name: schedule.name.trim(), raid_leader_id: Number(schedule.raid_leader_id), interval_weeks: Number(schedule.interval_weeks), timezone: schedule.timezone.trim(),
          ...(!associationUnchanged && catalog_instance_id ? { catalog_instance_id, raid_size: Number(raid_size) } : {}),
        }
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
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Raidserien</h1></div></div>
    <p className="muted">Beim Archivieren entsteht der nächste zukünftige Termin nach dieser Konfiguration. Die Wiederholung verwendet die konfigurierte IANA-Zeitzone, nicht die Browser-Zeitzone.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <ul className="series-directory">{series.map((s) => <li className="card" key={s.id}>
      <div><h2>{s.name}</h2><p>{s.weekdays.map((day) => weekdays[day]).join(', ')} · {s.start_time}</p><p className="muted">Alle {s.interval_weeks} Wochen · {s.timezone}</p><p className="muted">Raidleitung: {players.find((p) => p.id === s.raid_leader_id)?.name ?? 'Nicht verfügbar'}{s.catalog_instance_id && ` · ${catalog?.instances.find((instance) => instance.id === s.catalog_instance_id)?.name ?? `Instanz #${s.catalog_instance_id}`}${s.raid_size ? ` · ${s.raid_size} Spieler` : ''}`}</p></div>
      {isAdmin && <button disabled={pending} onClick={() => edit(s)}>Serie bearbeiten</button>}
    </li>)}</ul>
    {series.length === 0 && <p className="muted">Keine Raidserien konfiguriert.</p>}
    {isAdmin && <details ref={editor} className="card editor-disclosure"><summary>{editingID === null ? 'Neue Raidserie' : 'Raidserie bearbeiten'}</summary><form className="inline-form" onSubmit={save}>
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
      {catalogError && <p className="error" role="alert">{catalogError}</p>}
      <CatalogAssociationFields catalog={catalog} instanceID={schedule.catalog_instance_id} raidSize={schedule.raid_size} disabled={pending} existingInstanceID={existing?.catalog_instance_id ?? ''} existingRaidSize={existing?.raid_size == null ? '' : String(existing.raid_size)} onChange={(instance, size) => { if (!isPending() && isActive()) setSchedule({ ...schedule, catalog_instance_id: instance, raid_size: size }) }} />
      {!associationValid && <p className="error">Bitte eine durch lokale Drops belegte Raidgröße zur Instanz auswählen.</p>}
      <button disabled={pending || schedule.weekdays.length === 0 || !associationValid} type="submit">Serie speichern</button>
      {editingID !== null && <button disabled={pending} type="button" onClick={() => edit(null)}>Bearbeitung abbrechen</button>}
    </form></details>}
  </div>
}
