import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type ArchiveResponse, type Attendance, type AttendanceRole, type Character, type LootAward, type Player, type Raid, type RaidSeries } from '../types'

interface Props {
  raidID: number
}

const attendanceFlags = [
  { key: 'is_substitute', label: 'Ersatz' },
  { key: 'is_bench', label: 'Bank' },
  { key: 'late_join', label: 'Später dabei' },
  { key: 'early_leave', label: 'Früher gegangen' },
] as const

const snapshotFields = ['character_name', 'class', 'spec'] as const
type SnapshotField = typeof snapshotFields[number]

function localStart(timestamp: string | null | undefined): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

function raidError(err: unknown, fallback: string): string {
  if (!(err instanceof ApiError)) return fallback
  if (err.code === 'reason_required') {
    return 'Für eine Korrektur ist eine nichtleere Begründung erforderlich.'
  }
  if (err.code === 'raid_state') return 'Der Raid erlaubt diese Änderung nicht mehr. Bitte die Seite neu laden.'
  const messages: Record<string, string> = {
    series_required: 'Zum Archivieren muss zuerst eine Raidserie zugeordnet werden.',
    scheduled_start_required: 'Zum Archivieren fehlt der geplante Start. Bitte den Start in der Raidplanung eintragen und speichern.',
    series_not_found: 'Die Raidserie existiert nicht mehr. Bitte neu laden und eine gültige Serie wählen.',
    series_not_editable: 'Bei diesem Raid kann die Serie nicht mehr geändert werden.',
    schedule_not_editable: 'Die Planung eines abgeschlossenen, abgesagten oder archivierten Raids kann nicht mehr geändert werden. Bitte neu laden.',
    invalid_scheduled_start: 'Bitte einen gültigen geplanten Start angeben.',
    roster_incomplete: 'Die Teilnahme ist unvollständig. Bitte die betroffenen Zeilen korrigieren: gespeicherter Charaktername, gültige Klasse, tatsächlicher Spec und Rolle sind zum Archivieren erforderlich.',
    occurrence_conflict: 'Der nächste Serientermin existiert bereits. Das Archivieren wurde nicht gespeichert.',
    local_time_nonexistent: 'Die nächste Startzeit existiert wegen der Zeitumstellung nicht. Ein Admin muss die Serienzeit korrigieren.',
    local_time_ambiguous: 'Die nächste Startzeit ist wegen der Zeitumstellung mehrdeutig. Ein Admin muss die Serienzeit korrigieren.',
    schedule_range: 'Für diese Konfiguration konnte kein nächster Termin ermittelt werden.',
    raid_cancelled: 'Abgesagte Raids können nicht archiviert werden.',
    raid_not_archivable: 'Dieser Raid kann nicht archiviert werden. Bitte neu laden.',
  }
  if (messages[err.code]) return messages[err.code]
  return err.message
}

export function RaidDetailPage({ raidID }: Props) {
  return <RaidDetail key={raidID} raidID={raidID} />
}

