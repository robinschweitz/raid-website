import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { characterClassName } from '../characterTheme'
import { useSubmission } from '../useSubmission'
import { atLeast, type Attendance, type Character, type Player, type Raid, type RaidSignup, type SignupOverview } from '../types'

import { canonicalClass } from '../characterOptions'

interface Props {
  raid: Raid
  attendance: Attendance[]
  players: Player[]
  onRosterChange: () => Promise<void>
}

export function RaidSignups({ raid, attendance, players, onRosterChange }: Props) {
  const { player } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [overview, setOverview] = useState<SignupOverview | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [choices, setChoices] = useState<Record<number, string>>({})
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [switchEntry, setSwitchEntry] = useState<RaidSignup | null>(null)
  const [clock, setClock] = useState(Date.now())
  const loadVersion = useRef(0)
  const canManage = !!player && atLeast(player.role, 'RAIDASSISTENT')
  const canSignup = !!player && atLeast(player.role, 'MEMBER')
  const isActiveRaid = !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')
  const isFuture = !!raid.scheduled_start && Date.parse(raid.scheduled_start) > clock
  const isOpen = !!overview?.is_open && isActiveRaid && isFuture
  const base = `/api/v1/raids/${raid.id}/signups`

  const load = useCallback(async () => {
    const version = ++loadVersion.current
    const [data, own] = await Promise.all([
      api.get<SignupOverview>(base),
      player ? api.get<Character[]>(`/api/v1/players/${player.id}/characters`) : Promise.resolve([]),
    ])
    if (!isActive() || version !== loadVersion.current) return
    setOverview(data)
    setCharacters(own)
    setClock(Date.now())
  }, [base, player, isActive])

  useEffect(() => {
    void load().catch((err: unknown) => {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Anmeldungen konnten nicht geladen werden.')
    })
  }, [load, raid.scheduled_start, raid.status, isActive])

  useEffect(() => {
    if (!message) return undefined
    const timer = setTimeout(() => setMessage(''), 8000)
    return () => clearTimeout(timer)
  }, [message])

  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  async function mutate(work: () => Promise<unknown>, success: string, rosterChanged = false) {
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        await work()
        if (!isActive()) return
        setChoices({})
        setSwitchEntry(null)
        setMessage(success)
        await load()
        if (rosterChanged) await onRosterChange()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Speichern fehlgeschlagen. Bitte erneut laden.')
      }
    })
  }

  function save(event: FormEvent) {
    event.preventDefault()
    void mutate(() => api.put<SignupOverview>(base, {
      choices: Object.entries(choices).map(([characterID, spec]) => ({ character_id: Number(characterID), spec })),
    }), 'Ausgewählte Charaktere wurden angemeldet. Andere Anmeldungen bleiben erhalten.')
  }

  function accept(entry: RaidSignup, replace = false) {
    const current = attendance.find((row) => row.id === entry.attendance_id)
    void mutate(() => api.post(`${base}/${entry.id}/accept`, {
      replace_attendance_id: replace ? entry.attendance_id : 0,
      expected_character_id: replace ? current?.character_id ?? 0 : 0,
    }), 'Die Aufstellung wurde aktualisiert.', true)
  }

  return <section className="card" aria-label="Raidanmeldungen">
    <h2>Anmeldungen</h2>
    <p className="muted">Melde einen oder mehrere eigene Charaktere mit deinem gewünschten Spec an. Die Raidleitung übernimmt einen Charakter pro Spieler in die Aufstellung.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {message && <p className="success-message" role="status"><span>{message}</span><button type="button" aria-label="Meldung schließen" onClick={() => setMessage('')}>Schließen</button></p>}
    {pending && <p role="status">Wird gespeichert…</p>}
    <button type="button" disabled={pending} onClick={() => { void submit(async () => {
      try { await load(); await onRosterChange(); if (isActive()) setError('') }
      catch { if (isActive()) setError('Anmeldungen konnten nicht geladen werden.') }
    }) }}>Anmeldungen aktualisieren</button>
    {!overview ? <p className="muted">Anmeldungen werden geladen…</p> : <>
      <p><span className="badge info">{isOpen ? 'Anmeldung offen' : 'Anmeldung geschlossen'}</span></p>
      {raid.scheduled_start && <p className="muted">Anmeldeschluss: {new Date(raid.scheduled_start).toLocaleString('de-DE')} · Die Raidleitung kann früher schließen.</p>}
      {!raid.scheduled_start && <p className="muted">Vor dem Öffnen bitte unter „Planung & Details“ einen zukünftigen Start speichern.</p>}
      {canManage && isActiveRaid && <div className="actions">
        <button type="button" disabled={pending || (!overview.is_enabled && !isFuture)} onClick={() => {
          void mutate(() => api.put(`${base}/availability`, { is_enabled: !overview.is_enabled }), overview.is_enabled ? 'Anmeldungen geschlossen.' : 'Anmeldungen geöffnet.')
        }}>{overview.is_enabled ? 'Anmeldungen schließen' : 'Anmeldungen öffnen'}</button>
      </div>}
      {canSignup && isOpen && <form onSubmit={save}>
        <h3>Meine Charaktere anmelden / Spec ändern</h3>
        {!characters.length && <p className="muted">Deinem Account sind noch keine Charaktere zugeordnet.</p>}
        {characters.map((character) => {
          const entry = overview.entries.find((row) => row.character_id === character.id)
          const accepted = attendance.some((row) => row.player_id === player?.id && row.character_id === character.id)
          const specs = overview.class_specs[canonicalClass(character.class)] ?? []
          const selected = character.id in choices
          const defaultSpec = specs.includes(entry?.spec ?? '') ? entry!.spec : specs.includes(character.main_spec) ? character.main_spec : specs[0] ?? ''
          return <div className="subcard" key={character.id}>
            <label className={`check ${characterClassName(character.class)}`}>
              <input type="checkbox" checked={selected} disabled={pending || accepted || !specs.length} onChange={(event) => {
                const checked = event.target.checked
                setChoices((previous) => {
                  const next = { ...previous }
                  if (checked) next[character.id] = defaultSpec
                  else delete next[character.id]
                  return next
                })
              }} /> <span className="character-name">{character.name}</span> · {character.class}{accepted ? ' · Bestätigt (Änderungen durch Raidleitung)' : entry && !entry.is_withdrawn ? ' · Angemeldet' : ''}
            </label>
            <label>Gewünschter Spec für {character.name}
              <select disabled={pending || !selected || accepted} value={choices[character.id] ?? defaultSpec} onChange={(event) => setChoices((previous) => ({ ...previous, [character.id]: event.target.value }))}>
                {specs.map((spec) => <option key={spec} value={spec}>{spec}</option>)}
              </select>
            </label>
          </div>
        })}
        {characters.length > 0 && <>
          {!Object.keys(choices).length && !pending && <p className="field-hint">Mindestens einen Charakter ankreuzen.</p>}
          <button className="primary-action" disabled={pending || !Object.keys(choices).length}>Ausgewählte Charaktere anmelden</button>
        </>}
      </form>}
      <h3>Charakteroptionen nach Spieler</h3>
      {!overview.entries.some((entry) => !entry.is_withdrawn) && <p className="muted">Noch keine Anmeldungen.</p>}
      {[...new Set(overview.entries.filter((entry) => !entry.is_withdrawn).map((entry) => entry.player_id))].map((playerID) => <section className="subcard" key={playerID}>
        <h4>{players.find((row) => row.id === playerID)?.name ?? `Spieler #${playerID}`}</h4>
        {overview.entries.filter((entry) => entry.player_id === playerID && !entry.is_withdrawn).map((entry) => <div key={entry.id} className="subcard">
          <p><strong className={characterClassName(entry.class)}>{entry.character_name}</strong> · {entry.spec} · {entry.is_accepted ? 'Bestätigt' : entry.attendance_id ? 'Alternative' : 'Ausstehend'}</p>
          {isOpen && canSignup && playerID === player?.id && !entry.is_accepted && <button type="button" disabled={pending} onClick={() => {
            void mutate(() => api.post(`${base}/${entry.id}/withdraw`), 'Anmeldung zurückgezogen.')
          }}>Zurückziehen</button>}
          {canManage && isActiveRaid && !entry.is_accepted && <button type="button" className="primary-action" disabled={pending} onClick={() => {
            if (entry.attendance_id) setSwitchEntry(entry)
            else accept(entry)
          }}>{entry.attendance_id ? 'Auf diesen Charakter wechseln' : 'In Aufstellung übernehmen'}</button>}
          {switchEntry?.id === entry.id && <div>
            <p>Den bisherigen Charakter dieses Spielers in der Aufstellung durch {entry.character_name} ({entry.spec}) ersetzen?</p>
            <button type="button" disabled={pending} onClick={() => accept(entry, true)}>Charakterwechsel bestätigen</button>
            <button type="button" disabled={pending} onClick={() => setSwitchEntry(null)}>Abbrechen</button>
          </div>}
        </div>)}
      </section>)}
    </>}
  </section>
}
