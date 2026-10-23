import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link, navigate } from '../router'
import { atLeast, catalogRaidSizes, type LootCatalog, type Player, type Raid, type RaidSeries } from '../types'
import { useSubmission } from '../useSubmission'
import { CatalogAssociationFields } from '../components/CatalogAssociationFields'
import { ErrorMessage } from '../components/ErrorMessage'

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

function isoWeek(date: Date): number {
  const day = date.getUTCDay() || 7
  const thursday = new Date(date)
  thursday.setUTCDate(date.getUTCDate() + 4 - day)
  const yearStart = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 1))
  return Math.ceil(((thursday.getTime() - yearStart.getTime()) / 86400000 + 1) / 7)
}

function previewSeriesName(series: RaidSeries, scheduled: string): string {
  const template = (series.name_template ?? '').trim()
  if (!template) return series.name
  const parsed = scheduled ? new Date(scheduled) : new Date()
  const raidDate = Number.isNaN(parsed.getTime()) ? new Date() : parsed
  return template
    .replace(/\{number\}/g, String(series.next_raid_number ?? 1))
    .replace(/\{name\}/g, series.name)
    .replace(/\{date\}/g, raidDate.toISOString().slice(0, 10))
    .replace(/\{week\}/g, String(isoWeek(raidDate)))
}

