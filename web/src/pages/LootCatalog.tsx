import { useCallback, useEffect, useRef, useState } from 'react'
import { api, ApiError } from '../api'
import { type LootCatalog } from '../types'
import { useSubmission } from '../useSubmission'

export function LootCatalogPage() {
  const { isActive } = useSubmission()
  const loadVersion = useRef(0)
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setLoading(true)
    setError('')
    try {
      const value = await api.get<LootCatalog>('/api/v1/loot-catalog')
      if (isActive() && version === loadVersion.current) setCatalog(value)
    } catch (err) {
      if (isActive() && version === loadVersion.current) setError(err instanceof ApiError ? err.message : 'Beutekatalog konnte nicht geladen werden. Bitte erneut versuchen; kein Ersatzdatensatz wird verwendet.')
    } finally {
      if (isActive() && version === loadVersion.current) setLoading(false)
    }
  }, [isActive])
  useEffect(() => { void load() }, [load])

  return <div>
    <div className="page-heading"><div><p className="eyebrow">Dateibasierte Spielquelle</p><h1>Beutekatalog</h1></div></div>
    <section className="card"><h2>Verfügbare Dropdaten</h2>
      <p className="muted">Der Server stellt die geprüfte JSON-Quelldatei automatisch bereit. Diese Ansicht ist schreibgeschützt; hier werden keine Dateien hochgeladen oder importiert.</p>
      <p className="muted">Teilbestand: Eine vollständige Boss- oder Itemabdeckung ist nicht zugesichert. Bitte die Quellenhinweise beachten.</p>
      {error && <p className="error" role="alert">{error}</p>}
      {loading ? <p className="muted" role="status">Beutekatalog wird geladen…</p> : catalog && <>
        <p>{catalog.instances.length} Instanzen · {catalog.bosses.length} Bosse · {catalog.items.length} Items · {catalog.drops.length} Dropvarianten</p>
        {catalog.instances.length === 0 && <p className="muted">Keine geprüften Dropdaten verfügbar.</p>}
        <ul>{catalog.instances.map((instance) => <li key={instance.id}>{instance.name}</li>)}</ul>
        {catalog.sources.map((entry) => <details className="subcard" key={`${entry.source.id}-${entry.raw_hash}`}>
          <summary>Quelle: {entry.source.id} · {entry.source.license}</summary>
          <dl style={{ overflowWrap: 'anywhere' }}>
            <dt>Quelldatei / Quelle</dt><dd>{entry.source.id}</dd>
            <dt>Version</dt><dd>{entry.source.revision}</dd>
            <dt>Quellenadresse</dt><dd>{entry.source.url}</dd>
            <dt>Lizenz</dt><dd>{entry.source.license}</dd>
            <dt>Attribution</dt><dd>{entry.source.attribution}</dd>
            <dt>JSON-Prüfsumme</dt><dd>{entry.raw_hash}</dd>
          </dl>
          <ul>{entry.notices.map((notice, index) => <li key={index} style={{ overflowWrap: 'anywhere' }}>{notice}</li>)}</ul>
        </details>)}
      </>}
      <button type="button" disabled={loading} onClick={() => void load()}>Bestand neu laden</button>
    </section>
  </div>
}