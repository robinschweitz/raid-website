import { useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { AddonLootImportResult, AddonLootPreview, AddonLootPreviewRow, AwardMethod } from '../types'

interface Props {
  raidID: number
  onImported: () => Promise<void>
}

interface Choice {
  isSelected: boolean
  bossID: string
  method: AwardMethod
}

const methodLabels: Record<AwardMethod, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const resultLabels: Record<AddonLootImportResult['status'], string> = { created: 'Übernommen', already_imported: 'Bereits importiert', failed: 'Fehler' }

function initialChoices(preview: AddonLootPreview): Record<string, Choice> {
  return Object.fromEntries(preview.rows.map((row) => [row.line_key, { isSelected: row.is_importable, bossID: row.catalog_boss_id ?? '', method: row.award_method }]))
}

export function AddonLootImport({ raidID, onImported }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [payload, setPayload] = useState('')
  const [preview, setPreview] = useState<AddonLootPreview | null>(null)
  const [previewedPayload, setPreviewedPayload] = useState('')
  const [choices, setChoices] = useState<Record<string, Choice>>({})
  const [results, setResults] = useState<AddonLootImportResult[]>([])
  const [error, setError] = useState('')

  const loadPreview = async (text: string) => {
    const result = await api.post<AddonLootPreview>(`/api/v1/raids/${raidID}/addon-loot-import/preview`, { payload: text })
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

  const selectedRows = preview?.rows.filter((row) => row.is_importable && choices[row.line_key]?.isSelected) ?? []
  const isReady = selectedRows.length > 0 && selectedRows.every((row) => choices[row.line_key].bossID)

  const apply = () => submit(async () => {
    if (!preview || !isReady) return
    setError('')
    try {
      const selections = selectedRows.map((row) => ({ line_key: row.line_key, catalog_boss_id: choices[row.line_key].bossID, character_id: row.character_id, award_method: choices[row.line_key].method }))
      const imported = await api.post<AddonLootImportResult[]>(`/api/v1/raids/${raidID}/addon-loot-import`, { payload: previewedPayload, selections })
      if (!isActive()) return
      setResults(imported)
      await onImported()
      await loadPreview(previewedPayload)
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Import fehlgeschlagen.')
    }
  })

  const choose = (row: AddonLootPreviewRow, change: Partial<Choice>) => setChoices((current) => ({ ...current, [row.line_key]: { ...current[row.line_key], ...change } }))

  return <section className="subcard addon-loot-import" aria-label="Loot aus Addon importieren">
    <h3>Loot aus Addon importieren</h3>
    <p className="muted">Den Loot-Export des RaidSync-Addons einfügen, prüfen und die Vergabeart je Gegenstand wählen. Locks, Enhanced-Kosten und Raidkader werden wie bei manueller Erfassung geprüft. Bereits importierte Zeilen werden übersprungen.</p>
    <textarea className="addon-code-export" aria-label="Loot-Export" rows={4} value={payload} disabled={pending} placeholder="RS1|LOOT|…" onChange={(e) => setPayload(e.target.value)} />
    <div className="actions">
      <button type="button" disabled={pending || !payload.trim()} onClick={() => void runPreview()}>Vorschau</button>
    </div>
    {error && <p className="error" role="alert">{error}</p>}
    {preview && <>
      {preview.warnings.map((warning) => <p key={warning} className="error" role="alert">{warning}</p>)}
      <p className="muted">Aufgezeichnet von {preview.recorder || 'unbekannt'} · {preview.rows.length} Zeilen</p>
      <div className="table-scroll">
        <table className="table small">
          <thead><tr><th>Übernehmen</th><th>Gegenstand</th><th>Boss</th><th>Empfänger</th><th>Vergabeart</th><th>Hinweise</th></tr></thead>
          <tbody>
            {preview.rows.map((row) => {
              const choice = choices[row.line_key]
              return <tr key={row.line_key}>
                <td><input type="checkbox" aria-label={`Zeile ${row.line_number} übernehmen`} disabled={pending || !row.is_importable} checked={choice?.isSelected ?? false} onChange={(e) => choose(row, { isSelected: e.target.checked })} /></td>
                <td>{row.item_name || 'Unbekannter Gegenstand'}{row.item_id && <span className="muted"> · {row.item_id}</span>}</td>
                <td>{row.boss_options.length > 1
                  ? <select aria-label={`Boss für Zeile ${row.line_number}`} disabled={pending || !row.is_importable} value={choice?.bossID ?? ''} onChange={(e) => choose(row, { bossID: e.target.value })}>
                    <option value="">Boss wählen</option>
                    {row.boss_options.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
                  </select>
                  : row.boss_options[0]?.name ?? '—'}</td>
                <td>{row.character_name || row.recipient_name || '—'}</td>
                <td><select aria-label={`Vergabeart für Zeile ${row.line_number}`} disabled={pending || !row.is_importable} value={choice?.method ?? row.award_method} onChange={(e) => choose(row, { method: e.target.value as AwardMethod })}>
                  {Object.entries(methodLabels).map(([method, label]) => <option key={method} value={method}>{label}</option>)}
                </select></td>
                <td>{row.errors.map((text) => <div key={text} className="error">{text}</div>)}{row.warnings.map((text) => <div key={text} className="muted">{text}</div>)}</td>
              </tr>
            })}
          </tbody>
        </table>
      </div>
      <div className="actions">
        <button type="button" disabled={pending || !isReady} onClick={() => void apply()}>{selectedRows.length === 1 ? '1 Gegenstand übernehmen' : `${selectedRows.length} Gegenstände übernehmen`}</button>
        {selectedRows.length > 0 && !isReady && <span className="muted">Bitte für alle ausgewählten Zeilen einen Boss wählen.</span>}
      </div>
    </>}
    {results.length > 0 && <ul className="plain-list" role="status" aria-label="Importergebnis">
      {results.map((result) => <li key={result.line_key}>Zeile {result.line_number}: {resultLabels[result.status]}{result.character_name ? ` · ${result.character_name}` : ''}{result.message ? ` · ${result.message}` : ''}</li>)}
    </ul>}
  </section>
}
