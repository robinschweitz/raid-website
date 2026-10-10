import { useRef, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import type { Attendance, AttendanceRole, ExternalCharacter, Player } from '../types'

interface Props {
  raidID: number
  players: Player[]
  options: Record<string, string[]>
  needsReason: boolean
  onAdded: () => Promise<void>
  onCancel: () => void
  pending: boolean
  submit: (action: () => Promise<void>) => Promise<void>
  isActive: () => boolean
}

export function RaidQuickAdd({ raidID, players, options, needsReason, onAdded, onCancel, pending, submit, isActive }: Props) {
  void options
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ExternalCharacter[]>([])
  const [selected, setSelected] = useState<ExternalCharacter | null>(null)
  const [owner, setOwner] = useState('new')
  const [playerName, setPlayerName] = useState<string | null>(null)
  const [role, setRole] = useState<AttendanceRole>('DD')
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const [uncertain, setUncertain] = useState(false)
  const request = useRef<{ payload: string; id: string } | null>(null)
  const search = async () => {
    if (query.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setSelected(null)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(query.trim())}`)
        if (!isActive()) return
        setResults(rows)
        if (rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können angelegt werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen')
      }
    })
  }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected || (owner === 'new' && !(playerName ?? selected.name).trim()) || (needsReason && !reason.trim())) return
    await submit(async () => {
      const body = { profile_id: selected.profile_id, role,
        player_id: owner === 'new' ? 0 : Number(owner), player_name: owner === 'new' ? (playerName ?? selected.name).trim() : '', reason: needsReason ? reason.trim() : '' }
      const payload = JSON.stringify(body)
      if (!request.current || request.current.payload !== payload) request.current = { payload, id: crypto.randomUUID() }
      setError('')
      try {
        await api.post<Attendance>(`/api/v1/raids/${raidID}/quick-add`, { ...body, request_id: request.current.id })
        if (isActive()) await onAdded()
      } catch (err) {
        if (!isActive()) return
        // An unknown outcome must be retried with the same payload and key.
        const unknown = !(err instanceof ApiError) || err.status >= 500
        setUncertain(unknown)
        setError(unknown ? 'Antwort nicht verfügbar. Bitte unverändert erneut speichern; der Eintrag wird nicht doppelt angelegt.' : err.message)
      }
    })
  }
  return <form className="subcard" onSubmit={save} aria-label="Neuen Charakter zum Raid hinzufügen">
    <h3>Neuen Charakter anlegen</h3>
    {error && <p className="error" role="alert">{error}</p>}
    <fieldset disabled={pending || uncertain} className="inline-form">
      <label>Rising-Gods-Charakter<input required value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
      <button type="button" disabled={pending || query.trim().length < 3} onClick={() => void search()}>Suchen</button>
      <label>Spieler<select aria-label="Charakter-Besitzer" value={owner} onChange={(event) => setOwner(event.target.value)}>
        <option value="new">Neuer Spieler</option>{players.map((player) => <option key={player.id} value={player.id}>{player.name}</option>)}
      </select></label>
      {owner === 'new' && <label>Spielername<input required value={playerName ?? selected?.name ?? ''} onChange={(event) => setPlayerName(event.target.value)} /></label>}
      <label>Rolle im Raid<select value={role} onChange={(event) => setRole(event.target.value as AttendanceRole)}><option value="DD">DD</option><option value="TANK">Tank</option><option value="HEAL">Heal</option></select></label>
      {needsReason && <label>Begründung (Pflicht)<input required value={reason} onChange={(event) => setReason(event.target.value)} /></label>}
    </fieldset>
    {results.length > 0 && <div className="plain-list">{results.map((row) => <button key={row.profile_id} type="button" className={selected?.profile_id === row.profile_id ? 'wishlist-toggle active' : 'wishlist-toggle'} onClick={() => setSelected(row)}>{row.name} · {row.class} · {row.main_spec || 'Spec unbekannt'} · Stufe {row.level || '—'}</button>)}</div>}
    {selected && <p className="muted">Ausgewählt: {selected.name} · {selected.class} · {selected.main_spec}</p>}
    <p className="muted">Charakter und Teilnahme werden gemeinsam gespeichert. Neue Spieler benötigen hier keine Zugangsdaten.</p>
    <button type="submit" disabled={pending || !selected || (needsReason && !reason.trim())}>Anlegen und zum Raid hinzufügen</button>
    <button type="button" disabled={pending || uncertain} onClick={onCancel}>Abbrechen</button>
  </form>
}
