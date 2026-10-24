import { useCallback, useEffect, useRef, useState, type FormEvent, type ReactElement } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { characterClassName } from '../characterTheme'
import { useSubmission } from '../useSubmission'
import { atLeast, type Attendance, type Character, type Player, type Raid, type RaidSignup, type SignupOverview, isDeclined } from '../types'

import { canonicalClass } from '../characterOptions'
import { myRaidStatus, signupList, type SignupGroupState } from '../myRaids'
import { ClassEmblem } from './ClassEmblem'
import { ErrorMessage } from './ErrorMessage'

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
  const [absenceReason, setAbsenceReason] = useState('')
  const [absenceTarget, setAbsenceTarget] = useState('')
  const [clock, setClock] = useState(Date.now())
  const loadVersion = useRef(0)
  const canManage = !!player && atLeast(player.role, 'RAIDASSISTENT')
  const canSignup = !!player && atLeast(player.role, 'MEMBER')
  const isActiveRaid = !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')
  const isFuture = !!raid.scheduled_start && Date.parse(raid.scheduled_start) > clock
  const isOpen = !!overview?.is_open && isActiveRaid && isFuture
  const base = `/api/v1/raids/${raid.id}/signups`
  const absenceBase = `/api/v1/raids/${raid.id}/absences`
  const absences = overview?.absences ?? []
  const ownAbsence = absences.find((row) => row.player_id === player?.id)
  const canDeclineOwn = canSignup && isActiveRaid && isFuture
  const myStatus = overview && player ? myRaidStatus(overview, player.id, attendance) : null
  const playerLabel = (id: number) => players.find((row) => row.id === id)?.name ?? `Spieler #${id}`
  const list = overview ? signupList(overview, attendance, playerLabel) : []
  const count = (state: SignupGroupState) => list.filter((row) => row.state === state).length

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

  const wasOpen = useRef(false)
  useEffect(() => {
    if (isOpen) { wasOpen.current = true; return }
    if (!wasOpen.current) return
    wasOpen.current = false
    const unsent = Object.keys(choices).length > 0
    // Staff closing the signup themselves already get a confirmation from the action.
    if (!canManage || unsent) setMessage(unsent ? 'Die Anmeldung wurde gerade geschlossen. Die angekreuzten Charaktere wurden nicht mehr angemeldet.' : 'Die Anmeldung wurde gerade geschlossen.')
    setChoices({})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen])

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
        setAbsenceReason('')
        setAbsenceTarget('')
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

  const ownSignup = <>
    {myStatus && canSignup && <p className="my-raid-status" aria-label="Dein Status">Dein Status: <span className={myStatus.state === 'absent' ? 'badge bad' : myStatus.state === 'tentative' ? 'badge warn' : myStatus.state === 'accepted' ? 'badge ok' : 'badge info'}>{myStatus.label}</span>{myStatus.detail && <> mit {myStatus.detail}</>}</p>}
    {canSignup && isOpen && overview && <form onSubmit={save}>
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
    {canDeclineOwn && <section className="subcard absence-panel" aria-label="Absage">
      <h3>Kannst du nicht kommen oder bist du unsicher?</h3>
      {ownAbsence && isDeclined(ownAbsence) ? <>
        <p><span className="badge bad">Du hast abgesagt</span>{ownAbsence.reason && <> · {ownAbsence.reason}</>}</p>
        <p className="muted">Die Raidleitung sieht deine Absage. Bis zum Raidstart kannst du sie zurücknehmen.</p>
        <button type="button" disabled={pending} onClick={() => { void mutate(() => api.del(`${absenceBase}/${player!.id}`), 'Absage zurückgenommen. Du bist wieder dabei.', true) }}>Doch dabei</button>
      </> : <>
        {ownAbsence && <>
          <p><span className="badge warn">Du bist unsicher</span>{ownAbsence.reason && <> · {ownAbsence.reason}</>}</p>
          <p className="muted">Die Raidleitung sieht, dass du noch nicht sicher bist. Sobald du es weißt, sag sicher zu oder ab.</p>
          <button type="button" disabled={pending} onClick={() => { void mutate(() => api.del(`${absenceBase}/${player!.id}`), 'Du bist sicher dabei.', true) }}>Sicher dabei</button>
        </>}
        {!ownAbsence && <p className="muted">Sag der Raidleitung Bescheid, damit sie planen kann. Deine Anmeldung bleibt bestehen, die Raidleitung entscheidet über die Aufstellung.</p>}
        <label>{ownAbsence ? 'Grund für die Absage (optional)' : 'Grund oder Hinweis (optional)'}
          <input value={absenceReason} maxLength={200} disabled={pending} onChange={(event) => setAbsenceReason(event.target.value)} placeholder="z. B. krank, Urlaub oder komme evtl. später" />
        </label>
        <div className="actions">
          <button type="button" disabled={pending} onClick={() => { void mutate(() => api.put(`${absenceBase}/${player!.id}`, { reason: absenceReason, status: 'DECLINED' }), 'Absage gesendet. Die Raidleitung sieht sie jetzt.', true) }}>Ich kann nicht kommen</button>
          {!ownAbsence && <button type="button" disabled={pending} onClick={() => { void mutate(() => api.put(`${absenceBase}/${player!.id}`, { reason: absenceReason, status: 'TENTATIVE' }), 'Als unsicher gemeldet. Die Raidleitung sieht es jetzt.', true) }}>Ich bin unsicher</button>}
        </div>
      </>}
    </section>}
  </>

  const stateBadge: Record<SignupGroupState, ReactElement> = {
    pending: <span className="badge info">Ausstehend</span>,
    accepted: <span className="badge ok">In der Aufstellung</span>,
    tentative: <span className="badge warn">Unsicher</span>,
    declined: <span className="badge bad">Hat abgesagt</span>,
  }

  const signupBoard = overview && <section className="signup-board" aria-label="Anmeldeliste">
    <h3>Anmeldeliste</h3>
    <p className="signup-counts" aria-label="Anmeldestand">
      <span className="badge info">{count('pending')} ausstehend</span>
      <span className="badge ok">{count('accepted')} in der Aufstellung</span>
      <span className="badge warn">{count('tentative')} unsicher</span>
      <span className="badge bad">{count('declined')} abgesagt</span>
    </p>
    {list.length === 0 && <p className="muted">Noch keine Anmeldungen.</p>}
    <ul className="signup-list">{list.map((row) => <li key={row.playerID} className={`signup-row signup-row-${row.state}`}>
      <div className="signup-player">
        <strong>{row.playerName}</strong> {stateBadge[row.state]}
        {row.reason && <span className="muted"> · {row.reason}</span>}
        {canManage && isActiveRaid && (row.state === 'declined' || row.state === 'tentative') && <button type="button" disabled={pending} onClick={() => { void mutate(() => api.del(`${absenceBase}/${row.playerID}`), `${row.playerName} ist wieder dabei.`, true) }}>{row.state === 'declined' ? 'Absage zurücknehmen' : 'Sicher dabei'}</button>}
      </div>
      {(row.entries.length > 0 || row.directRoster.length > 0) && <ul className="signup-characters">
        {row.entries.map((entry) => <li key={entry.id} className="signup-character">
          <span className="signup-character-name"><ClassEmblem characterClass={entry.class} /><strong className={characterClassName(entry.class)}>{entry.character_name}</strong></span>
          <span className="muted">{entry.class} · {entry.spec}</span>
          {(entry.is_accepted || entry.attendance_id > 0 || row.state !== 'pending') && <span className={entry.is_accepted ? 'badge ok' : entry.attendance_id ? 'badge' : 'badge info'}>{entry.is_accepted ? 'Bestätigt' : entry.attendance_id ? 'Alternative' : 'Ausstehend'}</span>}
          <span className="signup-character-actions">
            {isOpen && canSignup && row.playerID === player?.id && !entry.is_accepted && <button type="button" disabled={pending} onClick={() => {
              void mutate(() => api.post(`${base}/${entry.id}/withdraw`), 'Anmeldung zurückgezogen.')
            }}>Zurückziehen</button>}
            {canManage && isActiveRaid && !entry.is_accepted && <button type="button" className="primary-action" disabled={pending} onClick={() => {
              if (entry.attendance_id) setSwitchEntry(entry)
              else accept(entry)
            }}>{entry.attendance_id ? 'Auf diesen Charakter wechseln' : 'In Aufstellung übernehmen'}</button>}
          </span>
          {switchEntry?.id === entry.id && <div className="signup-switch">
            <p>Den bisherigen Charakter dieses Spielers in der Aufstellung durch {entry.character_name} ({entry.spec}) ersetzen?</p>
            <button type="button" disabled={pending} onClick={() => accept(entry, true)}>Charakterwechsel bestätigen</button>
            <button type="button" disabled={pending} onClick={() => setSwitchEntry(null)}>Abbrechen</button>
          </div>}
        </li>)}
        {row.directRoster.map((roster) => <li key={`roster-${roster.id}`} className="signup-character">
          <span className="signup-character-name"><ClassEmblem characterClass={roster.class ?? ''} /><strong className={characterClassName(roster.class ?? '')}>{roster.character_name ?? `Charakter #${roster.character_id}`}</strong></span>
          <span className="muted">{roster.class ?? 'Klasse unbekannt'} · {roster.spec ?? 'Spec unbekannt'}</span>
          <span className="badge ok">Von der Raidleitung eingetragen</span>
        </li>)}
      </ul>}
    </li>)}</ul>
    {canManage && isActiveRaid && <div className="actions signup-mark">
      <label>Spieler markieren
        <select aria-label="Spieler markieren" value={absenceTarget} disabled={pending} onChange={(event) => setAbsenceTarget(event.target.value)}>
          <option value="">Spieler wählen</option>
          {players.filter((row) => row.active && !absences.some((a) => a.player_id === row.id && isDeclined(a))).map((row) => <option key={row.id} value={row.id}>{row.name}</option>)}
        </select>
      </label>
      <button type="button" disabled={pending || !absenceTarget} onClick={() => { void mutate(() => api.put(`${absenceBase}/${absenceTarget}`, { reason: '', status: 'DECLINED' }), 'Spieler als abgesagt markiert.', true) }}>Als abgesagt markieren</button>
      <button type="button" disabled={pending || !absenceTarget} onClick={() => { void mutate(() => api.put(`${absenceBase}/${absenceTarget}`, { reason: '', status: 'TENTATIVE' }), 'Spieler als unsicher markiert.', true) }}>Als unsicher markieren</button>
    </div>}
  </section>

  return <section className="card raid-signups" aria-label="Raidanmeldungen">
    <h2>Anmeldungen</h2>
    {!canManage && <p className="muted">Melde einen oder mehrere eigene Charaktere mit deinem gewünschten Spec an. Die Raidleitung übernimmt einen Charakter pro Spieler in die Aufstellung.</p>}
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {message && <p className="success-message" role="status"><span>{message}</span><button type="button" aria-label="Meldung schließen" onClick={() => setMessage('')}>Schließen</button></p>}
    {pending && <p role="status">Wird gespeichert…</p>}
    {!overview ? <p className="muted">Anmeldungen werden geladen…</p> : <>
      <div className="actions signup-toolbar">
        <span className="badge info">{isOpen ? 'Anmeldung offen' : 'Anmeldung geschlossen'}</span>
        {raid.scheduled_start && <span className="muted">Anmeldeschluss: {new Date(raid.scheduled_start).toLocaleString('de-DE')}</span>}
        {canManage && isActiveRaid && <button type="button" disabled={pending || (!overview.is_enabled && !isFuture)} onClick={() => {
          void mutate(() => api.put(`${base}/availability`, { is_enabled: !overview.is_enabled }), overview.is_enabled ? 'Anmeldungen geschlossen.' : 'Anmeldungen geöffnet.')
        }}>{overview.is_enabled ? 'Anmeldungen schließen' : 'Anmeldungen öffnen'}</button>}
        <button type="button" disabled={pending} onClick={() => { void submit(async () => {
          try { await load(); await onRosterChange(); if (isActive()) setError('') }
          catch { if (isActive()) setError('Anmeldungen konnten nicht geladen werden.') }
        }) }}>Anmeldungen aktualisieren</button>
      </div>
      {!raid.scheduled_start && <p className="muted">Vor dem Öffnen bitte unter „Planung & Details“ einen zukünftigen Start speichern.</p>}
      {canManage ? <>
        {signupBoard}
        {canSignup && (isOpen || canDeclineOwn) && <details className="subcard own-signup">
          <summary>Eigene Anmeldung{myStatus ? ` · ${myStatus.label}` : ''}</summary>
          {ownSignup}
        </details>}
      </> : <>
        {ownSignup}
        {signupBoard}
      </>}
    </>}
  </section>
}