export function RaidsPage() {
  const { pending, submit, isActive, isPending } = useSubmission()
  const { player } = useAuth()
  const [raids, setRaids] = useState<Raid[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [scheduled, setScheduled] = useState('')
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [seriesID, setSeriesID] = useState('')
  const [players, setPlayers] = useState<Player[]>([])
  const [leaderID, setLeaderID] = useState('')
  const [filter, setFilter] = useState('all')
  const [planSeriesID, setPlanSeriesID] = useState('')
  const [planUntil, setPlanUntil] = useState('')
  const [planMessage, setPlanMessage] = useState('')
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogError, setCatalogError] = useState('')
  const [instanceID, setInstanceID] = useState('')
  const [raidSize, setRaidSize] = useState('')
  const selectedSeries = series.find((s) => s.id === Number(seriesID))
  const associationValid = !!catalog?.sources.length && !!catalog.instances.some((instance) => instance.id === instanceID) && !!raidSize && catalogRaidSizes(catalog, instanceID).includes(Number(raidSize))
  const visibleRaids = raids.filter((r) => filter === 'all' || (filter === 'upcoming'
    ? !r.archived_at && (r.status === 'PLANNED' || r.status === 'OPEN')
    : !!r.archived_at || r.status === 'COMPLETED' || r.status === 'CANCELLED'))

  const canManage = player ? atLeast(player.role, 'RAIDASSISTENT') : false

  useEffect(() => {
    Promise.all([api.get<Raid[]>('/api/v1/raids?include_archived=true'), api.get<RaidSeries[]>('/api/v1/raid-series'), api.get<Player[]>('/api/v1/players')])
      .then(([r, s, p]) => { if (isActive()) { setRaids(r); setSeries(s); setPlayers(p) } })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(() => {
    api.get<LootCatalog>('/api/v1/loot-catalog')
      .then((value) => { if (isActive()) setCatalog(value) })
      .catch((err) => { if (isActive()) setCatalogError(err instanceof ApiError ? err.message : 'Beuteauswahl konnte nicht geladen werden. Neue Raids benötigen eine Raidinstanz und Raidgröße.') })
  }, [isActive])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!player || !canManage || (!name.trim() && !selectedSeries) || !leaderID || !scheduled || !associationValid || (!!selectedSeries?.catalog_instance_id && !instanceID)) return
    await submit(async () => {
      setError('')
      try {
        const created = await api.post<Raid>('/api/v1/raids', {
          name: name.trim(),
          raid_leader_id: Number(leaderID),
          ...(selectedSeries ? { series_id: selectedSeries.id } : {}),
          scheduled_start: new Date(scheduled).toISOString(),
          ...(instanceID ? { catalog_instance_id: instanceID, ...(raidSize ? { raid_size: Number(raidSize) } : {}) } : {}),
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

  const onPlan = async (event: FormEvent) => {
    event.preventDefault()
    if (!canManage || !planSeriesID || !planUntil) return
    await submit(async () => {
      setError('')
      setPlanMessage('')
      try {
        const planned = await api.post<Raid[]>(`/api/v1/raid-series/${planSeriesID}/plan`, { until: planUntil })
        if (!isActive()) return
        const rows = await api.get<Raid[]>('/api/v1/raids?include_archived=true')
        if (!isActive()) return
        setRaids(rows)
        setPlanMessage(planned.length === 0
          ? 'Alle Termine bis zu diesem Datum sind schon angelegt.'
          : `${planned.length === 1 ? '1 Raid' : `${planned.length} Raids`} angelegt. Die Anmeldung ist offen.`)
      } catch (err) {
        if (!isActive()) return
        if (err instanceof ApiError && err.code === 'plan_until_past') setError('Bitte ein Datum in der Zukunft wählen.')
        else if (err instanceof ApiError && err.code === 'plan_too_far') setError('So weit im Voraus kann nicht geplant werden (höchstens zehn Jahre).')
        else setError(err instanceof ApiError ? err.message : 'Planen fehlgeschlagen')
      }
    })
  }

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Planung & Historie</p><h1>Raids</h1></div><Link to="/">Zur Übersicht</Link></div>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {canManage && (
        <details className="card editor-disclosure"><summary>Neuer Raid</summary>
        <form className="card inline-form" onSubmit={onCreate}>
          <label>Raidserie<select aria-label="Neue Raidserie" disabled={pending} value={seriesID} onChange={(e) => {
            if (isPending() || !isActive()) return
            setSeriesID(e.target.value)
            const selected = series.find((s) => s.id === Number(e.target.value))
            setLeaderID(selected ? String(selected.raid_leader_id) : '')
            setInstanceID(selected?.catalog_instance_id ?? '')
            setRaidSize(selected?.raid_size == null ? '' : String(selected.raid_size))
          }}><option value="">Keine Serie</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <p className="muted">Die Serie übernimmt Raidleiter, Raidinstanz und Raidgröße; der Raidname wird aus der Namensvorlage abgeleitet. {selectedSeries && `Wiederholung: ${selectedSeries.start_time}, ${selectedSeries.timezone}, alle ${selectedSeries.interval_weeks} Wochen.`}</p>
          {series.length === 0 && <p className="muted">Keine Raidserie vorhanden — ein Raid ohne Serie benötigt einen eigenen Namen.</p>}
          <label>Raidleiter<select aria-label="Neuer Raidleiter" disabled={pending} required value={leaderID} onChange={(e) => setLeaderID(e.target.value)}><option value="">Raidleiter auswählen</option>{players.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
          {catalogError && <ErrorMessage>{catalogError}</ErrorMessage>}
          <CatalogAssociationFields catalog={catalog} instanceID={instanceID} raidSize={raidSize} disabled={pending} existingInstanceID={selectedSeries?.catalog_instance_id ?? ''} existingRaidSize={selectedSeries?.raid_size == null ? '' : String(selectedSeries?.raid_size)} onChange={(instance, size) => { if (!isPending() && isActive()) { setInstanceID(instance); setRaidSize(size) } }} />
          {!associationValid && <ErrorMessage>Bitte eine verfügbare Raidgröße zur Raidinstanz auswählen.</ErrorMessage>}
          {selectedSeries ? (
            <p className="muted">Name wird aus der Namensvorlage abgeleitet: {previewSeriesName(selectedSeries, scheduled)}</p>
          ) : (
            <label>
              Name
              <input aria-label="Neuer Raidname" value={name} onChange={(e) => setName(e.target.value)} required />
            </label>
          )}
          <label>
            Start ({Intl.DateTimeFormat().resolvedOptions().timeZone}, wird als UTC gespeichert)
            <input disabled={pending} required type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
          </label>
          <button type="submit" disabled={pending || !associationValid}>{pending ? 'Anlegen…' : 'Anlegen'}</button>
        </form>
        </details>
      )}
      {canManage && series.length > 0 && (
        <details className="card editor-disclosure"><summary>Termine im Voraus anlegen</summary>
        <form className="card inline-form" onSubmit={onPlan}>
          <p className="muted">Legt alle Raids der Serie bis zum gewählten Tag an, nach deren Wochentagen und Startzeit. Bereits angelegte Termine bleiben unverändert, die Anmeldung ist sofort offen.</p>
          <label>Raidserie<select aria-label="Raidserie für Vorausplanung" disabled={pending} required value={planSeriesID} onChange={(e) => { setPlanSeriesID(e.target.value); setPlanMessage('') }}>
            <option value="">Raidserie auswählen</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </select></label>
          <label>Bis einschließlich<input aria-label="Planen bis" disabled={pending} required type="date" value={planUntil} onChange={(e) => { setPlanUntil(e.target.value); setPlanMessage('') }} /></label>
          {planMessage && <p className="success-message" role="status"><span>{planMessage}</span></p>}
          <button type="submit" disabled={pending || !planSeriesID || !planUntil}>{pending ? 'Anlegen…' : 'Raids anlegen'}</button>
        </form>
        </details>
      )}
      <section className="card directory-panel" aria-label="Raidübersicht">
      <div className="section-heading"><h2>Raidkalender & Historie</h2><label>Ansicht <select aria-label="Raid-Ansicht" value={filter} onChange={(e) => setFilter(e.target.value)}>
        <option value="all">Alle Raids</option><option value="upcoming">Anstehend</option><option value="history">Historie</option>
      </select></label></div>
      <div className="table-scroll"><table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Start</th>
            <th>Notizen</th>
          </tr>
        </thead>
        <tbody>
          {visibleRaids.map((raid) => (
            <tr key={raid.id}>
              <td>
                <Link to={`/raids/${raid.id}`}>{raid.name}</Link>
              </td>
              <td>
                <span className={statusClass(raid.status)}>{raid.archived_at ? 'ARCHIVIERT' : raid.status}</span>
              </td>
              <td>{raid.scheduled_start ? new Date(raid.scheduled_start).toLocaleString() : '—'}</td>
              <td>{raid.notes}</td>
            </tr>
          ))}
          {visibleRaids.length === 0 && (
            <tr>
              <td colSpan={4} className="muted">
                {canManage ? 'Noch keine Raids. Mit „Neuer Raid“ den ersten Termin anlegen.' : 'Noch keine Raids.'}
              </td>
            </tr>
          )}
        </tbody>
      </table></div>
      </section>

    </div>
  )
}
