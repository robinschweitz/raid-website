import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import type { ImportRecord, ImportReport } from '../types'

const DEFAULT_SHEET_ID = '1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU'

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleString() : '—'
}

export function ImportsPage() {
  const [records, setRecords] = useState<ImportRecord[]>([])
  const [report, setReport] = useState<ImportReport | null>(null)
  const [sheetID, setSheetID] = useState(DEFAULT_SHEET_ID)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const { pending: busy, submit: submitRequest, isActive } = useSubmission()
  const [previewSheet, setPreviewSheet] = useState<string | null>(null)
  const sourceVersion = useRef(0)
  const renderedSourceVersion = sourceVersion.current
  const needsReason = report?.warnings?.some((warning) => warning.includes('reason_required')) ?? false
  const canMerge = previewSheet === sheetID && report?.mode === 'dry_run' && !!report.raw_hash?.trim()
    && !report.errors?.length && (!needsReason || !!reason.trim())

  const load = useCallback(() => {
    api
      .get<ImportRecord[]>('/api/v1/imports')
      .then((rs) => { if (isActive()) setRecords(rs) })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(load, [load])

  const run = async (mode: 'dry-run' | 'merge') => {
    if (renderedSourceVersion !== sourceVersion.current || (mode === 'merge' && !canMerge)) return
    await submitRequest(async () => {
      const source = sheetID
      const version = sourceVersion.current
      setPreviewSheet(null)
      setError('')
      setReport(null)
      try {
        const path = mode === 'dry-run' ? '/api/v1/imports/dry-run' : '/api/v1/imports'
        const rep = await api.post<ImportReport>(path, {
          sheet_id: source,
          ...(mode === 'merge' ? { raw_hash: report?.raw_hash, reason: reason.trim() } : {}),
        })
        if (!isActive() || version !== sourceVersion.current) return
        setReport(rep)
        if (mode === 'dry-run' && rep.mode === 'dry_run' && rep.raw_hash?.trim() && !rep.errors?.length) setPreviewSheet(source)
        if (mode === 'merge') {
          load()
        }
      } catch (err) {
        if (isActive() && version === sourceVersion.current) {
          setError(err instanceof ApiError && (err.code === 'source_changed' || err.code === 'preview_required')
            ? 'Die Quelle hat sich geändert oder die Vorschau ist nicht mehr gültig. Bitte eine neue Vorschau erstellen.'
            : err instanceof ApiError ? err.message : 'Import fehlgeschlagen')
        }
      }
    })
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    run('dry-run')
  }

  return (
    <div>
      <h1>Importe</h1>
      {error && <p className="error">{error}</p>}

      <form className="card" onSubmit={submit}>
        <h2>Google-Sheet importieren</h2>
        <label>
          Sheet-ID
          <input disabled={busy} value={sheetID} onChange={(e) => {
            ++sourceVersion.current
            setSheetID(e.target.value)
            setPreviewSheet(null)
            setReport(null)
            setError('')
          }} required />
        </label>
        <label>
          Begründung für historische Ergänzungen {needsReason && '(für diese Zusammenführung erforderlich)'}
          <input disabled={busy} aria-required={needsReason} placeholder="Import-Begründung" value={reason} onChange={(e) => setReason(e.target.value)} />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy}>
            Vorschau (Dry-Run)
          </button>
          <button type="button" disabled={busy || !canMerge} onClick={() => run('merge')}>
            Zusammenführen
          </button>
        </div>
        <p className="muted">Vor dem Zusammenführen ist eine fehlerfreie Vorschau erforderlich. Der Server prüft den Vorschau-Hash beim erneuten Laden der Quelle. Hat sich die Quelle geändert, ist eine neue Vorschau nötig.</p>
        <p className="muted">Ergänzungen zu bereits importierten Raids benötigen eine ausdrückliche Begründung für das Audit-Protokoll. Beim Erstimport darf sie leer bleiben. Die Vorschau kann ohne Begründung erstellt werden.</p>
        {needsReason && previewSheet === sheetID && <p className="muted" role="status">Die Vorschau ist gültig, aber vor dem Zusammenführen ist eine nichtleere Begründung erforderlich.</p>}
        <p className="muted">Für neue Loot-Zeilen nach einem Import bitte im Loot Archive die optionale Spalte „Import ID“ verwenden: pro neuer Zeile eine eindeutige, dauerhaft unveränderte ID. Bereits importierte Zeilen unverändert lassen; IDs nicht wiederverwenden oder ändern. Neue Zeilen ohne stabile ID werden nach einem früheren Import als mehrdeutig abgelehnt. Konflikte stehen im Bericht, bestehende Daten werden nicht überschrieben.</p>
        {busy && <p className="muted" role="status">Import wird verarbeitet…</p>}
      </form>

      {report && (
        <section className="card">
          <h2>
            Bericht <span className="badge">{report.skipped ? 'übersprungen' : report.mode === 'dry_run' ? 'Vorschau' : 'Zusammenführung'}</span>
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
          <h3>{report.skipped ? 'Einträge des früheren Imports' : report.mode === 'dry_run' ? 'Geplante neue Einträge' : 'Neu angelegte Einträge'}</h3>
          {report.mode === 'dry_run' && <p className="muted">Die Vorschau legt keine Einträge an.</p>}
          {report.skipped && <p className="muted">Diese Quelle wurde bereits importiert. Die Zahlen stammen aus dem früheren Bericht; jetzt wurden keine neuen Einträge angelegt.</p>}
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
        <h2>Import-Historie</h2>
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
                  Keine Importe.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>
    </div>
  )
}
