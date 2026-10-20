import { useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { AddonCalendarImportResult, AddonCalendarPreview, AddonCalendarPreviewRow } from '../types'

interface Props {
  raidID: number
  onImported: () => Promise<void>
}

const answerLabels: Record<string, string> = {
  invited: 'Keine Antwort', notsignedup: 'Keine Antwort', accepted: 'Zugesagt', signedup: 'Angemeldet',
  declined: 'Abgelehnt', out: 'Abwesend', confirmed: 'Bestätigt', standby: 'Ersatzbank', tentative: 'Vorläufig',
}
const websiteLabels: Record<AddonCalendarPreviewRow['website_states'][number], string> = {
  roster: 'Aufstellung', bench: 'Ersatzbank', signup: 'Angemeldet', tentative: 'Unsicher', declined: 'Kann nicht',
}
const actionLabels: Record<AddonCalendarPreviewRow['action'], string> = {
  none: '—', signup: 'Anmelden', decline: '„Kann nicht“ eintragen', tentative: '„Unsicher“ eintragen',
}
const resultLabels: Record<AddonCalendarImportResult['status'], string> = {
  signed_up: 'Angemeldet', declined: '„Kann nicht“ eingetragen', tentative: '„Unsicher“ eingetragen', unchanged: 'Unverändert', failed: 'Fehler',
}

function proposed(preview: AddonCalendarPreview): Record<number, boolean> {
  return Object.fromEntries(preview.rows.filter((row) => row.action !== 'none' && row.character_id != null).map((row) => [row.character_id, true]))
}

// AddonCalendarImport brings the answers of the game calendar event (RaidLedger
// „ANTWORTEN FÜR WEBSEITE“) onto the website: preview first, then apply (ADR 092).
export function AddonCalendarImport({ raidID, onImported }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [payload, setPayload] = useState('')
  const [preview, setPreview] = useState<AddonCalendarPreview | null>(null)
  const [previewedPayload, setPreviewedPayload] = useState('')
  const [selected, setSelected] = useState<Record<number, boolean>>({})
  const [results, setResults] = useState<AddonCalendarImportResult[]>([])
  const [error, setError] = useState('')

  const loadPreview = async (text: string) => {
    const result = await api.post<AddonCalendarPreview>(`/api/v1/raids/${raidID}/addon-calendar-import/preview`, { payload: text })
    if (!isActive()) return
    setPreview(result)
    setPreviewedPayload(text)
    setSelected(proposed(result))
  }

  const runPreview = () => submit(async () => {
    setError('')
    setResults([])
    try {
      await loadPreview(payload)
    } catch (err) {
      if (isActive()) { setPreview(null); setError(err instanceof ApiError ? err.message : 'Vorschau fehlgeschlagen.') }
    }
  })

  const selections = (preview?.rows ?? [])
    .filter((row) => row.action !== 'none' && row.character_id != null && selected[row.character_id])
    .map((row) => ({ character_id: row.character_id as number, action: row.action }))

  const apply = () => submit(async () => {
    if (selections.length === 0) return
    setError('')
    try {
      const applied = await api.post<AddonCalendarImportResult[]>(`/api/v1/raids/${raidID}/addon-calendar-import`, { payload: previewedPayload, selections })
      if (!isActive()) return
      setResults(applied)
      await onImported()
      await loadPreview(previewedPayload)
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Übernahme fehlgeschlagen.')
    }
  })

  return <section className="subcard addon-roster-import" aria-label="Antworten aus dem Spielkalender">
    <h3>Antworten aus dem Spielkalender</h3>
    <p className="muted">Wer im Spielkalender zu- oder abgesagt hat, hier übernehmen: in RaidLedger unter „RAID“ bei „SPIELKALENDER“ auf „ANTWORTEN FÜR WEBSEITE“ klicken und den Text hier einfügen. Eine Zusage wird zur offenen Anmeldung mit der Main-Spec, eine Absage zu „kann nicht“. Die Aufstellung ändert sich dabei nicht.</p>
    <textarea className="addon-code-export" aria-label="Kalender-Export" rows={4} value={payload} disabled={pending} placeholder="RS2;CAL;…" onChange={(e) => setPayload(e.target.value)} />
    <div className="actions">
      <button type="button" disabled={pending || !payload.trim()} onClick={() => void runPreview()}>Abgleichen</button>
    </div>
    {error && <p className="error" role="alert">{error}</p>}
    {preview && <>
      {preview.warnings.map((warning) => <p key={warning} className="error" role="alert">{warning}</p>)}
      <p className="muted">Exportiert von {preview.recorder || 'unbekannt'} · {preview.rows.length} Einladungen im Kalender</p>
      <div className="table-scroll">
        <table className="table small">
          <thead><tr><th>Übernehmen</th><th>Charakter</th><th>Im Kalender</th><th>Auf der Webseite</th><th>Übernahme</th></tr></thead>
          <tbody>
            {preview.rows.map((row) => {
              const id = row.character_id ?? 0
              const selectable = row.action !== 'none' && row.character_id != null
              return <tr key={`${row.line_number}:${row.name}`}>
                <td>{selectable && <input type="checkbox" aria-label={`${row.name} übernehmen`} disabled={pending} checked={selected[id] ?? false} onChange={(e) => setSelected((current) => ({ ...current, [id]: e.target.checked }))} />}</td>
                <td>{row.character_name || row.name || '—'}{row.class && <span className="muted"> · {row.class}</span>}</td>
                <td>{row.answer ? answerLabels[row.answer] ?? row.answer : '—'}</td>
                <td>{row.website_states.length > 0 ? row.website_states.map((state) => websiteLabels[state]).join(', ') : '—'}</td>
                <td className="addon-hints">
                  {row.errors.map((text) => <div key={text} className="addon-hint-error">{text}</div>)}
                  {selectable && <div><strong>{actionLabels[row.action]}</strong></div>}
                  {row.notes.map((text) => <div key={text} className="muted">{text}</div>)}
                </td>
              </tr>
            })}
          </tbody>
        </table>
      </div>
      <div className="actions">
        <button type="button" disabled={pending || selections.length === 0} onClick={() => void apply()}>{selections.length === 1 ? '1 Antwort übernehmen' : `${selections.length} Antworten übernehmen`}</button>
      </div>
    </>}
    {results.length > 0 && <ul className="plain-list" role="status" aria-label="Kalender-Ergebnis">
      {results.map((result) => <li key={result.character_id}>{result.character_name || `Charakter #${result.character_id}`}: {resultLabels[result.status]}{result.message ? ` · ${result.message}` : ''}</li>)}
    </ul>}
  </section>
}
