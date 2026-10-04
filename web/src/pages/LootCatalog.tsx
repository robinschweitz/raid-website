import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type CatalogDataset, type CatalogReport, type CatalogSource, type LootCatalog } from '../types'
import { useSubmission } from '../useSubmission'

function hasFields(value: unknown, required: string[], optional: string[] = []): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
    && required.every((key) => key in value)
    && Object.keys(value).every((key) => required.includes(key) || optional.includes(key))
}

function isSource(value: unknown): value is CatalogSource {
  return hasFields(value, ['id', 'revision', 'url', 'license', 'attribution']) && Object.values(value).every((field) => typeof field === 'string')
}

function stringList(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((entry) => typeof entry === 'string')
}

function isReport(value: unknown): value is CatalogReport {
  return hasFields(value, ['source', 'raw_hash', 'notices', 'errors', 'conflicts', 'inserted', 'skipped', 'merged'])
    && isSource(value.source) && typeof value.raw_hash === 'string' && stringList(value.notices)
    && stringList(value.errors) && stringList(value.conflicts)
    && typeof value.inserted === 'number' && Number.isInteger(value.inserted) && value.inserted >= 0
    && typeof value.skipped === 'number' && Number.isInteger(value.skipped) && value.skipped >= 0
    && typeof value.merged === 'boolean'
}

function parseDataset(value: unknown): CatalogDataset {
  if (!hasFields(value, ['source', 'notices', 'instances', 'bosses', 'items', 'drops']) || !isSource(value.source) || !stringList(value.notices)) {
    throw new Error('Erwartet wird ein normalisierter JSON-Katalog mit Quelle, Hinweisen, Instanzen, Bossen, Items und Drops. Unbekannte Felder sind nicht erlaubt.')
  }
  for (const kind of ['instances', 'bosses', 'items', 'drops'] as const) {
    const rows = value[kind]
    if (!Array.isArray(rows)) throw new Error(`${kind}: Eine JSON-Liste ist erforderlich.`)
    for (const [index, row] of rows.entries()) {
      const fields = kind === 'instances' ? ['id', 'name'] : kind === 'bosses' ? ['id', 'instance_id', 'name'] : kind === 'items' ? ['item_id', 'name'] : ['boss_id', 'item_id', 'difficulty', 'raid_size', 'loot_mode']
      if (!hasFields(row, fields, kind === 'items' ? ['binding', 'quality'] : [])) throw new Error(`${kind}, Zeile ${index + 1}: Fehlende oder unbekannte Felder.`)
      for (const [key, field] of Object.entries(row)) {
        const numeric = ['raid_size', 'loot_mode', 'binding', 'quality'].includes(key)
        if (numeric ? typeof field !== 'number' || !Number.isInteger(field) : typeof field !== 'string') {
          throw new Error(`${kind}, Zeile ${index + 1}: ${key} hat einen ungültigen Datentyp.`)
        }
      }
    }
  }
  // Structural validation is local; references, variants and conflicts remain server-authoritative.
  return value as unknown as CatalogDataset
}

function SourceDetails({ source, notices }: { source: CatalogSource; notices: string[] }) {
  return <div className="subcard">
    <dl><dt>Quelle</dt><dd>{source.id}</dd><dt>Revision</dt><dd style={{ overflowWrap: 'anywhere' }}>{source.revision}</dd>
      <dt>Quellenadresse</dt><dd style={{ overflowWrap: 'anywhere' }}>{source.url}</dd><dt>Lizenz</dt><dd>{source.license}</dd><dt>Attribution</dt><dd>{source.attribution}</dd></dl>
    <h3>Quellenhinweise</h3><ul>{notices.map((notice, index) => <li key={index} style={{ overflowWrap: 'anywhere' }}>{notice}</li>)}</ul>
    {notices.length === 0 && <p className="muted">Keine Quellenhinweise im Datensatz.</p>}
  </div>
}

