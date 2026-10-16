import { useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { AddonRosterImportResult, AddonRosterPreview, AddonRosterPreviewRow, AttendanceRole } from '../types'

interface Props {
  raidID: number
  onImported: () => Promise<void>
  onQuickAdd: (name: string) => void
}

interface Choice {
  isSelected: boolean
  role: AttendanceRole
  lateJoin: boolean
}

const statusLabels: Record<AddonRosterPreviewRow['status'], string> = { in_roster: 'Bereits in der Aufstellung', known: 'Bekannt, noch nicht in der Aufstellung', unknown: 'Nicht auf der Website', invalid: 'Fehlerhafte Zeile' }
const resultLabels: Record<AddonRosterImportResult['status'], string> = { added: 'Hinzugefügt', already_in_roster: 'War schon in der Aufstellung', failed: 'Fehler' }

function initialChoices(preview: AddonRosterPreview): Record<number, Choice> {
  return Object.fromEntries(preview.rows.filter((row) => row.status === 'known' && row.character_id != null)
    .map((row) => [row.character_id, { isSelected: true, role: row.suggested_role, lateJoin: row.suggest_late_join }]))
}

export function AddonRosterImport({ raidID, onImported, onQuickAdd }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [payload, setPayload] = useState('')
  const [preview, setPreview] = useState<AddonRosterPreview | null>(null)
  const [previewedPayload, setPreviewedPayload] = useState('')
  const [choices, setChoices] = useState<Record<number, Choice>>({})
  const [results, setResults] = useState<AddonRosterImportResult[]>([])
  const [error, setError] = useState('')

  const loadPreview = async (text: string) => {
    const result = await api.post<AddonRosterPreview>(`/api/v1/raids/${raidID}/addon-roster-import/preview`, { payload: text })
    if (!isActive()) return
    setPreview(result)
    setPreviewedPayload(text)
    setChoices(initialChoices(result))
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

  const selected = Object.entries(choices).filter(([, choice]) => choice.isSelected)

  const apply = () => submit(async () => {
    if (selected.length === 0) return
    setError('')
    try {
      const selections = selected.map(([characterID, choice]) => ({ character_id: Number(characterID), role: choice.role, late_join: choice.lateJoin }))
      const added = await api.post<AddonRosterImportResult[]>(`/api/v1/raids/${raidID}/addon-roster-import`, { payload: previewedPayload, selections })
      if (!isActive()) return
      setResults(added)
      await onImported()
      await loadPreview(previewedPayload)
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Übernahme fehlgeschlagen.')
    }
  })

  const choose = (characterID: number, change: Partial<Choice>) => setChoices((current) => ({ ...current, [characterID]: { ...current[characterID], ...change } }))

  return <section className="subcard addon-roster-import" aria-label="Aufstellung aus Addon abgleichen">
    <h3>Aufstellung aus Addon abgleichen</h3>
    <p className="muted">Im Spiel nachgeladene Charaktere (z. B. aus dem Weltchat) übernehmen: in ForgeLootLedger unter „RAID“ bei „Teilnahme für die Webseite“ auf „EXPORT“ klicken und den Text hier einfügen. Unbekannte Charaktere werden per Schnellanlage angelegt.</p>
    <textarea className="addon-code-export" aria-label="Aufstellungs-Export" rows={4} value={payload} disabled={pending} placeholder="RS1|ROSTER|…" onChange={(e) => setPayload(e.target.value)} />
    <div className="actions">
      <button type="button" disabled={pending || !payload.trim()} onClick={() => void runPreview()}>Abgleichen</button>
    </div>
    {error && <p className="error" role="alert">{error}</p>}
    {preview && <>
      {preview.warnings.map((warning) => <p key={warning} className="error" role="alert">{warning}</p>)}
      <p className="muted">Exportiert von {preview.recorder || 'unbekannt'} · {preview.rows.length} Charaktere im Raid gesehen</p>
      {preview.missing.length > 0 && <p className="muted">In der Aufstellung, aber im Spiel nicht gesehen: {preview.missing.join(', ')}</p>}
      <div className="table-scroll">
        <table className="table small">
          <thead><tr><th>Übernehmen</th><th>Charakter</th><th>Rolle</th><th>Später dazu</th><th>Status</th></tr></thead>
          <tbody>
            {preview.rows.map((row) => {
              const choice = row.status === 'known' && row.character_id != null ? choices[row.character_id] : undefined
              const id = row.character_id ?? 0
              return <tr key={`${row.line_number}:${row.name}`}>
                <td>{choice && <input type="checkbox" aria-label={`${row.name} übernehmen`} disabled={pending} checked={choice.isSelected} onChange={(e) => choose(id, { isSelected: e.target.checked })} />}</td>
                <td>{row.character_name || row.name || '—'}{row.class && <span className="muted"> · {row.class}</span>}</td>
                <td>{choice
                  ? <select aria-label={`Rolle für ${row.name}`} disabled={pending} value={choice.role} onChange={(e) => choose(id, { role: e.target.value as AttendanceRole })}>
                    <option value="TANK">Tank</option><option value="HEAL">Heal</option><option value="DD">DD</option>
                  </select>
                  : '—'}</td>
                <td>{choice ? <input type="checkbox" aria-label={`${row.name} später dazugekommen`} disabled={pending} checked={choice.lateJoin} onChange={(e) => choose(id, { lateJoin: e.target.checked })} /> : '—'}</td>
                <td className="addon-hints">
                  {row.errors.length > 0 ? row.errors.map((text) => <div key={text} className="addon-hint-error">{text}</div>) : <div className={row.status === 'in_roster' ? 'muted' : ''}>{statusLabels[row.status]}</div>}
                  {row.status === 'unknown' && <button type="button" disabled={pending} onClick={() => onQuickAdd(row.name)}>Per Schnellanlage hinzufügen</button>}
                </td>
              </tr>
            })}
          </tbody>
        </table>
      </div>
      <div className="actions">
        <button type="button" disabled={pending || selected.length === 0} onClick={() => void apply()}>{selected.length === 1 ? '1 Charakter hinzufügen' : `${selected.length} Charaktere hinzufügen`}</button>
      </div>
    </>}
    {results.length > 0 && <ul className="plain-list" role="status" aria-label="Abgleichsergebnis">
      {results.map((result) => <li key={result.character_id}>{result.character_name || `Charakter #${result.character_id}`}: {resultLabels[result.status]}{result.message ? ` · ${result.message}` : ''}</li>)}
    </ul>}
  </section>
}
