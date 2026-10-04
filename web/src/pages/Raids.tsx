import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link, navigate } from '../router'
import { atLeast, catalogRaidSizes, type LootCatalog, type Player, type Raid, type RaidSeries } from '../types'
import { useSubmission } from '../useSubmission'
import { CatalogAssociationFields } from '../components/CatalogAssociationFields'

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
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogError, setCatalogError] = useState('')
  const [instanceID, setInstanceID] = useState('')
  const [raidSize, setRaidSize] = useState('')
  const selectedSeries = series.find((s) => s.id === Number(seriesID))
  const associationInherited = instanceID === (selectedSeries?.catalog_instance_id ?? '') && raidSize === (selectedSeries?.raid_size == null ? '' : String(selectedSeries.raid_size))
  const associationValid = associationInherited || (!instanceID && !raidSize) || (!!catalog?.instances.some((instance) => instance.id === instanceID) && !!raidSize && catalogRaidSizes(catalog, instanceID).includes(Number(raidSize)))
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
      .catch((err) => { if (isActive()) setCatalogError(err instanceof ApiError ? err.message : 'Beutekatalog konnte nicht geladen werden. Raidplanung ohne neue Zuordnung bleibt verfügbar.') })
  }, [isActive])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!player || !canManage || !name.trim() || !leaderID || !scheduled || !selectedSeries || !associationValid || (!!selectedSeries.catalog_instance_id && !instanceID)) return
    await submit(async () => {
      setError('')
      try {
        const created = await api.post<Raid>('/api/v1/raids', {
          name,
          raid_leader_id: Number(leaderID),
          series_id: selectedSeries.id,
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

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Planung & Historie</p><h1>Raids</h1></div><Link to="/">Zur Übersicht</Link></div>
      {error && <p className="error">{error}</p>}
      <label>Ansicht <select aria-label="Raid-Ansicht" value={filter} onChange={(e) => setFilter(e.target.value)}>
        <option value="all">Alle Raids</option><option value="upcoming">Anstehend</option><option value="history">Historie</option>
      </select></label>
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
                Noch keine Raids.
              </td>
            </tr>
          )}
        </tbody>
      </table></div>

      {canManage && (
        <form className="card inline-form" onSubmit={onCreate}>
          <h2>Neuer Raid</h2>
          <label>Raidserie<select aria-label="Neue Raidserie" disabled={pending} value={seriesID} required onChange={(e) => {
            if (isPending() || !isActive()) return
            setSeriesID(e.target.value)
            const selected = series.find((s) => s.id === Number(e.target.value))
            setName(selected?.name ?? '')
            setLeaderID(selected ? String(selected.raid_leader_id) : '')
            setInstanceID(selected?.catalog_instance_id ?? '')
            setRaidSize(selected?.raid_size == null ? '' : String(selected.raid_size))
          }}><option value="">Serie auswählen</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
          <p className="muted">Bei Serienauswahl werden der gespeicherte Serienname und Raidleiter übernommen. {selectedSeries && `Wiederholung: ${selectedSeries.start_time}, ${selectedSeries.timezone}, alle ${selectedSeries.interval_weeks} Wochen.`}</p>
          {series.length === 0 && <p className="muted">Ein Admin muss zuerst eine Raidserie konfigurieren.</p>}
          <label>Raidleiter<select aria-label="Neuer Raidleiter" disabled={pending} required value={leaderID} onChange={(e) => setLeaderID(e.target.value)}><option value="">Raidleiter auswählen</option>{players.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
          {catalogError && <p className="error" role="alert">{catalogError}</p>}
          <CatalogAssociationFields catalog={catalog} instanceID={instanceID} raidSize={raidSize} disabled={pending} existingInstanceID={selectedSeries?.catalog_instance_id ?? ''} existingRaidSize={selectedSeries?.raid_size == null ? '' : String(selectedSeries.raid_size)} onChange={(instance, size) => { if (!isPending() && isActive()) { setInstanceID(instance); setRaidSize(size) } }} />
          {!associationValid && <p className="error">Bitte eine durch lokale Drops belegte Raidgröße zur Instanz auswählen.</p>}
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Start ({Intl.DateTimeFormat().resolvedOptions().timeZone}, wird als UTC gespeichert)
            <input disabled={pending} required type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
          </label>
          <button type="submit" disabled={pending || !associationValid}>{pending ? 'Anlegen…' : 'Anlegen'}</button>
        </form>
      )}
    </div>
  )
}