export function LootCatalogPage() {
  const { player } = useAuth()
  const isAdmin = !!player && atLeast(player.role, 'ADMIN')
  const { pending, submit, isActive, isPending } = useSubmission()
  const fileVersion = useRef(0)
  const loadVersion = useRef(0)
  const currentDataset = useRef<CatalogDataset | null>(null)
  const reviewed = useRef<{ version: number; hash: string } | null>(null)
  const confirmedRef = useRef(false)
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [dataset, setDataset] = useState<CatalogDataset | null>(null)
  const [fileName, setFileName] = useState('')
  const [reading, setReading] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [catalogError, setCatalogError] = useState('')
  const [report, setReport] = useState<CatalogReport | null>(null)
  const [previewVersion, setPreviewVersion] = useState<number | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const [importSucceeded, setImportSucceeded] = useState(false)
  const version = fileVersion.current

  const load = useCallback(async () => {
    if (!isActive()) return
    const requestVersion = ++loadVersion.current
    setCatalogError('')
    setLoading(true)
    try {
      const value = await api.get<LootCatalog>('/api/v1/loot-catalog')
      if (isActive() && requestVersion === loadVersion.current) setCatalog(value)
    } catch (err) {
      if (isActive() && requestVersion === loadVersion.current) setCatalogError(err instanceof ApiError ? err.message : 'Beutekatalog konnte nicht geladen werden. Kein Ersatzdatensatz wird verwendet.')
    } finally {
      if (isActive() && requestVersion === loadVersion.current) setLoading(false)
    }
  }, [isActive])

  useEffect(() => { if (isAdmin) void load() }, [isAdmin, load])

  const chooseFile = async (event: ChangeEvent<HTMLInputElement>) => {
    if (!isAdmin || !isActive() || isPending()) return
    const nextVersion = ++fileVersion.current
    const file = event.target.files?.[0]
    currentDataset.current = null
    reviewed.current = null
    confirmedRef.current = false
    setDataset(null); setReport(null); setPreviewVersion(null); setConfirmed(false); setImportSucceeded(false); setError(''); setFileName(file?.name ?? '')
    setReading(!!file)
    if (!file) return
    try {
      if (!file.name.toLowerCase().endsWith('.json')) throw new Error('Bitte eine geprüfte .json-Datei auswählen, keine SQL-Datei.')
      if (file.size > 16 * 1024 * 1024) throw new Error('Die Datei überschreitet das Upload-Limit von 16 MiB.')
      const parsed: unknown = JSON.parse(await file.text())
      const next = parseDataset(parsed)
      if (!isActive() || nextVersion !== fileVersion.current) return
      currentDataset.current = next
      setDataset(next)
    } catch (err) {
      if (isActive() && nextVersion === fileVersion.current) setError(err instanceof SyntaxError ? 'Die Datei enthält kein gültiges JSON.' : err instanceof Error ? err.message : 'Datei konnte nicht gelesen werden.')
    } finally {
      if (isActive() && nextVersion === fileVersion.current) setReading(false)
    }
  }

  const upload = (merge: boolean) => submit(async () => {
    if (!isAdmin || !dataset || currentDataset.current !== dataset || version !== fileVersion.current) return
    const preview = reviewed.current
    if (merge && (!confirmedRef.current || !preview || preview.version !== version)) return
    reviewed.current = null
    confirmedRef.current = false
    setPreviewVersion(null); setConfirmed(false); setImportSucceeded(false); setReport(null); setError('')
    try {
      const result: unknown = await api.post<CatalogReport>(`/api/v1/loot-catalog/${merge ? 'merge' : 'preview'}`, { catalog: dataset, ...(merge ? { raw_hash: preview!.hash } : {}) })
      if (!isActive() || version !== fileVersion.current) return
      if (!isReport(result)) throw new Error('Der Server hat keinen gültigen Katalogbericht geliefert. Bitte erneut prüfen.')
      setReport(result)
      if ((Object.keys(dataset.source) as (keyof CatalogSource)[]).some((key) => dataset.source[key] !== result.source[key])) {
        setError('Quelle oder Revision des Berichts stimmt nicht mit der ausgewählten Datei überein. Import gesperrt; bitte eine neue Vorschau prüfen.')
        return
      }
      if (!merge && !result.merged && result.errors.length === 0 && result.conflicts.length === 0 && /^[a-f0-9]{64}$/.test(result.raw_hash)) {
        reviewed.current = { version, hash: result.raw_hash }
        setPreviewVersion(version)
      } else if (merge && result.merged && result.raw_hash === preview!.hash && result.errors.length === 0 && result.conflicts.length === 0) {
        setImportSucceeded(true)
        await load()
      } else if (result.errors.length === 0 && result.conflicts.length === 0) {
        setError('Keine gültige Vorschaufreigabe oder Importbestätigung erhalten. Bitte eine neue Vorschau prüfen.')
      }
    } catch (err) {
      if (!isActive() || version !== fileVersion.current) return
      if (err instanceof ApiError && isReport(err.data)) setReport(err.data)
      setError(err instanceof ApiError && err.code === 'catalog_hash_mismatch' ? 'Datei und geprüfter Hash stimmen nicht überein. Bitte eine neue Vorschau prüfen.' : err instanceof Error ? err.message : 'Katalog konnte nicht verarbeitet werden.')
    }
  })

  if (!isAdmin) return <p className="error" role="alert">Der Beutekatalog kann nur von einem Admin verwaltet werden.</p>
  const canMerge = !!dataset && previewVersion === version && !!reviewed.current && confirmed && !pending && !reading
  return <div>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Beutekatalog</h1></div><Link to="/roster">Gildenkonfiguration →</Link></div>
    <section className="card"><h2>Lokaler Bestand</h2>
      {catalogError && <p className="error" role="alert">{catalogError}</p>}
      {loading ? <p className="muted" role="status">Beutekatalog wird geladen…</p> : catalog && <>
        <p>{catalog.instances.length} Instanzen · {catalog.bosses.length} Bosse · {catalog.items.length} Items · {catalog.drops.length} Dropvarianten</p>
        {catalog.instances.length === 0 && <p className="muted">Der lokale Katalog ist leer. Es ist kein vollständiger eingebauter Raidkatalog verfügbar.</p>}
        {catalog.instances.length > 0 && <ul>{catalog.instances.map((instance) => <li key={instance.id}>{instance.name} <span className="muted">#{instance.id}</span></li>)}</ul>}
        {catalog.sources.map((provenance) => <SourceDetails key={provenance.raw_hash} source={provenance.source} notices={provenance.notices} />)}
      </>}
      <button disabled={pending || loading} onClick={() => void load()}>Bestand neu laden</button>
    </section>
    <section className="card"><h2>Geprüften JSON-Katalog importieren</h2>
      <p className="muted">Wähle einen geprüften, normalisierten JSON-Datensatz. Roh-SQL wird nicht angenommen. Die Vorschau zeigt Ergänzungen und Konflikte; vorhandene Daten werden niemals überschrieben.</p>
      <label>Katalogdatei (.json)<input type="file" accept=".json,application/json" aria-label="Katalogdatei" disabled={pending} onChange={(event) => void chooseFile(event)} /></label>
      {reading && <p role="status" className="muted">Datei wird geprüft…</p>}
      {fileName && <p className="muted">Ausgewählte Datei: {fileName}</p>}
      {error && <p className="error" role="alert">{error}</p>}
      {dataset && <><p>{dataset.instances.length} Instanzen · {dataset.bosses.length} Bosse · {dataset.items.length} Items · {dataset.drops.length} Dropvarianten in der Datei</p><SourceDetails source={dataset.source} notices={dataset.notices} /></>}
      <div className="actions"><button disabled={pending || reading || !dataset} onClick={() => void upload(false)}>Vorschau prüfen</button></div>
      {report && <div className="subcard"><h3>{report.merged ? 'Importbericht' : 'Vorschau / Prüfbericht'}</h3>
        <SourceDetails source={report.source} notices={report.notices} />
        <p>{report.merged ? 'Eingefügt' : 'Geplante Ergänzungen'}: {report.inserted} · Übersprungen: {report.skipped}</p>
        <p className="muted" style={{ overflowWrap: 'anywhere' }}>Geprüfter Hash: {report.raw_hash || 'Keine Freigabe'}</p>
        {report.errors.length > 0 && <><h3>Fehler</h3><ul>{report.errors.map((message, index) => <li className="negative" key={index}>{message}</li>)}</ul></>}
        {report.conflicts.length > 0 && <><h3>Konflikte</h3><ul>{report.conflicts.map((message, index) => <li className="negative" key={index}>{message}</li>)}</ul></>}
        {importSucceeded ? <p className="positive">Import abgeschlossen. Vorhandene Daten wurden nicht überschrieben.</p> : <p className="muted">Dieser Bericht bestätigt keinen Import. Die Vorschau schreibt keine Katalogeinträge.</p>}
      </div>}
      <label className="check"><input type="checkbox" disabled={pending || previewVersion !== version} checked={confirmed} onChange={(event) => { if (!isPending() && isActive()) { confirmedRef.current = event.target.checked; setConfirmed(event.target.checked) } }} />Ich habe Quelle, Lizenz, Hinweise und Vorschau geprüft und bestätige den Import ohne Überschreiben.</label>
      <div className="actions"><button disabled={!canMerge} onClick={() => void upload(true)}>Katalog importieren</button></div>
      {pending && <p className="muted" role="status">Katalog wird verarbeitet…</p>}
    </section>
  </div>
}