function RaidDetail({ raidID }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const characterVersion = useRef(0)
  const snapshotDirty = useRef(new Set<SnapshotField>())
  const lootFormVersion = useRef(0)
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<LootAward[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [seriesID, setSeriesID] = useState('')
  const [scheduled, setScheduled] = useState('')
  const [archiveResult, setArchiveResult] = useState<ArchiveResponse | null>(null)

  const [attPlayer, setAttPlayer] = useState('')
  const [attCharacter, setAttCharacter] = useState('')
  const [attRole, setAttRole] = useState<AttendanceRole>('DD')
  const [characters, setCharacters] = useState<Character[]>([])
  const [attEditing, setAttEditing] = useState<Attendance | null>(null)
  const [attDetails, setAttDetails] = useState({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
  const [attReason, setAttReason] = useState('')
  const [attSnapshot, setAttSnapshot] = useState<{ character_name?: string; class?: string; spec?: string }>({})

  const [lootItem, setLootItem] = useState('')
  const [lootItemID, setLootItemID] = useState('')
  const [lootBoss, setLootBoss] = useState('')
  const [lootPlayer, setLootPlayer] = useState('')
  const [lootMain, setLootMain] = useState(true)
  const [lootEditing, setLootEditing] = useState<LootAward | null>(null)
  const [lootCharacter, setLootCharacter] = useState('')
  const [lootNotes, setLootNotes] = useState('')
  const [lootReason, setLootReason] = useState('')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  const isActiveRaid = !!raid && !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')
  const canWrite = canManage && !!raid && !raid.archived_at && (isActiveRaid || raid.status === 'COMPLETED')
  const attNeedsReason = attEditing !== null || raid?.status === 'COMPLETED'
  const lootNeedsReason = lootEditing !== null || raid?.status === 'COMPLETED'
  const renderedCharacterVersion = characterVersion.current
  const renderedLootVersion = lootFormVersion.current
  const hasAttCharacter = characters.some((c) => c.id === Number(attCharacter) && c.player_id === Number(attPlayer))

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [r, a, l, p, s] = await Promise.all([
        api.get<Raid>(`/api/v1/raids/${raidID}`),
        api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
        api.get<LootAward[]>(`/api/v1/raids/${raidID}/loot`),
        api.get<Player[]>('/api/v1/players'),
        api.get<RaidSeries[]>('/api/v1/raid-series'),
      ])
      if (isActive() && version === loadVersion.current) {
        setRaid(r)
        setAttendance(a)
        setLoot(l)
        setPlayers(p)
        setSeries(s)
        setSeriesID(r.series_id ? String(r.series_id) : '')
        setScheduled(localStart(r.scheduled_start))
      }
    } catch (err) {
      if (isActive() && version === loadVersion.current) {
        setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen')
      }
    }
  }, [raidID, isActive])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    let obsolete = false
    const version = characterVersion.current
    if (!attPlayer) {
      setCharacters([])
      return
    }
    api
      .get<Character[]>(`/api/v1/players/${attPlayer}/characters`)
      .then((cs) => {
        if (!isActive() || obsolete || version !== characterVersion.current) return
        setCharacters(cs)
        setAttCharacter((selected) => selected || (cs.length > 0 ? String(cs[0].id) : ''))
        if (!attEditing || attEditing.player_id !== Number(attPlayer)) {
          const selected = cs[0]
          if (selected) {
            const defaults = { character_name: selected.name, class: selected.class, spec: selected.main_spec }
            setAttSnapshot((previous) => {
              if (version !== characterVersion.current) return previous
              return {
                character_name: snapshotDirty.current.has('character_name') ? previous.character_name : defaults.character_name,
                class: snapshotDirty.current.has('class') ? previous.class : defaults.class,
                spec: snapshotDirty.current.has('spec') ? previous.spec : defaults.spec,
              }
            })
          }
        }
      })
      .catch((err) => {
        if (!isActive() || obsolete || version !== characterVersion.current) return
        setCharacters([])
        setAttCharacter('')
        setError(err instanceof ApiError ? err.message : 'Charaktere laden fehlgeschlagen')
      })
    return () => { obsolete = true }
  }, [attPlayer, attEditing, isActive])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`

  const editAttendance = (row: Attendance | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++characterVersion.current
    snapshotDirty.current.clear()
    setAttEditing(row)
    setAttPlayer(row ? String(row.player_id) : '')
    setAttCharacter(row ? String(row.character_id) : '')
    setCharacters([])
    setAttRole(row?.role ?? 'DD')
    setAttDetails({
      is_substitute: row?.is_substitute ?? false, is_bench: row?.is_bench ?? false,
      late_join: row?.late_join ?? false, early_leave: row?.early_leave ?? false, notes: row?.notes ?? '',
    })
    setAttReason('')
    setAttSnapshot(row ? { character_name: row.character_name || undefined, class: row.class || undefined, spec: row.spec || undefined } : {})
  }

  const changeSnapshot = (field: SnapshotField, value: string) => {
    if (!canWrite || !isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
    snapshotDirty.current.add(field)
    setAttSnapshot((previous) => ({ ...previous, [field]: value }))
  }

  const editLoot = (row: LootAward | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++lootFormVersion.current
    setLootEditing(row)
    setLootItem(row?.item_name ?? '')
    setLootItemID(row?.item_id ?? '')
    setLootBoss(row?.boss ?? '')
    setLootPlayer(row ? String(row.awarded_to_player_id) : '')
    setLootCharacter(row?.awarded_to_character_id ? String(row.awarded_to_character_id) : '')
    setLootMain(row?.is_main_spec ?? true)
    setLootNotes(row?.notes ?? '')
    setLootReason('')
  }

  const addAttendance = async (event: FormEvent) => {
    event.preventDefault()
    if (!canWrite || renderedCharacterVersion !== characterVersion.current
      || !hasAttCharacter) return
    if (attNeedsReason && !attReason.trim()) {
      setError('Für eine Korrektur ist eine nichtleere Begründung erforderlich.')
      return
    }
    const unchangedRefs = attEditing && attEditing.player_id === Number(attPlayer) && attEditing.character_id === Number(attCharacter)
    const snapshotChanges: Partial<Record<SnapshotField, string>> = {}
    for (const field of snapshotFields) {
      const value = attSnapshot[field]
      if (value !== undefined && (!unchangedRefs || value !== (attEditing[field] || undefined))) {
        snapshotChanges[field] = value
      }
    }
    if (Object.values(snapshotChanges).some((value) => !value.trim())) {
      setError('Gespeicherter Charaktername, Klasse und tatsächlicher Spec dürfen nicht leer sein.')
      return
    }
    await submit(async () => {
      setError('')
      try {
        const body = {
          player_id: Number(attPlayer),
          character_id: Number(attCharacter),
          role: attRole,
          ...attDetails,
          ...snapshotChanges,
          ...(attNeedsReason ? { reason: attReason.trim() } : {}),
        }
        if (attEditing) {
          await api.patch<Attendance>(`/api/v1/raids/${raidID}/attendance/${attEditing.id}`, body)
        } else {
          await api.post<Attendance>(`/api/v1/raids/${raidID}/attendance`, body)
        }
        if (!isActive()) return
        ++characterVersion.current
        snapshotDirty.current.clear()
        setAttPlayer('')
        setCharacters([])
        setAttCharacter('')
        setAttEditing(null)
        setAttRole('DD')
        setAttDetails({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
        setAttReason('')
        setAttSnapshot({})
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Teilnahme fehlgeschlagen'))
      }
    })
  }

  const addLoot = async (event: FormEvent) => {
    event.preventDefault()
    if (!canWrite || renderedLootVersion !== lootFormVersion.current) return
    if (lootNeedsReason && !lootReason.trim()) {
      setError('Für eine Korrektur ist eine nichtleere Begründung erforderlich.')
      return
    }
    await submit(async () => {
      setError('')
      try {
        const body = {
          boss: lootBoss,
          item_id: lootItemID,
          item_name: lootItem,
          awarded_to_player_id: Number(lootPlayer),
          is_main_spec: lootMain,
          awarded_to_character_id: lootCharacter ? Number(lootCharacter) : null,
          notes: lootNotes,
          ...(lootNeedsReason ? { reason: lootReason.trim() } : {}),
        }
        if (lootEditing) {
          await api.patch<LootAward>(`/api/v1/raids/${raidID}/loot/${lootEditing.id}`, body)
        } else {
          await api.post<LootAward>(`/api/v1/raids/${raidID}/loot`, body)
        }
        if (!isActive()) return
        ++lootFormVersion.current
        setLootItem('')
        setLootItemID('')
        setLootBoss('')
        setLootPlayer('')
        setLootEditing(null)
        setLootCharacter('')
        setLootMain(true)
        setLootNotes('')
        setLootReason('')
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Loot fehlgeschlagen'))
      }
    })
  }

  const action = async (name: 'cancel' | 'archive') => {
    if (!canManage || !raid || raid.archived_at || raid.status === 'CANCELLED' || (name !== 'archive' && !isActiveRaid)) return
    if (name === 'archive' && isActiveRaid && (!raid.series_id || !raid.scheduled_start)) return
    await submit(async () => {
      setError('')
      try {
        if (name === 'archive') {
          const result = await api.postEnvelope<ArchiveResponse>(`/api/v1/raids/${raidID}/archive`)
          if (!isActive()) return
          setArchiveResult(result)
          setRaid(result.data)
        } else {
          await api.post<Raid>(`/api/v1/raids/${raidID}/cancel`)
        }
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Aktion fehlgeschlagen'))
      }
    })
  }

  const saveSchedule = async () => {
    if (!canManage || !isActiveRaid || !raid || !scheduled || (raid.series_id && !seriesID)) return
    const changes: { series_id?: number; scheduled_start?: string } = {}
    if (seriesID && Number(seriesID) !== raid.series_id) changes.series_id = Number(seriesID)
    if (scheduled !== localStart(raid.scheduled_start)) {
      const date = new Date(scheduled)
      if (Number.isNaN(date.getTime())) { setError('Bitte einen gültigen geplanten Start angeben.'); return }
      changes.scheduled_start = date.toISOString()
    }
    if (Object.keys(changes).length === 0) return
    await submit(async () => {
      setError('')
      try {
        const updated = await api.patch<Raid>(`/api/v1/raids/${raidID}`, changes)
        if (isActive()) {
          setRaid(updated)
          setSeriesID(updated.series_id ? String(updated.series_id) : '')
          setScheduled(localStart(updated.scheduled_start))
        }
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Planung speichern fehlgeschlagen'))
      }
    })
  }

  if (!raid) {
    return (
      <div>
        <p>
          <Link to="/">← Raids</Link>
        </p>
        {error ? <p className="error">{error}</p> : <p className="muted">Lade…</p>}
      </div>
    )
  }

  return (
    <div>
      <p>
        <Link to="/">← Raids</Link>
      </p>
      <h1>
        {raid.name} <span className="badge">{raid.archived_at ? 'ARCHIVIERT' : raid.status}</span>
      </h1>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}
      <p>Geplanter Start: {raid.scheduled_start ? new Date(raid.scheduled_start).toLocaleString() : 'Nicht verfügbar'}. Serie: {series.find((s) => s.id === raid.series_id)?.name ?? 'Nicht verfügbar'}.</p>
      {raid.previous_raid_id && <p><Link to={`/raids/${raid.previous_raid_id}`}>Vorheriger Raid</Link></p>}
      {archiveResult && <p role="status">Raid archiviert und weiterhin in der Historie sichtbar. {archiveResult.next_raid
        ? <Link to={`/raids/${archiveResult.next_raid.id}`}>Nächster Raid: {archiveResult.next_raid.name}{archiveResult.next_raid.scheduled_start ? ` (${new Date(archiveResult.next_raid.scheduled_start).toLocaleString()})` : ''}</Link>
        : 'Kein Nachfolger erstellt.'}</p>}
      {canWrite && <p className="muted">Änderungen bestehender Einträge erfordern eine Begründung und werden mit Vorher/Nachher protokolliert. Bei abgeschlossenen Raids gilt das auch für Ergänzungen; der Server gleicht betroffene EP und Tokens aus.</p>}

      {canManage && isActiveRaid && <section className="card">
        <h2>Raidplanung</h2>
        <select aria-label="Raidserie zuordnen" disabled={pending} value={seriesID} onChange={(e) => setSeriesID(e.target.value)}>
          <option value="">Serie auswählen</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name} ({s.start_time}, {s.timezone})</option>)}
        </select>
        <label>Geplanter Start ({Intl.DateTimeFormat().resolvedOptions().timeZone}, wird als UTC gespeichert)
          <input aria-label="Geplanter Start" disabled={pending} required={!raid.scheduled_start} type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
        </label>
        <button disabled={pending || !scheduled || (!!raid.series_id && !seriesID) || (Number(seriesID) === (raid.series_id ?? 0) && scheduled === localStart(raid.scheduled_start))} onClick={saveSchedule}>Planung speichern</button>
        {!raid.series_id && <p className="muted">Vor dem Archivieren muss eine Serie zugeordnet sein.</p>}
        {!raid.scheduled_start && <p className="muted">Vor dem Archivieren muss ein geplanter Start eingetragen und gespeichert sein.</p>}
        <p className="muted">Nur geänderte Werte werden gespeichert. Vorhandene Werte können nicht geleert werden.</p>
      </section>}
      {canManage && !raid.archived_at && raid.status !== 'CANCELLED' && (
        <section>
        <p className="muted">{isActiveRaid ? 'Archivieren beendet den Raid endgültig: +10 EP pro Teilnehmer ohne Main-Spec-Loot, ungenutzte Locks verfallen und der nächste zukünftige Serientermin wird angelegt. Der Raid bleibt als schreibgeschützte Historie sichtbar.' : 'Dieser historische abgeschlossene Raid wird nur archiviert. Keine erneute EP-Vergabe und kein Nachfolger.'}</p>
        <div className="actions">
          {isActiveRaid && <button disabled={pending} onClick={() => action('cancel')}>Absagen</button>}
          <button disabled={pending || (isActiveRaid && (!raid.series_id || !raid.scheduled_start))} onClick={() => action('archive')}>Archivieren</button>
        </div>
        </section>
      )}

      <section className="card">
        <h2>Teilnahme ({attendance.length})</h2>
        <div className="table-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>Spieler</th>
              <th>Charakter</th>
              <th>Tatsächliche Rolle</th>
              <th>Klasse</th>
              <th>Tatsächlicher Spec</th>
              {canWrite && <th></th>}
            </tr>
          </thead>
          <tbody>
            {attendance.map((a) => (
              <tr key={a.id}>
                <td>{playerName(a.player_id)}</td>
                <td>{a.character_name?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</td>
                <td>{a.role}</td>
                <td>{a.class?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</td>
                <td>{a.spec?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</td>
                {canWrite && <td>
                  {isActiveRaid && (!a.character_name?.trim() || !a.class?.trim() || !a.spec?.trim()) && <p className="error">Teilnahme #{a.id}: {[
                    !a.character_name?.trim() && 'Charaktername', !a.class?.trim() && 'Klasse', !a.spec?.trim() && 'tatsächlicher Spec',
                  ].filter(Boolean).join(', ')} fehlt. Vor dem Archivieren korrigieren.</p>}
                  <button disabled={pending} onClick={() => editAttendance(a)}>Teilnahme korrigieren</button>
                </td>}
              </tr>
            ))}
            {attendance.length === 0 && (
              <tr>
                <td colSpan={canWrite ? 6 : 5} className="muted">
                  Keine Einträge.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
        {canWrite && (
          <div className="subcard">
            <h3>{attEditing ? `Teilnahme #${attEditing.id} korrigieren` : raid.status === 'COMPLETED' ? 'Teilnahme nachtragen' : 'Teilnahme hinzufügen'}</h3>
            {attEditing && <p className="muted">Vorher: {playerName(attEditing.player_id)}, Charakter {attEditing.character_name || 'Nicht verfügbar'}, Klasse {attEditing.class || 'Nicht verfügbar'}, tatsächlicher Spec {attEditing.spec || 'Nicht verfügbar'}, Rolle {attEditing.role}. {attendanceFlags.map(({ key, label }) => `${label}: ${attEditing[key] ? 'ja' : 'nein'}`).join(', ')}. Notizen: {attEditing.notes || 'keine'}</p>}
            <form className="inline-form" onSubmit={addAttendance}>
              <select aria-label="Teilnahme-Spieler" disabled={pending} value={attPlayer} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                snapshotDirty.current.clear()
                setAttPlayer(e.target.value)
                setCharacters([])
                setAttCharacter('')
                setAttSnapshot({})
              }} required>
                <option value="">Spieler…</option>
                {players.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <select aria-label="Teilnahme-Charakter" disabled={pending || characters.length === 0} value={attCharacter} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                snapshotDirty.current.clear()
                setAttCharacter(e.target.value)
                const selected = characters.find((c) => c.id === Number(e.target.value))
                setAttSnapshot(selected ? { character_name: selected.name, class: selected.class, spec: selected.main_spec } : {})
              }} required>
                <option value="">Charakter…</option>
                {characters.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
              <label>Gespeicherter Charaktername<input disabled={pending} placeholder="Gespeicherter Charaktername" value={attSnapshot.character_name ?? ''} onChange={(e) => changeSnapshot('character_name', e.target.value)} /></label>
              <label>Gespeicherte Klasse<input disabled={pending} placeholder="Gespeicherte Klasse" value={attSnapshot.class ?? ''} onChange={(e) => changeSnapshot('class', e.target.value)} /></label>
              <label>Tatsächlicher Spec<input disabled={pending} placeholder="Tatsächlicher Spec" value={attSnapshot.spec ?? ''} onChange={(e) => changeSnapshot('spec', e.target.value)} /></label>
              {characters.find((c) => c.id === Number(attCharacter))?.second_spec && <button disabled={pending} type="button" onClick={() => changeSnapshot('spec', characters.find((c) => c.id === Number(attCharacter))!.second_spec)}>Second-Spec übernehmen</button>}
              <select aria-label="Teilnahme-Rolle" disabled={pending} value={attRole} onChange={(e) => setAttRole(e.target.value as AttendanceRole)}>
                <option value="TANK">Tank</option>
                <option value="HEAL">Heal</option>
                <option value="DD">DD</option>
              </select>
              {attendanceFlags.map(({ key, label }) => <label className="check" key={key}>
                <input type="checkbox" disabled={pending} checked={attDetails[key]} onChange={(e) => {
                  const checked = e.target.checked
                  setAttDetails((previous) => ({ ...previous, [key]: checked }))
                }} /> {label}
              </label>)}
              <label>Notizen<input disabled={pending} placeholder="Teilnahme-Notizen" value={attDetails.notes} onChange={(e) => {
                const notes = e.target.value
                setAttDetails((previous) => ({ ...previous, notes }))
              }} /></label>
              {attNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Teilnahme-Begründung" value={attReason} onChange={(e) => setAttReason(e.target.value)} required /></label>}
              <button type="submit" disabled={pending || !hasAttCharacter || (attNeedsReason && !attReason.trim())}>{attEditing ? 'Teilnahme-Korrektur speichern' : 'Hinzufügen'}</button>
              {attEditing && <button type="button" disabled={pending} onClick={() => editAttendance(null)}>Bearbeitung abbrechen</button>}
            </form>
          </div>
        )}
      </section>

      <section className="card">
        <h2>Loot ({loot.length})</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Item</th>
              <th>Boss</th>
              <th>Spieler</th>
              <th>Spec</th>
              {canWrite && <th></th>}
            </tr>
          </thead>
          <tbody>
            {loot.map((l) => (
              <tr key={l.id}>
                <td>{l.item_name}</td>
                <td>{l.boss || '—'}</td>
                <td>{playerName(l.awarded_to_player_id)}</td>
                <td>{l.is_main_spec ? 'Main' : 'Second'}</td>
                {canWrite && <td><button disabled={pending} onClick={() => editLoot(l)}>Loot korrigieren</button></td>}
              </tr>
            ))}
            {loot.length === 0 && (
              <tr>
                <td colSpan={canWrite ? 5 : 4} className="muted">
                  Kein Loot.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {canWrite && (
          <div className="subcard">
            <h3>{lootEditing ? `Loot #${lootEditing.id} korrigieren` : raid.status === 'COMPLETED' ? 'Loot nachtragen' : 'Loot vergeben'}</h3>
            {lootEditing && <p className="muted">Vorher: {lootEditing.item_name} (Item-ID: {lootEditing.item_id || 'keine'}), Boss: {lootEditing.boss || 'keiner'}, Empfänger: {playerName(lootEditing.awarded_to_player_id)}, Charakter: {lootEditing.awarded_to_character_id ? `#${lootEditing.awarded_to_character_id}` : 'keiner'}, {lootEditing.is_main_spec ? 'Main-Spec' : 'Second-Spec'}. Notizen: {lootEditing.notes || 'keine'}</p>}
            <form className="inline-form" onSubmit={addLoot}>
              <label>Item<input disabled={pending} placeholder="Item" value={lootItem} onChange={(e) => setLootItem(e.target.value)} required /></label>
              <label>Item-ID<input disabled={pending} placeholder="Item-ID" value={lootItemID} onChange={(e) => setLootItemID(e.target.value)} /></label>
              <label>Boss<input disabled={pending} placeholder="Boss" value={lootBoss} onChange={(e) => setLootBoss(e.target.value)} /></label>
              <select aria-label="Loot-Empfänger" disabled={pending} value={lootPlayer} onChange={(e) => {
                setLootPlayer(e.target.value)
                setLootCharacter('')
              }} required>
                <option value="">Spieler…</option>
                {players.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <label>Empfänger-Charakter-ID (optional)<input disabled={pending} type="number" min="1" step="1" placeholder="Loot-Charakter-ID" value={lootCharacter} onChange={(e) => setLootCharacter(e.target.value)} /></label>
              <label className="check">
                <input disabled={pending} type="checkbox" checked={lootMain} onChange={(e) => setLootMain(e.target.checked)} /> Main-Spec
              </label>
              <label>Notizen<input disabled={pending} placeholder="Loot-Notizen" value={lootNotes} onChange={(e) => setLootNotes(e.target.value)} /></label>
              {lootNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Loot-Begründung" value={lootReason} onChange={(e) => setLootReason(e.target.value)} required /></label>}
              <button type="submit" disabled={pending || (lootNeedsReason && !lootReason.trim())}>{lootEditing ? 'Loot-Korrektur speichern' : 'Vergeben'}</button>
              {lootEditing && <button type="button" disabled={pending} onClick={() => editLoot(null)}>Bearbeitung abbrechen</button>}
            </form>
          </div>
        )}
      </section>
    </div>
  )
}
