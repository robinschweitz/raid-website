import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Attendance, type AttendanceRole, type Character, type LootAward, type Player, type Raid } from '../types'

interface Props {
  raidID: number
}

const attendanceFlags = [
  { key: 'is_substitute', label: 'Ersatz' },
  { key: 'is_bench', label: 'Bank' },
  { key: 'late_join', label: 'Später dabei' },
  { key: 'early_leave', label: 'Früher gegangen' },
] as const

function raidError(err: unknown, fallback: string): string {
  if (!(err instanceof ApiError)) return fallback
  if (err.code === 'reason_required') {
    return 'Für eine Korrektur ist eine nichtleere Begründung erforderlich.'
  }
  if (err.code === 'raid_state') return 'Der Raid erlaubt diese Änderung nicht mehr. Bitte die Seite neu laden.'
  return err.message
}

export function RaidDetailPage({ raidID }: Props) {
  return <RaidDetail key={raidID} raidID={raidID} />
}

function RaidDetail({ raidID }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const characterVersion = useRef(0)
  const lootFormVersion = useRef(0)
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<LootAward[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')

  const [attPlayer, setAttPlayer] = useState('')
  const [attCharacter, setAttCharacter] = useState('')
  const [attRole, setAttRole] = useState<AttendanceRole>('DD')
  const [characters, setCharacters] = useState<Character[]>([])
  const [attEditing, setAttEditing] = useState<Attendance | null>(null)
  const [attDetails, setAttDetails] = useState({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
  const [attReason, setAttReason] = useState('')

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
      const [r, a, l, p] = await Promise.all([
        api.get<Raid>(`/api/v1/raids/${raidID}`),
        api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
        api.get<LootAward[]>(`/api/v1/raids/${raidID}/loot`),
        api.get<Player[]>('/api/v1/players'),
      ])
      if (isActive() && version === loadVersion.current) {
        setRaid(r)
        setAttendance(a)
        setLoot(l)
        setPlayers(p)
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
      })
      .catch((err) => {
        if (!isActive() || obsolete || version !== characterVersion.current) return
        setCharacters([])
        setAttCharacter('')
        setError(err instanceof ApiError ? err.message : 'Charaktere laden fehlgeschlagen')
      })
    return () => { obsolete = true }
  }, [attPlayer, attEditing?.id, isActive])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`

  const editAttendance = (row: Attendance | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++characterVersion.current
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
    await submit(async () => {
      setError('')
      try {
        const body = {
          player_id: Number(attPlayer),
          character_id: Number(attCharacter),
          role: attRole,
          ...attDetails,
          ...(attNeedsReason ? { reason: attReason.trim() } : {}),
        }
        if (attEditing) {
          await api.patch<Attendance>(`/api/v1/raids/${raidID}/attendance/${attEditing.id}`, body)
        } else {
          await api.post<Attendance>(`/api/v1/raids/${raidID}/attendance`, body)
        }
        if (!isActive()) return
        ++characterVersion.current
        setAttPlayer('')
        setCharacters([])
        setAttCharacter('')
        setAttEditing(null)
        setAttRole('DD')
        setAttDetails({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
        setAttReason('')
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

  const action = async (name: 'complete' | 'cancel' | 'archive') => {
    if (!canManage || !raid || raid.archived_at || (name !== 'archive' && !isActiveRaid)) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Raid>(`/api/v1/raids/${raidID}/${name}`)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktion fehlgeschlagen')
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
        {raid.name} <span className="badge">{raid.status}</span>
      </h1>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}
      {canWrite && <p className="muted">Änderungen bestehender Einträge erfordern eine Begründung und werden mit Vorher/Nachher protokolliert. Bei abgeschlossenen Raids gilt das auch für Ergänzungen; der Server gleicht betroffene EP und Tokens aus.</p>}

      {canManage && !raid.archived_at && (
        <div className="actions">
          {isActiveRaid && (
            <button disabled={pending} onClick={() => action('complete')}>Abschließen</button>
          )}
          {isActiveRaid && <button disabled={pending} onClick={() => action('cancel')}>Absagen</button>}
          {!raid.archived_at && <button disabled={pending} onClick={() => action('archive')}>Archivieren</button>}
        </div>
      )}

      <section className="card">
        <h2>Teilnahme ({attendance.length})</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Spieler</th>
              <th>Charakter</th>
              <th>Rolle</th>
              {canWrite && <th></th>}
            </tr>
          </thead>
          <tbody>
            {attendance.map((a) => (
              <tr key={a.id}>
                <td>{playerName(a.player_id)}</td>
                <td>#{a.character_id}</td>
                <td>{a.role}</td>
                {canWrite && <td><button disabled={pending} onClick={() => editAttendance(a)}>Teilnahme korrigieren</button></td>}
              </tr>
            ))}
            {attendance.length === 0 && (
              <tr>
                <td colSpan={canWrite ? 4 : 3} className="muted">
                  Keine Einträge.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        {canWrite && (
          <div className="subcard">
            <h3>{attEditing ? `Teilnahme #${attEditing.id} korrigieren` : raid.status === 'COMPLETED' ? 'Teilnahme nachtragen' : 'Teilnahme hinzufügen'}</h3>
            {attEditing && <p className="muted">Vorher: {playerName(attEditing.player_id)}, Charakter #{attEditing.character_id}, Rolle {attEditing.role}. {attendanceFlags.map(({ key, label }) => `${label}: ${attEditing[key] ? 'ja' : 'nein'}`).join(', ')}. Notizen: {attEditing.notes || 'keine'}</p>}
            <form className="inline-form" onSubmit={addAttendance}>
              <select aria-label="Teilnahme-Spieler" disabled={pending} value={attPlayer} onChange={(e) => {
                ++characterVersion.current
                setAttPlayer(e.target.value)
                setCharacters([])
                setAttCharacter('')
              }} required>
                <option value="">Spieler…</option>
                {players.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <select aria-label="Teilnahme-Charakter" disabled={pending || characters.length === 0} value={attCharacter} onChange={(e) => setAttCharacter(e.target.value)} required>
                <option value="">Charakter…</option>
                {characters.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
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
