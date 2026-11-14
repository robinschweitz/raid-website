import { useEffect, useRef, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, catalogRaidSizes, type LootCatalog, type Player, type RaidSeries } from '../types'
import { useSubmission } from '../useSubmission'
import { CatalogAssociationFields } from '../components/CatalogAssociationFields'
import { ErrorMessage } from '../components/ErrorMessage'
import { EpPoolsPanel } from '../components/EpPoolsPanel'
import { useEpPools } from '../epPools'

const weekdays = ['Sonntag', 'Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag']
const emptySchedule = { name: '', name_template: '', next_raid_number: '1', raid_leader_id: '', extra_leaders: [] as number[], weekdays: [] as number[], interval_weeks: '', start_time: '', plan_weeks_ahead: '0', catalog_instance_id: '', raid_size: '', ep_pool_id: '', ep_award: '10' }

export function RaidSeriesPage() {
  const { player } = useAuth()
  const { pending, submit, isActive, isPending } = useSubmission()
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [editingID, setEditingID] = useState<number | null>(null)
  const editVersion = useRef(0)
  const renderedEditVersion = editVersion.current
  const [schedule, setSchedule] = useState(emptySchedule)
  const [error, setError] = useState('')
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogError, setCatalogError] = useState('')
  const existing = series.find((row) => row.id === editingID)
  const associationUnchanged = schedule.catalog_instance_id === (existing?.catalog_instance_id ?? '') && schedule.raid_size === (existing?.raid_size == null ? '' : String(existing.raid_size))
  const associationValid = (!!existing && associationUnchanged) || (!!catalog?.sources.length && !!catalog.instances.some((instance) => instance.id === schedule.catalog_instance_id) && !!schedule.raid_size && catalogRaidSizes(catalog, schedule.catalog_instance_id).includes(Number(schedule.raid_size)))
  const isAdmin = !!player && atLeast(player.role, 'ADMIN')
  const [poolsVersion, setPoolsVersion] = useState(0)
  const pools = useEpPools(poolsVersion)
  const poolName = (id: number | null | undefined) => id == null ? null : pools.find((pool) => pool.id === id)?.name ?? `EP-Topf #${id}`
  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? 'Nicht verfügbar'

  useEffect(() => {
    Promise.all([api.get<RaidSeries[]>('/api/v1/raid-series'), api.get<Player[]>('/api/v1/players')])
      .then(([s, p]) => { if (isActive()) { setSeries(s); setPlayers(p) } })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(() => {
    api.get<LootCatalog>('/api/v1/loot-catalog')
      .then((value) => { if (isActive()) setCatalog(value) })
      .catch((err) => { if (isActive()) setCatalogError(err instanceof ApiError ? err.message : 'Beuteauswahl konnte nicht geladen werden. Bestehende Zuordnungen bleiben unverändert.') })
  }, [isActive])

  const edit = (row: RaidSeries | null) => {
    if (!isAdmin || isPending() || !isActive()) return
    ++editVersion.current
    setEditingID(row?.id ?? null)
    setSchedule(row ? { name: row.name, name_template: row.name_template ?? '', next_raid_number: String(row.next_raid_number ?? 1), raid_leader_id: String(row.raid_leader_id), extra_leaders: (row.leader_ids ?? []).slice(1), weekdays: [...row.weekdays], interval_weeks: String(row.interval_weeks), start_time: row.start_time, plan_weeks_ahead: String(row.plan_weeks_ahead ?? 0), catalog_instance_id: row.catalog_instance_id ?? '', raid_size: row.raid_size == null ? '' : String(row.raid_size), ep_pool_id: row.ep_pool_id == null ? '0' : String(row.ep_pool_id), ep_award: String(row.ep_award ?? 10) } : emptySchedule)
    setError('')
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || renderedEditVersion !== editVersion.current || !schedule.name.trim() || !schedule.raid_leader_id || !schedule.weekdays.length
      || !Number.isInteger(Number(schedule.interval_weeks)) || Number(schedule.interval_weeks) < 1
      || !schedule.start_time || !associationValid
      || !Number.isInteger(Number(schedule.plan_weeks_ahead)) || Number(schedule.plan_weeks_ahead) < 0
      || !Number.isInteger(Number(schedule.ep_award)) || Number(schedule.ep_award) < 0 || Number(schedule.ep_award) > 100
      || (!!existing?.catalog_instance_id && !schedule.catalog_instance_id) || (existing?.raid_size != null && !schedule.raid_size)) return
    await submit(async () => {
      setError('')
      try {
        const { catalog_instance_id, raid_size, name_template, next_raid_number, extra_leaders, raid_leader_id, ep_pool_id, ep_award, ...fields } = schedule
        const trimmedTemplate = name_template.trim()
        // The first leader comes first; the others follow once each (ADR 112).
        const firstLeader = Number(raid_leader_id)
        const body = { ...fields, name: schedule.name.trim(), leader_ids: [firstLeader, ...extra_leaders.filter((id) => id !== firstLeader)], interval_weeks: Number(schedule.interval_weeks), plan_weeks_ahead: Number(schedule.plan_weeks_ahead),
          ...(trimmedTemplate || (editingID !== null && !!existing?.name_template) ? { name_template: trimmedTemplate, next_raid_number: Number(next_raid_number) } : {}),
          ...(!associationUnchanged && catalog_instance_id ? { catalog_instance_id, raid_size: Number(raid_size) } : {}),
          // An empty choice on a new series keeps the server default: the first pool (ADR 113).
          ...(ep_pool_id !== '' ? { ep_pool_id: Number(ep_pool_id) } : {}),
          ep_award: Number(ep_award),
        }
        const saved = editingID === null
          ? await api.post<RaidSeries>('/api/v1/raid-series', body)
          : await api.patch<RaidSeries>(`/api/v1/raid-series/${editingID}`, body)
        if (!isActive()) return
        ++editVersion.current
        setSeries((rows) => editingID === null ? [...rows, saved] : rows.map((r) => r.id === editingID ? saved : r))
        setEditingID(null)
        setSchedule(emptySchedule)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Speichern fehlgeschlagen')
      }
    })
  }

  const editorForm = <form className="inline-form" onSubmit={save}>
      <label>Name<input disabled={pending} required placeholder="Serienname" value={schedule.name} onChange={(e) => setSchedule({ ...schedule, name: e.target.value })} /></label>
      <label>Namensvorlage<input disabled={pending} placeholder="z.B. icc25-{number}" value={schedule.name_template} onChange={(e) => setSchedule({ ...schedule, name_template: e.target.value })} /></label>
      <label>Nächste Nummer<input disabled={pending} required type="number" min="1" step="1" value={schedule.next_raid_number} onChange={(e) => setSchedule({ ...schedule, next_raid_number: e.target.value })} /></label>
      <p className="muted">Variablen: {'{number}'}, {'{name}'}, {'{date}'}, {'{week}'}</p>
      <label>Raidleiter<select aria-label="Serien-Raidleiter" disabled={pending} required value={schedule.raid_leader_id} onChange={(e) => setSchedule({ ...schedule, raid_leader_id: e.target.value })}>
        <option value="">Raidleiter auswählen</option>{players.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
      </select></label>
      <fieldset className="series-leaders" disabled={pending}><legend>Weitere Raidleiter (optional)</legend>
        {schedule.extra_leaders.map((id) => <p key={id}>{playerName(id)} <button type="button" onClick={() => setSchedule({ ...schedule, extra_leaders: schedule.extra_leaders.filter((row) => row !== id) })}>Entfernen</button></p>)}
        <label>Raidleiter hinzufügen<select aria-label="Weiteren Serien-Raidleiter hinzufügen" value="" onChange={(e) => { const id = Number(e.target.value); if (id) setSchedule({ ...schedule, extra_leaders: [...schedule.extra_leaders, id] }) }}>
          <option value="">Spieler wählen</option>
          {players.filter((p) => p.active && String(p.id) !== schedule.raid_leader_id && !schedule.extra_leaders.includes(p.id)).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select></label>
        <p className="muted">Neue Raids der Serie bekommen alle Raidleiter. Bestehende Raids ändern sich nicht.</p>
      </fieldset>
      <fieldset disabled={pending}><legend>Wochentage (mindestens einer)</legend>{weekdays.map((day, index) => <label className="check" key={day}>
        <input type="checkbox" checked={schedule.weekdays.includes(index)} onChange={(e) => setSchedule({ ...schedule, weekdays: e.target.checked ? [...schedule.weekdays, index].sort((a, b) => a - b) : schedule.weekdays.filter((d) => d !== index) })} /> {day}
      </label>)}</fieldset>
      <label>Intervall in Wochen<input disabled={pending} required type="number" min="1" step="1" placeholder="Wochenintervall" value={schedule.interval_weeks} onChange={(e) => setSchedule({ ...schedule, interval_weeks: e.target.value })} /></label>
      <label>Startzeit (Serverzeit)<input disabled={pending} required type="time" value={schedule.start_time} onChange={(e) => setSchedule({ ...schedule, start_time: e.target.value })} /></label>
      <label>Automatisch im Voraus planen (Wochen, 0 = aus)<input aria-label="Wochen im Voraus" disabled={pending} required type="number" min="0" step="1" value={schedule.plan_weeks_ahead} onChange={(e) => setSchedule({ ...schedule, plan_weeks_ahead: e.target.value })} /></label>
      <p className="muted">Hält so viele Wochen an Raids dieser Serie angelegt, mit offener Anmeldung. Aufgefüllt wird beim Speichern der Serie und bei jedem Archivieren.</p>
      <fieldset disabled={pending}><legend>Enhance Points</legend>
        <label>EP-Topf<select aria-label="EP-Topf der Serie" value={schedule.ep_pool_id} onChange={(e) => setSchedule({ ...schedule, ep_pool_id: e.target.value })}>
          {editingID === null && <option value="">Standard ({pools[0]?.name ?? 'erster EP-Topf'})</option>}
          {pools.map((pool) => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
          <option value="0">Keine EP</option>
        </select></label>
        <label>EP beim Archivieren<input aria-label="EP beim Archivieren" required type="number" min="0" max="100" step="1" value={schedule.ep_award} onChange={(e) => setSchedule({ ...schedule, ep_award: e.target.value })} /></label>
        <p className="muted">Jeder ohne Main-Spec-Loot bekommt diese EP beim Archivieren, im gewählten Topf; 0 gibt keine EP. Ohne EP-Topf gibt es weder EP noch Enhanced-Loot oder Locks. Bestehende Raids behalten ihre Einstellung.</p>
      </fieldset>
      {catalogError && <ErrorMessage>{catalogError}</ErrorMessage>}
      <CatalogAssociationFields catalog={catalog} instanceID={schedule.catalog_instance_id} raidSize={schedule.raid_size} disabled={pending} existingInstanceID={existing?.catalog_instance_id ?? ''} existingRaidSize={existing?.raid_size == null ? '' : String(existing.raid_size)} onChange={(instance, size) => { if (!isPending() && isActive()) setSchedule({ ...schedule, catalog_instance_id: instance, raid_size: size }) }} />
      {!associationValid && <ErrorMessage>Bitte eine verfügbare Raidgröße zur Raidinstanz auswählen.</ErrorMessage>}
      <button disabled={pending || schedule.weekdays.length === 0 || !associationValid} type="submit">Serie speichern</button>
      {editingID !== null && <button disabled={pending} type="button" onClick={() => edit(null)}>Bearbeitung abbrechen</button>}
    </form>

  return <div>
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Raidserien</h1></div></div>
    <p className="muted">Beim Archivieren entsteht der nächste zukünftige Termin nach dieser Konfiguration; abgesagte Termine werden übersprungen. Raids lassen sich auch auf der Raidseite im Voraus anlegen. Startzeiten sind Serverzeit (deutsche Zeit, mit Sommerzeit), nicht die Zeitzone des Browsers.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {isAdmin && editingID === null && <details className="card editor-disclosure"><summary>Neue Raidserie</summary>{editorForm}</details>}
    <ul className="series-directory">{series.map((s) => <li className={`card${editingID === s.id ? ' is-editing' : ''}`} key={s.id}>
      <h2>{s.name}</h2>
      {isAdmin && editingID === s.id ? editorForm : <>
        <p>{s.weekdays.map((day) => weekdays[day]).join(', ')} · {s.start_time}</p>
        <p className="muted">{s.interval_weeks === 1 ? 'Jede Woche' : `Alle ${s.interval_weeks} Wochen`}</p>
        {!!s.plan_weeks_ahead && <p className="muted">Automatisch {s.plan_weeks_ahead === 1 ? '1 Woche' : `${s.plan_weeks_ahead} Wochen`} im Voraus geplant</p>}
        {s.name_template && <p className="muted">Namensvorlage: {s.name_template} · Nächste Nummer: {s.next_raid_number}</p>}
        <p className="muted">Raidleitung: {(s.leader_ids?.length ? s.leader_ids : [s.raid_leader_id]).map(playerName).join(', ')}{s.catalog_instance_id && ` · ${catalog?.instances.find((instance) => instance.id === s.catalog_instance_id)?.name ?? `Instanz #${s.catalog_instance_id}`}${s.raid_size ? ` · ${s.raid_size} Spieler` : ''}`}</p>
        <p className="muted">{s.ep_pool_id == null || s.ep_award === 0 ? 'Keine EP beim Archivieren' : `+${s.ep_award ?? 10} EP im Topf ${poolName(s.ep_pool_id)}`}{s.ep_pool_id == null && ' · kein Enhanced-Loot'}</p>
        {isAdmin && <button disabled={pending} onClick={() => edit(s)}>Serie bearbeiten</button>}
      </>}
    </li>)}</ul>
    {isAdmin && <EpPoolsPanel pools={pools} onChanged={() => setPoolsVersion((v) => v + 1)} />}
    {series.length === 0 && <p className="muted">{isAdmin ? 'Keine Raidserien konfiguriert. Mit „Neue Raidserie“ die erste anlegen.' : 'Keine Raidserien konfiguriert.'}</p>}
  </div>
}
