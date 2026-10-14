import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import type { ImportRecord, ImportReport } from '../types'

interface SheetSyncStatus {
  enabled: boolean
  automatic?: boolean
  sheet_id?: string
  last_run?: string
  last_success?: string
  last_error?: string
  current_setup?: { raid_id: number; added: number; warnings: string[]; conflicts: string[] }
  last_import?: ImportReport
}

interface SheetSyncConfig {
  enabled: boolean
  automatic: boolean
  sheet_id: string
  reason: string
  interval: string
  credentials_configured: boolean
}

const DEFAULT_SHEET_ID = '10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8'

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleString() : '—'
}

export function ImportsPage() {
  const [records, setRecords] = useState<ImportRecord[]>([])
  const [syncStatus, setSyncStatus] = useState<SheetSyncStatus | null>(null)
  const [syncConfig, setSyncConfig] = useState<SheetSyncConfig | null>(null)
  const [credentialJSON, setCredentialJSON] = useState('')
  const [credentialFilename, setCredentialFilename] = useState('')
  const [configBusy, setConfigBusy] = useState(false)
  const [configMessage, setConfigMessage] = useState('')
  const [syncBusy, setSyncBusy] = useState(false)
  const [syncPreview, setSyncPreview] = useState<{ ready: boolean; blockers: string[]; import?: ImportReport } | null>(null)
  const [report, setReport] = useState<ImportReport | null>(null)
  const [sheetID, setSheetID] = useState(DEFAULT_SHEET_ID)
  const [reason, setReason] = useState('')
  const [includeSetup, setIncludeSetup] = useState(true)
  const [setup, setSetup] = useState({ name: 'ICC25', leader: 'Oxora', first_date: '2026-10-05', start_time: '19:00', timezone: 'Europe/Berlin', weekdays: [1], interval_weeks: 1 })
  const [error, setError] = useState('')
  const [operation, setOperation] = useState<{ mode: 'dry-run' | 'merge'; startedAt: number } | null>(null)
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
    : operation?.mode === 'merge'
      ? elapsedSeconds < 5 ? 'Zusammenfuehrung laedt Quelle und prueft Vorschau-Hash...'
        : elapsedSeconds < 10 ? 'Rising-Gods-Verknuepfungen werden kurz vorbereitet...'
          : elapsedSeconds < 25 ? 'Datenbank schreibt Spieler, Charaktere, Raids, Loot und EP...'
            : 'Zusammenfuehrung laeuft noch. Bitte warten; der Server schreibt atomar und verhindert doppelte Imports.'
      : ''

  const invalidatePreview = () => {
    ++sourceVersion.current
    setPreviewSheet(null)
    setReport(null)
    setError('')
  }

  const load = useCallback(() => {
    api
      .get<ImportRecord[]>('/api/v1/imports')
      .then((rs) => { if (isActive()) setRecords(rs) })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen') })
  }, [isActive])

  useEffect(load, [load])

  const loadSync = useCallback(() => {
    api.get<SheetSyncStatus>('/api/v1/sheet-sync')
      .then((status) => { if (isActive()) setSyncStatus(status) })
      .catch(() => { if (isActive()) setSyncStatus(null) })
  }, [isActive])

  useEffect(loadSync, [loadSync])

  const loadConfig = useCallback(() => {
    api.get<SheetSyncConfig>('/api/v1/sheet-sync/config')
      .then((config) => { if (isActive()) setSyncConfig(config) })
      .catch((err) => { if (isActive()) setConfigMessage(err instanceof ApiError ? err.message : 'Sync-Einstellungen konnten nicht geladen werden') })
  }, [isActive])

  useEffect(loadConfig, [loadConfig])

  const saveSyncConfig = async (event: FormEvent) => {
    event.preventDefault()
    if (!syncConfig || configBusy || syncBusy || busy) return
    setConfigBusy(true)
    setConfigMessage('')
    try {
      const updated = await api.put<SheetSyncConfig>('/api/v1/sheet-sync/config', {
        ...syncConfig,
        service_account_json: credentialJSON,
      })
      if (!isActive()) return
      setSyncConfig(updated)
      setCredentialJSON('')
      setCredentialFilename('')
      setSyncPreview(null)
      setConfigMessage('Einstellungen gespeichert. Änderungen sind sofort wirksam.')
      loadSync()
    } catch (err) {
      if (isActive()) setConfigMessage(err instanceof ApiError ? err.message : 'Einstellungen konnten nicht gespeichert werden')
    } finally {
      if (isActive()) setConfigBusy(false)
    }
  }

  const checkSync = async () => {
    if (syncBusy || busy) return
    setSyncBusy(true)
    setSyncPreview(null)
    try {
      const result = await api.post<{ ready: boolean; blockers: string[]; import?: ImportReport }>('/api/v1/sheet-sync/preflight', {})
      if (isActive()) setSyncPreview(result)
    } catch (err) {
      if (isActive()) setSyncPreview({ ready: false, blockers: [err instanceof ApiError ? err.message : 'Vorabprüfung fehlgeschlagen'] })
    } finally { if (isActive()) setSyncBusy(false) }
  }

  const syncNow = async () => {
    if (syncBusy || busy) return
    setSyncBusy(true)
    setSyncPreview(null)
    try {
      await api.post<SheetSyncStatus>('/api/v1/sheet-sync/run', {})
    } catch {
      // The persisted status contains the actionable conflict, even when the
      // reconciliation endpoint returned a conflict HTTP status.
    } finally {
      if (isActive()) {
        setSyncBusy(false)
        loadSync()
        load()
      }
    }
  }

  useEffect(() => {
    if (!operation) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [operation])

  const run = async (mode: 'dry-run' | 'merge') => {
    if (renderedSourceVersion !== sourceVersion.current || (mode === 'merge' && !canMerge)) return
    await submitRequest(async () => {
      const source = sheetID
      const version = sourceVersion.current
      setOperation({ mode, startedAt: Date.now() })
      setNow(Date.now())
      setPreviewSheet(null)
      setError('')
      setReport(null)
      try {
        const path = mode === 'dry-run' ? '/api/v1/imports/dry-run' : '/api/v1/imports'
        const rep = await api.post<ImportReport>(path, {
          sheet_id: source,
          ...(includeSetup ? { setup } : {}),
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
      } finally {
        if (isActive() && version === sourceVersion.current) setOperation(null)
      }
    })
  }

  const submit = (event: FormEvent) => {
    event.preventDefault()
    run('dry-run')
  }

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Administration · Datenübernahme</p><h1>Importe</h1></div></div>
      {error && <p className="error">{error}</p>}

      <section className="card">
        <h2>Fortlaufende Google-Sheets-Synchronisierung</h2>
        <p className="muted">Im Sheet archivierte Raids werden einschließlich Loot und bereits verbuchter EP als abgeschlossen übernommen. Abschluss-EP werden dabei nicht nochmals vergeben. Das aktuelle ICC25 Raidsetup ergänzt nur eindeutig bekannte Teilnehmer des aktiven Raids. Die Website veröffentlicht ihre Daten in separaten „Website …“-Tabs. Widersprüche werden gemeldet, niemals automatisch überschrieben oder gelöscht.</p>
        {syncConfig && <form className="subcard" onSubmit={saveSyncConfig}>
          <h3>Synchronisierung einrichten</h3>
          <p className="muted">Diese Einstellungen werden serverseitig gespeichert. Die Google-Zugangsdaten werden nicht zurück an den Browser gesendet. Nur die freigegebene Arbeitskopie ist erlaubt.</p>
          <label className="check"><input type="checkbox" checked={syncConfig.enabled} disabled={configBusy || syncBusy} onChange={(e) => setSyncConfig({ ...syncConfig, enabled: e.target.checked, automatic: e.target.checked && syncConfig.automatic })} /> Synchronisierung aktivieren</label>
          <label className="check"><input type="checkbox" checked={syncConfig.automatic} disabled={configBusy || syncBusy || !syncConfig.enabled} onChange={(e) => setSyncConfig({ ...syncConfig, automatic: e.target.checked })} /> Automatisch regelmäßig abgleichen (erst nach erfolgreicher Vorabprüfung aktivieren)</label>
          <label>Freigegebene Sheet-ID
            <input value={syncConfig.sheet_id} readOnly aria-readonly="true" />
          </label>
          <label>Intervall (mindestens 1 Minute, z. B. 10m oder 1h)
            <input required value={syncConfig.interval} disabled={configBusy || syncBusy} onChange={(e) => setSyncConfig({ ...syncConfig, interval: e.target.value })} />
          </label>
          <label>Begründung für den Abgleich
            <input required={syncConfig.enabled} value={syncConfig.reason} disabled={configBusy || syncBusy} onChange={(e) => setSyncConfig({ ...syncConfig, reason: e.target.value })} placeholder="Geprüfter Abgleich mit der freigegebenen Kopie" />
          </label>
          <label>Google-Servicekonto als JSON-Datei (optional ersetzen)
            <input type="file" accept=".json,application/json" disabled={configBusy || syncBusy} onChange={(e) => {
              const file = e.currentTarget.files?.[0]
              e.currentTarget.value = ''
              if (!file) return
              if (file.size > 32 * 1024) {
                setConfigMessage('Die Servicekonto-Datei ist zu groß (maximal 32 KiB).')
                return
              }
              void file.text().then((content) => {
                setCredentialJSON(content)
                setCredentialFilename(file.name)
                setConfigMessage('')
              }).catch(() => setConfigMessage('Servicekonto-Datei konnte nicht gelesen werden'))
            }} />
          </label>
          <p className="muted">{credentialFilename ? 'Neue Servicekonto-Datei ausgewählt: ' + credentialFilename : syncConfig.credentials_configured ? 'Servicekonto vorhanden. Leer lassen, um es beizubehalten.' : 'Noch kein Servicekonto eingerichtet. Für die Aktivierung eine JSON-Datei auswählen.'}</p>
          <div className="actions"><button type="submit" disabled={configBusy || syncBusy || busy}>{configBusy ? 'Speichert…' : 'Einstellungen speichern'}</button></div>
          {configMessage && <p role="status" className="muted">{configMessage}</p>}
        </form>}
        {syncStatus?.enabled ? (
          <>
            <p className="muted">{syncStatus.automatic ? 'Automatischer Zeitplan läuft.' : 'Nur Prüf- und manueller Modus: kein automatischer Zeitplan.'}</p>
            <p role="status">{syncStatus.last_error ? 'Synchronisierung benötigt Prüfung' : syncStatus.last_success ? 'Letzter Abgleich erfolgreich' : 'Synchronisierung aktiviert'}</p>
            <p className="muted">Letzter Versuch: {formatTime(syncStatus.last_run ?? null)} · Letzter Erfolg: {formatTime(syncStatus.last_success ?? null)}</p>
            {!!syncStatus.last_error && <p className="error">{syncStatus.last_error}</p>}
            {syncStatus.current_setup && <p className="muted">Raidsetup: Raid #{syncStatus.current_setup.raid_id || '—'}, {syncStatus.current_setup.added} neue Teilnahmen.</p>}
            {!!syncStatus.current_setup?.warnings?.length && <ul className="plain-list">{syncStatus.current_setup.warnings.map((message, i) => <li key={i}>{message}</li>)}</ul>}
            {syncPreview && <p role="status">{syncPreview.ready ? 'Vorabprüfung bestanden. Der Abgleich prüft den Stand vor jedem Schreiben erneut.' : 'Aktivierung blockiert: Altbestand oder Quellenkonflikt zuerst prüfen.'}</p>}
            {!!syncPreview?.blockers.length && <ul className="plain-list">{syncPreview.blockers.map((message, i) => <li key={i}>{message}</li>)}</ul>}
            <p className="muted">Die Vorabprüfung zeigt Archivübernahmen und EP-Ausgleiche, ohne Daten zu verändern. Alte Import-Zuordnungen werden geprüft; widersprüchliche Aufstellungen, unbekannter Website-Loot oder parallele EP-Änderungen blockieren den Abgleich.</p>
            {!!syncPreview?.import?.archive_transfers?.length && <>
              <h3>Geplante Archivübernahme</h3>
              <ul className="plain-list">{syncPreview.import.archive_transfers.map((raid) => <li key={raid.raid_name}>{raid.raid_name} · Sheet-Archivdatum: {formatTime(raid.archived_at)}</li>)}</ul>
            </>}
            {!!syncPreview?.import?.point_transfers?.length && <div className="table-wrap">
              <table><caption>EP-Abgleich mit dem Sheet</caption><thead><tr><th>Spieler</th><th>Website bisher</th><th>Sheet-Zielstand</th><th>Ausgleich</th></tr></thead>
                <tbody>{syncPreview.import.point_transfers.map((entry) => <tr key={entry.player}><td>{entry.player}</td><td>{entry.before}</td><td>{entry.target}</td><td>{entry.adjustment > 0 ? '+' : ''}{entry.adjustment}</td></tr>)}</tbody>
              </table>
            </div>}
            {!!syncPreview?.import?.warnings?.length && <ul className="plain-list">{syncPreview.import.warnings.map((message, i) => <li key={i}>{message}</li>)}</ul>}
            <div className="actions">
              <button type="button" disabled={busy || syncBusy} onClick={checkSync}>Vorabprüfung ohne Änderungen</button>
              <button type="button" disabled={busy || syncBusy || !syncPreview?.ready} onClick={syncNow}>{syncBusy ? 'Abgleich läuft…' : 'Jetzt synchronisieren'}</button>
              <button type="button" disabled={syncBusy} onClick={loadSync}>Status aktualisieren</button>
            </div>
          </>
        ) : <p className="muted">Die Synchronisierung ist noch nicht aktiv. Richte sie oben ein, speichere die Einstellungen und führe zuerst eine Vorabprüfung durch.</p>}
      </section>

      <form className="card import-workflow" onSubmit={submit}>
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
        <label className="check"><input type="checkbox" disabled={busy} checked={includeSetup} onChange={(e) => { invalidatePreview(); setIncludeSetup(e.target.checked) }} />ICC25 Raidsetup (A–F) einmalig als Raid und Serie übernehmen</label>
        {includeSetup && <fieldset disabled={busy}>
          <legend>Erster Raid und wöchentliche Serie</legend>
          <p className="muted">Die Aufstellung stammt aus dem ersten nummerierten Block in A–F. Datum und Serienregeln werden hier ausdrücklich festgelegt. Eine bereits importierte Aufstellung wird nicht überschrieben.</p>
          <label>Name<input required value={setup.name} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, name: e.target.value }) }} /></label>
          <label>Raidleitung (Spielername)<input required value={setup.leader} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, leader: e.target.value }) }} /></label>
          <label>Erster Raid (Datum in der Serien-Zeitzone)<input required type="date" value={setup.first_date} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, first_date: e.target.value }) }} /></label>
          <label>Startzeit<input required type="time" value={setup.start_time} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, start_time: e.target.value }) }} /></label>
          <label>Zeitzone<input required value={setup.timezone} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, timezone: e.target.value }) }} /></label>
          <label>Wochentag<select value={setup.weekdays[0]} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, weekdays: [Number(e.target.value)] }) }}>{['Sonntag', 'Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag', 'Samstag'].map((day, i) => <option key={day} value={i}>{day}</option>)}</select></label>
          <label>Alle … Wochen<input required type="number" min="1" step="1" value={setup.interval_weeks} onChange={(e) => { invalidatePreview(); setSetup({ ...setup, interval_weeks: Number(e.target.value) }) }} /></label>
        </fieldset>}
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
        {needsReason && previewSheet === sheetID && <p className="muted" role="status">Die Vorschau ist gültig, aber vor dem Zusammenführen ist eine nichtleere Begründung erforderlich.</p>}
        <details className="editor-disclosure"><summary>Hinweise zu historischen Ergänzungen & Import-IDs</summary>
        <p className="muted">Ergänzungen zu bereits importierten Raids sind auch im Archiv mit ausdrücklicher Begründung möglich. Bestehende Zeilen bleiben erhalten; Änderungen werden auditiert und EP-/Token-Differenzen als neue Buchungen erfasst. Abgesagte Raids bleiben gesperrt. Beim Erstimport darf die Begründung leer bleiben.</p>
        <p className="muted">Für neue Loot-Zeilen nach einem Import bitte im Loot Archive die optionale Spalte „Import ID“ verwenden: pro neuer Zeile eine eindeutige, dauerhaft unveränderte ID. Bereits importierte Zeilen unverändert lassen; IDs nicht wiederverwenden oder ändern. Neue Zeilen ohne stabile ID werden nach einem früheren Import als mehrdeutig abgelehnt. Konflikte stehen im Bericht, bestehende Daten werden nicht überschrieben.</p>
        </details>
        {busy && operation && <div className="subcard" role="status" aria-live="polite">
          <h3>{operation.mode === 'dry-run' ? 'Vorschau laeuft' : 'Zusammenfuehrung laeuft'}</h3>
          <p className="muted">{operationText}</p>
          <p className="muted">Dauer bisher: {elapsedSeconds}s</p>
        </div>}
      </form>

      {report && (
        <section className="card">
          <h2>
            Bericht <span className="badge">{report.skipped ? 'übersprungen' : report.mode === 'dry_run' ? 'Vorschau' : 'Zusammenführung'}</span>
          </h2>
          {report.setup && <div className="subcard"><h3>{report.setup.already_imported ? 'Bereits übernommene Raidaufstellung' : 'Geplante Raidaufstellung'}</h3><p>{report.setup.name} · Raidleitung: {report.setup.leader} · {new Date(report.setup.scheduled_start).toLocaleString('de-DE')} (lokale Anzeige)</p><p>{report.setup.attendance} Teilnahmen · Serien-Zeitzone: {report.setup.timezone} · alle {report.setup.interval_weeks} Wochen</p></div>}
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
            <li>Raidserien: {report.created.series ?? 0}</li>
            <li>Bestehende historische Raids als archiviert klassifiziert: {report.archived_raids ?? 0}</li>
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
