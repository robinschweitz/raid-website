import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import type { Character, ImportRecord, Player, SyncAdoption, SyncReport } from '../types'

const DEFAULT_SHEET_ID = '1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU'

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleString() : '—'
}

export function ImportsPage() {
  const [records, setRecords] = useState<ImportRecord[]>([])
  const [report, setReport] = useState<SyncReport | null>(null)
  const [sheetID, setSheetID] = useState(DEFAULT_SHEET_ID)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const [syncStatus, setSyncStatus] = useState('')
  const [adoption, setAdoption] = useState<SyncAdoption | null>(null)
  const [adoptionReason, setAdoptionReason] = useState('')
  const [adoptionPlayers, setAdoptionPlayers] = useState<Record<string, string>>({})
  const [adoptionCharacters, setAdoptionCharacters] = useState<Record<string, string>>({})
  const [players, setPlayers] = useState<Player[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [operation, setOperation] = useState<{ mode: 'dry-run' | 'apply'; startedAt: number } | null>(null)
  const [now, setNow] = useState(Date.now())
  const { pending: busy, submit: submitRequest, isActive } = useSubmission()
  const [previewSheet, setPreviewSheet] = useState<string | null>(null)
  const sourceVersion = useRef(0)
  const renderedSourceVersion = sourceVersion.current
  const needsReason = report?.warnings?.some((warning) => warning.includes('reason_required')) ?? false
  const canMerge = previewSheet === sheetID && report?.mode === 'dry_run' && !!report.raw_hash?.trim()
    && !report.errors?.length && (!needsReason || !!reason.trim())
  const elapsedSeconds = operation ? Math.max(0, Math.floor((now - operation.startedAt) / 1000)) : 0
  const operationText = operation?.mode === 'dry-run'
    ? elapsedSeconds < 5 ? 'Vorschau laedt Google-Sheet und prueft die Daten...'
      : elapsedSeconds < 15 ? 'Vorschau analysiert Raids, Charaktere, Loot und Punkte...'
        : 'Vorschau laeuft noch. Grosse Sheets koennen etwas dauern; bitte nicht neu starten.'
    : operation?.mode === 'apply'
      ? elapsedSeconds < 5 ? 'Website-Aenderungen werden an Google Sheets uebertragen...'
        : elapsedSeconds < 10 ? 'Synchronisierung laedt Quelle und prueft Vorschau-Hash...'
          : elapsedSeconds < 25 ? 'Datenbank schreibt Spieler, Charaktere, Raids, Loot und EP...'
            : 'Synchronisierung laeuft noch. Bitte warten; der Server schreibt atomar und verhindert doppelte Eintraege.'
      : ''

  const load = useCallback(() => {
    api
      .get<ImportRecord[]>('/api/v1/imports')
      .then((rs) => { if (isActive()) setRecords(rs) })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(load, [load])

  useEffect(() => {
    api.get<Player[]>('/api/v1/players').then((rows) => { if (isActive()) setPlayers(rows) }).catch(() => {})
    api.get<Character[]>('/api/v1/characters').then((rows) => { if (isActive()) setCharacters(rows) }).catch(() => {})
  }, [isActive])

  useEffect(() => {
    if (!operation) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [operation])

  const run = async (mode: 'dry-run' | 'apply') => {
    if (renderedSourceVersion !== sourceVersion.current || (mode === 'apply' && !canMerge)) return
    await submitRequest(async () => {
      const source = sheetID
      const version = sourceVersion.current
      setOperation({ mode, startedAt: Date.now() })
      setNow(Date.now())
      setError('')
      setSyncStatus('')
      if (mode === 'dry-run') {
        setPreviewSheet(null)
        setReport(null)
      }
      try {
        if (mode === 'apply') {
          const flushed = await api.post<{ sent: number; failed: number }>('/api/v1/website-sync/flush', {})
          if (flushed.failed > 0) {
            setSyncStatus(`${flushed.sent} Website-Änderungen synchronisiert, ${flushed.failed} fehlgeschlagen. Die eingehende Synchronisierung wurde nicht gestartet.`)
            return
          }
          setSyncStatus(`${flushed.sent} Website-Änderungen an Google Sheets übertragen.`)
        }
        if (mode === 'apply') {
          setPreviewSheet(null)
          setReport(null)
        }
        const path = mode === 'dry-run' ? '/api/v1/sync/dry-run' : '/api/v1/sync'
        const rep = await api.post<SyncReport>(path, {
          sheet_id: source,
          ...(mode === 'apply' ? { raw_hash: report?.raw_hash, reason: reason.trim() } : {}),
        })
        if (!isActive() || version !== sourceVersion.current) return
        setReport(rep)
        if (mode === 'dry-run' && rep.mode === 'dry_run' && rep.raw_hash?.trim() && !rep.errors?.length) setPreviewSheet(source)
        if (mode === 'apply') {
          load()
        }
      } catch (err) {
        if (isActive() && version === sourceVersion.current) {
          setError(err instanceof ApiError && (err.code === 'source_changed' || err.code === 'preview_required')
            ? 'Die Quelle hat sich geändert oder die Vorschau ist nicht mehr gültig. Bitte eine neue Vorschau erstellen.'
            : err instanceof ApiError ? err.message : 'Synchronisierung fehlgeschlagen')
        }
      } finally {
        if (isActive() && version === sourceVersion.current) setOperation(null)
      }
    })
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    run('dry-run')
  }

  const flushWebsiteSync = async () => {
    await submitRequest(async () => {
      setError('')
      try {
        const result = await api.post<{ sent: number; failed: number }>('/api/v1/website-sync/flush', {})
        if (isActive()) setSyncStatus(result.failed > 0
          ? `${result.sent} Website-Änderungen synchronisiert, ${result.failed} fehlgeschlagen. Bitte Berechtigung und Sheet prüfen, dann erneut versuchen.`
          : `${result.sent} Website-Änderungen mit Google Sheets synchronisiert.`)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Google-Sheets-Synchronisierung fehlgeschlagen.')
      }
    })
  }

  const previewAdoption = async () => {
    await submitRequest(async () => {
      setError('')
      try {
        const result = await api.post<SyncAdoption>('/api/v1/sync/adopt/dry-run', { sheet_id: sheetID })
        if (!isActive()) return
        setAdoption(result)
        setAdoptionPlayers({})
        setAdoptionCharacters({})
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Zuordnungsvorschau fehlgeschlagen.')
      }
    })
  }

  const applyAdoption = async () => {
    if (!adoption || !adoptionReason.trim()) return
    await submitRequest(async () => {
      setError('')
      try {
        const playersToMap = adoption.players.flatMap((candidate) => {
          const id = Number(adoptionPlayers[candidate.player_name])
          return id > 0 ? [{ player_name: candidate.player_name, player_id: id }] : []
        })
        const charactersToMap = adoption.characters.flatMap((candidate) => {
          const key = `${candidate.player_name}\u0000${candidate.character_name}`
          const id = Number(adoptionCharacters[key])
          return id > 0 ? [{ player_name: candidate.player_name, character_name: candidate.character_name, character_id: id }] : []
        })
        const result = await api.post<SyncAdoption>('/api/v1/sync/adopt', { sheet_id: sheetID, reason: adoptionReason.trim(), players: playersToMap, characters: charactersToMap })
        if (!isActive()) return
        setAdoption(result)
        setSyncStatus('Bestehende Raids und bestätigte Identitäten wurden auditiert verknüpft. Erstellen Sie jetzt eine Synchronisierungsvorschau.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Übernahme der Zuordnungen fehlgeschlagen.')
      }
    })
  }

  const playerName = (id: number) => players.find((player) => player.id === id)?.name ?? `Spieler #${id}`
  const characterName = (id: number) => characters.find((character) => character.id === id)?.name ?? `Charakter #${id}`

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Administration · Google Sheets</p><h1>Synchronisierung</h1></div></div>
      {error && <p className="error">{error}</p>}

      <form className="card import-workflow" onSubmit={submit}>
        <h2>Google Sheets synchronisieren</h2>
        <label>
          Sheet-ID
          <input disabled={busy} value={sheetID} onChange={(e) => {
            ++sourceVersion.current
            setSheetID(e.target.value)
            setPreviewSheet(null)
            setReport(null)
            setAdoption(null)
            setError('')
          }} required />
        </label>
        <label>
          Begründung für historische Ergänzungen {needsReason && '(für diese Synchronisierung erforderlich)'}
          <input disabled={busy} aria-required={needsReason} placeholder="Synchronisierungs-Begründung" value={reason} onChange={(e) => setReason(e.target.value)} />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy}>
            Vorschau synchronisieren
          </button>
          <button type="button" disabled={busy || !canMerge} onClick={() => run('apply')}>
            Synchronisierung anwenden
          </button>
        </div>
        <p className="muted">Vor dem Anwenden ist eine fehlerfreie Vorschau erforderlich. Website-Änderungen werden vor der eingehenden Synchronisierung übertragen. Der Server prüft den Vorschau-Hash beim erneuten Laden der Quelle.</p>
        {needsReason && previewSheet === sheetID && <p className="muted" role="status">Die Vorschau ist gültig, aber vor dem Anwenden ist eine nichtleere Begründung erforderlich.</p>}
        <details className="editor-disclosure"><summary>Hinweise zu historischen Ergänzungen und Import-IDs</summary>
        <p className="muted">Ergänzungen zu bereits synchronisierten historischen Raids sind auch im Archiv mit ausdrücklicher Begründung möglich. Bestehende Zeilen bleiben erhalten; Änderungen werden auditiert und EP-/Token-Differenzen als neue Buchungen erfasst. Abgesagte Raids bleiben gesperrt.</p>
        <p className="muted">Für neue Loot-Zeilen in historischen Daten bitte im Loot Archive die optionale Spalte „Import ID“ verwenden: pro neuer Zeile eine eindeutige, dauerhaft unveränderte ID. Bereits übernommene Zeilen unverändert lassen; IDs nicht wiederverwenden oder ändern. Neue Zeilen ohne stabile ID werden als mehrdeutig abgelehnt. Konflikte stehen im Bericht, bestehende Daten werden nicht überschrieben.</p>
        </details>
        {busy && operation && <div className="subcard" role="status" aria-live="polite">
          <h3>{operation.mode === 'dry-run' ? 'Vorschau läuft' : 'Synchronisierung läuft'}</h3>
          <p className="muted">{operationText}</p>
          <p className="muted">Dauer bisher: {elapsedSeconds}s</p>
        </div>}
      </form>

      <section className="card">
        <h2>Bestehende Historie verknüpfen</h2>
        <p className="muted">Übernimmt ausschließlich vorhandene Raids mit exakt gleichem Raid-Schlüssel. Mehrdeutige Besitzer müssen ausdrücklich bestätigt werden; bestehende Import-Provenienz wird nicht verändert.</p>
        <button type="button" disabled={busy} onClick={() => void previewAdoption()}>Zuordnungsvorschau laden</button>
        {adoption && <div className="subcard">
          <h3>Zuordnungsvorschau</h3>
          <p className="muted">Raids: {adoption.raids.filter((row) => row.mapped_raid_id || row.raid_ids.length === 1).length}/{adoption.raids.length} eindeutig.</p>
          {adoption.players.filter((row) => !row.mapped_player_id && (row.requires_selection || row.player_ids.length > 1)).map((row) => (
            <label key={row.player_name}>
              Besitzer für Sheet-Spieler „{row.player_name}“
              <select value={adoptionPlayers[row.player_name] ?? ''} onChange={(event) => setAdoptionPlayers({ ...adoptionPlayers, [row.player_name]: event.target.value })}>
                <option value="">Bitte auswählen</option>
                {row.player_ids.map((id) => <option key={id} value={id}>{playerName(id)}</option>)}
              </select>
            </label>
          ))}
          {adoption.characters.filter((row) => !row.mapped_character_id && row.characters.length > 1).map((row) => {
            const key = `${row.player_name}\u0000${row.character_name}`
            const selectedPlayer = Number(adoptionPlayers[row.player_name])
            const options = row.characters.filter((candidate) => !selectedPlayer || candidate.player_id === selectedPlayer)
            return <label key={key}>
              Charakter für „{row.character_name}“ ({row.player_name})
              <select value={adoptionCharacters[key] ?? ''} onChange={(event) => setAdoptionCharacters({ ...adoptionCharacters, [key]: event.target.value })}>
                <option value="">Bitte auswählen</option>
                {options.map((candidate) => <option key={candidate.character_id} value={candidate.character_id}>{characterName(candidate.character_id)} · {playerName(candidate.player_id)}</option>)}
              </select>
            </label>
          })}
          <label>
            Begründung für die Übernahme
            <input value={adoptionReason} onChange={(event) => setAdoptionReason(event.target.value)} placeholder="Historische Zuordnung geprüft" required />
          </label>
          <button type="button" disabled={busy || !adoptionReason.trim()} onClick={() => void applyAdoption()}>Zuordnungen auditiert übernehmen</button>
        </div>}
      </section>

      <section className="card">
        <h2>Ausgehende Änderungen erneut senden</h2>
        <p className="muted">Loot-Änderungen aus verknüpften Raids werden dauerhaft vorgemerkt. Dieser erneute Versand überträgt nur ausstehende oder fehlgeschlagene Website-Änderungen.</p>
        <button type="button" disabled={busy} onClick={() => void flushWebsiteSync()}>Ausgehende Änderungen erneut senden</button>
        {syncStatus && <p className="muted" role="status">{syncStatus}</p>}
      </section>

      {report && (
        <section className="card">
          <h2>
            Synchronisierungsbericht <span className="badge">{report.skipped ? 'übersprungen' : report.mode === 'dry_run' ? 'Vorschau' : 'angewendet'}</span>
          </h2>
          <ul className="plain-list">
            <li>Zeilen gelesen: {report.rows_read}</li>
            <li>Spieler: {report.players}</li>
            <li>Charaktere: {report.characters}</li>
            <li>Raids: {report.raids}</li>
            <li>Teilnahmen: {report.attendance}</li>
            <li>Loot: {report.loot}</li>
            <li>Tokens: {report.tokens}</li>
            <li>Punkteeinträge: {report.points_entries}</li>
          </ul>
          <h3>{report.skipped ? 'Einträge der früheren Synchronisierung' : report.mode === 'dry_run' ? 'Geplante Änderungen' : 'Übernommene Änderungen'}</h3>
          {report.mode === 'dry_run' && <p className="muted">Die Vorschau legt keine Einträge an.</p>}
          {report.skipped && <p className="muted">Diese Quelle wurde bereits synchronisiert. Die Zahlen stammen aus dem früheren Bericht; jetzt wurden keine neuen Einträge angelegt.</p>}
          <ul className="plain-list">
            <li>Spieler: {report.created.players}</li>
            <li>Charaktere: {report.created.characters}</li>
            <li>Raids: {report.created.raids}</li>
            <li>Teilnahmen: {report.created.attendance}</li>
            <li>Loot: {report.created.loot}</li>
            <li>Tokens: {report.created.tokens}</li>
            <li>EP-Transaktionen (einschließlich Korrekturen): {report.created.points}</li>
          </ul>
          {!!report.errors?.length && (
            <>
              <h3>Fehler ({report.errors.length})</h3>
              <p className="error">Der Bericht enthält Fehler. Es wurden keine Einträge geschrieben. {report.errors.some((message) => message.includes('reason_required'))
                ? 'Bitte eine Begründung eingeben und erneut eine Vorschau erstellen.'
                : 'Bitte die Quelle korrigieren und erneut eine Vorschau erstellen.'}</p>
              <ul className="plain-list scroll">
                {report.errors.map((message, i) => <li key={i} className="error">{message}</li>)}
              </ul>
            </>
          )}
          {report.warnings && report.warnings.length > 0 && (
            <>
              <h3>Warnungen ({report.warnings.length})</h3>
              <ul className="plain-list scroll">
                {report.warnings.map((w, i) => (
                  <li key={i} className="muted">
                    {w}
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      )}

      <section className="card">
        <h2>Historische Import-Historie</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Zeit</th>
              <th>Status</th>
              <th>Neu</th>
              <th>Warnungen</th>
              <th>Fehler</th>
            </tr>
          </thead>
          <tbody>
            {records.map((r) => (
              <tr key={r.id}>
                <td>{formatTime(r.completed_at ?? r.started_at)}</td>
                <td>{r.status}</td>
                <td>{r.created_count}</td>
                <td>{r.warnings_count}</td>
                <td>{r.errors_count}</td>
              </tr>
            ))}
            {records.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Keine historischen Importe.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    </div>
  )
}
