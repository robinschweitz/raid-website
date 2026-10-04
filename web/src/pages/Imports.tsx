import { useEffect, useState, type FormEvent } from 'react'
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
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const load = () => {
    api
      .get<ImportRecord[]>('/api/v1/imports')
      .then(setRecords)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const run = async (mode: 'dry-run' | 'merge') => {
    setError('')
    setBusy(true)
    try {
      const path = mode === 'dry-run' ? '/api/v1/imports/dry-run' : '/api/v1/imports'
      const rep = await api.post<ImportReport>(path, { sheet_id: sheetID })
      setReport(rep)
      if (mode === 'merge') {
        load()
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Import fehlgeschlagen')
    } finally {
      setBusy(false)
    }
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
          <input value={sheetID} onChange={(e) => setSheetID(e.target.value)} required />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy}>
            Vorschau (Dry-Run)
          </button>
          <button type="button" disabled={busy} onClick={() => run('merge')}>
            Zusammenführen
          </button>
        </div>
      </form>

      {report && (
        <section className="card">
          <h2>
            Bericht <span className="badge">{report.skipped ? 'übersprungen' : report.mode}</span>
          </h2>
          <ul className="plain-list">
            <li>Zeilen gelesen: {report.rows_read}</li>
            <li>Spieler: {report.players}</li>
            <li>Charaktere: {report.characters}</li>
            <li>Raids: {report.raids}</li>
            <li>Teilnahmen: {report.attendance}</li>
            <li>Loot: {report.loot}</li>
            <li>Tokens: {report.tokens}</li>
          </ul>
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
