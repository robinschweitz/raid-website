import { useRef, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import type { Attendance, AttendanceRole, Player } from '../types'
import { SpecSelect } from './SpecSelect'

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
  const [name, setName] = useState('')
  const [owner, setOwner] = useState('new')
  const [playerName, setPlayerName] = useState<string | null>(null)
  const [characterClass, setClass] = useState('')
  const [spec, setSpec] = useState('')
  const [secondSpec, setSecondSpec] = useState('')
  const [role, setRole] = useState<AttendanceRole>('DD')
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const [uncertain, setUncertain] = useState(false)
  const request = useRef<{ payload: string; id: string } | null>(null)
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim() || !options[characterClass]?.includes(spec) || (owner === 'new' && !(playerName ?? name).trim()) || (needsReason && !reason.trim())) return
    await submit(async () => {
      const body = { name: name.trim(), class: characterClass, main_spec: spec, second_spec: secondSpec, role,
        player_id: owner === 'new' ? 0 : Number(owner), player_name: owner === 'new' ? (playerName ?? name).trim() : '', reason: needsReason ? reason.trim() : '' }
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
      <label>Charaktername<input required value={name} onChange={(event) => setName(event.target.value)} /></label>
      <label>Klasse<select required value={characterClass} onChange={(event) => { setClass(event.target.value); setSpec(''); setSecondSpec('') }}>
        <option value="">Klasse wählen…</option>{Object.keys(options).sort().map((value) => <option key={value}>{value}</option>)}
      </select></label>
      <SpecSelect label="Main-Spec" required value={spec} specs={options[characterClass] ?? []} onChange={setSpec} />
      <SpecSelect label="Second-Spec" value={secondSpec} specs={options[characterClass] ?? []} onChange={setSecondSpec} />
      <label>Spieler<select aria-label="Charakter-Besitzer" value={owner} onChange={(event) => setOwner(event.target.value)}>
        <option value="new">Neuer Spieler</option>{players.map((player) => <option key={player.id} value={player.id}>{player.name}</option>)}
      </select></label>
      {owner === 'new' && <label>Spielername<input required value={playerName ?? name} onChange={(event) => setPlayerName(event.target.value)} /></label>}
      <label>Rolle im Raid<select value={role} onChange={(event) => setRole(event.target.value as AttendanceRole)}><option value="DD">DD</option><option value="TANK">Tank</option><option value="HEAL">Heal</option></select></label>
      {needsReason && <label>Begründung (Pflicht)<input required value={reason} onChange={(event) => setReason(event.target.value)} /></label>}
    </fieldset>
    <p className="muted">Charakter und Teilnahme werden gemeinsam gespeichert. Neue Spieler benötigen hier keine Zugangsdaten.</p>
    <button type="submit" disabled={pending || !name.trim() || !spec || (needsReason && !reason.trim())}>Anlegen und zum Raid hinzufügen</button>
    <button type="button" disabled={pending || uncertain} onClick={onCancel}>Abbrechen</button>
  </form>
}
