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
  characterID: string
  method: AwardMethod
}

const methodLabels: Record<AwardMethod, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const resultLabels: Record<AddonLootImportResult['status'], string> = { created: 'Übernommen', already_imported: 'Bereits importiert', failed: 'Fehler' }

const stateLabels: Partial<Record<AddonLootPreviewRow['state'], string>> = { held: 'beim Plündermeister', parked: 'verwahrt, noch nicht verlost', deleted: 'im Addon gelöscht', disenchanted: 'entzaubert', bank: 'für die Gildenbank' }

function rollText(row: AddonLootPreviewRow) {
  if (row.roll == null) return ''
  const spec = row.spec === 'ms' ? 'Main-Spec' : row.spec === 'os' ? 'Second-Spec' : ''
  const parts = [row.roll_range, spec, row.enhanced_points > 0 ? `Enhanced ${row.enhanced_points} EP` : ''].filter(Boolean)
  return `Wurf ${row.roll}${parts.length > 0 ? ` (${parts.join(', ')})` : ''}`
}

function initialChoices(preview: AddonLootPreview): Record<string, Choice> {
  // Held drops are still with the loot master, so they wait for an explicit
  // choice unless the addon already knows the roll winner (RS2).
  return Object.fromEntries(preview.rows.map((row) => [row.line_key, { isSelected: row.is_importable && row.character_id != null && (row.state !== 'held' || Boolean(row.winner_name)), bossID: row.catalog_boss_id ?? '', characterID: row.character_id != null ? String(row.character_id) : '', method: row.award_method }]))
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
  const isReady = selectedRows.length > 0 && selectedRows.every((row) => choices[row.line_key].bossID && choices[row.line_key].characterID)

  const apply = () => submit(async () => {
    if (!preview || !isReady) return
    setError('')
    try {
      const selections = selectedRows.map((row) => ({ line_key: row.line_key, catalog_boss_id: choices[row.line_key].bossID, character_id: Number(choices[row.line_key].characterID), award_method: choices[row.line_key].method }))
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
    <p className="muted">Den Export aus ForgeLootLedger („EXPORT“) einfügen, prüfen und Empfänger und Vergabeart je Gegenstand wählen. Hat jemand anderes ein Item per Handel weitergegeben, hier den tatsächlichen Gewinner wählen. Locks, Enhanced-Kosten und Raidkader werden wie bei manueller Erfassung geprüft. Bereits importierte Zeilen werden übersprungen.</p>
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
                <td>{row.item_name || 'Unbekannter Gegenstand'}{row.item_id && <span className="muted"> · {row.item_id}</span>}
                  {rollText(row) && <div className="muted">{rollText(row)}</div>}</td>
                <td className="addon-boss">{row.is_importable && row.boss_options.length > 1
                  ? <select aria-label={`Boss für Zeile ${row.line_number}`} disabled={pending} value={choice?.bossID ?? ''} onChange={(e) => choose(row, { bossID: e.target.value })}>
                    <option value="">Boss wählen</option>
                    {row.boss_options.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
                  </select>
                  : row.boss_options.find((boss) => boss.id === row.catalog_boss_id)?.name ?? '—'}</td>
                <td>{row.is_importable
                  ? <select aria-label={`Empfänger für Zeile ${row.line_number}`} disabled={pending} value={choice?.characterID ?? ''} onChange={(e) => choose(row, { characterID: e.target.value })}>
                    <option value="">Empfänger wählen</option>
                    {preview.recipient_options.map((option) => <option key={option.character_id} value={option.character_id}>{option.character_name}</option>)}
                  </select>
                  : row.character_name || row.recipient_name || '—'}
                  {row.is_importable && row.recipient_name && row.recipient_name.toLocaleLowerCase('de') !== (row.character_name || '').toLocaleLowerCase('de') && <div className="muted">Addon: {row.recipient_name}</div>}
                  {stateLabels[row.state] && <div className="muted">{stateLabels[row.state]}</div>}</td>
                <td>{row.is_importable
                  ? <select aria-label={`Vergabeart für Zeile ${row.line_number}`} disabled={pending} value={choice?.method ?? row.award_method} onChange={(e) => choose(row, { method: e.target.value as AwardMethod })}>
                    {Object.entries(methodLabels).map(([method, label]) => <option key={method} value={method}>{label}</option>)}
                  </select>
                  : '—'}</td>
                <td className="addon-hints">{row.errors.length > 0
                  ? row.errors.map((text) => <div key={text} className="addon-hint-error">{text}</div>)
                  : row.warnings.map((text) => <div key={text} className="muted">{text}</div>)}</td>
              </tr>
            })}
          </tbody>
        </table>
      </div>
      <div className="actions">
        <button type="button" disabled={pending || !isReady} onClick={() => void apply()}>{selectedRows.length === 1 ? '1 Gegenstand übernehmen' : `${selectedRows.length} Gegenstände übernehmen`}</button>
        {selectedRows.length > 0 && !isReady && <span className="muted">Bitte für alle ausgewählten Zeilen Boss und Empfänger wählen.</span>}
      </div>
    </>}
    {results.length > 0 && <ul className="plain-list" role="status" aria-label="Importergebnis">
      {results.map((result) => <li key={result.line_key}>Zeile {result.line_number}: {resultLabels[result.status]}{result.character_name ? ` · ${result.character_name}` : ''}{result.message ? ` · ${result.message}` : ''}</li>)}
    </ul>}
  </section>
}
