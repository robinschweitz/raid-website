import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import type { Attendance, Raid } from '../types'
import { defaultRecruitmentTemplate, recruitmentRoles, renderRecruitmentText } from '../recruitmentText'
import { iccIdealSetup, matchRaidSetup, setupOptions, type RaidSetup } from '../raidSetup'

interface StoredTemplate { text: string; setup: RaidSetup }
const defaultSetup = () => iccIdealSetup.map((group) => [...group])
const storageKey = (raidID: number) => `raid-recruitment-template-v1:${raidID}`

function loadTemplate(raidID: number): StoredTemplate {
  const fallback = { text: defaultRecruitmentTemplate, setup: defaultSetup() }
  try {
    const saved = window.localStorage.getItem(storageKey(raidID))
    if (!saved) return fallback
    const parsed: unknown = JSON.parse(saved)
    if (typeof parsed !== 'object' || parsed === null) return fallback
    const value = parsed as Partial<StoredTemplate>
    if (typeof value.text !== 'string') return fallback
    if (!Array.isArray(value.setup) || value.setup.length !== 5 || !value.setup.every((group) => Array.isArray(group) && group.length === 5 && group.every((slot) => typeof slot === 'string' && setupOptions.some((option) => option === slot)))) return fallback
    return { text: value.text, setup: value.setup }
  } catch {
    return fallback
  }
}

export function RecruitmentPage() {
  const [raids, setRaids] = useState<Raid[]>([])
  const [raidID, setRaidID] = useState(0)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [template, setTemplate] = useState<StoredTemplate>({ text: defaultRecruitmentTemplate, setup: defaultSetup() })
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')

  useEffect(() => {
    let active = true
    api.get<Raid[]>('/api/v1/raids?include_archived=false').then((rows) => {
      if (!active) return
      const current = rows.filter((raid) => !raid.archived_at && (raid.status === 'OPEN' || raid.status === 'PLANNED'))
      setRaids(current)
      setRaidID((previous) => current.some((raid) => raid.id === previous) ? previous : (current[0]?.id ?? 0))
    }).catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Raids konnten nicht geladen werden.') })
    return () => { active = false }
  }, [])

  useEffect(() => {
    setTemplate(raidID ? loadTemplate(raidID) : { text: defaultRecruitmentTemplate, setup: defaultSetup() })
    setCopyStatus('')
  }, [raidID])

  useEffect(() => {
    if (!raidID) return
    let active = true
    const refresh = () => api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`)
      .then((rows) => { if (active) { setAttendance(rows); setError('') } })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Aufstellung konnte nicht geladen werden.') })
    setAttendance([])
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 15000)
    return () => { active = false; window.clearInterval(timer) }
  }, [raidID])

  const updateTemplate = (next: StoredTemplate) => {
    setTemplate(next)
    setCopyStatus('')
    if (!raidID) return
    try {
      window.localStorage.setItem(storageKey(raidID), JSON.stringify(next))
    } catch {
      setError('Vorlage konnte in diesem Browser nicht gespeichert werden.')
    }
  }

  const { missingSlots, missing, matchedIndices } = matchRaidSetup(template.setup, attendance)
  const preview = renderRecruitmentText(template.text, missing).replace(/{{SETUP}}/g, missingSlots.length ? missingSlots.map((slot) => slot.label).join(', ') : 'keine offenen Plätze')
  const copyText = async () => {
    try {
      await navigator.clipboard.writeText(preview)
      setCopyStatus('Text kopiert.')
    } catch {
      setCopyStatus('Kopieren nicht möglich. Bitte den Text markieren und manuell kopieren.')
    }
  }

  return <div>
    <div className="page-heading"><div><p className="eyebrow">Raidplanung</p><h1>Gesuchtext</h1></div></div>
    <p className="muted">Die Zahlen werden aus der gespeicherten Raidaufstellung berechnet und alle 15 Sekunden aktualisiert. Ersatzbank-Spieler zählen nicht als besetzte Plätze.</p>
    <p className="muted">Vorlage und Soll-Besetzung werden pro Raid in diesem Browser gespeichert, nicht auf dem Server.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <section className="card">
      <h2>Raid und Soll-Besetzung</h2>
      <label htmlFor="recruitment-raid">Raid</label>
      <select id="recruitment-raid" value={raidID} onChange={(event) => setRaidID(Number(event.target.value))}>
        {raids.length === 0 && <option value={0}>Keine aktiven Raids</option>}
        {raids.map((raid) => <option key={raid.id} value={raid.id}>{raid.name}</option>)}
      </select>
      <p className="muted">Das ICC-Idealsetup ist vorbelegt. Jede Gruppe hat fünf Positionen; „Leer“ deaktiviert einen Platz. Jeder aufgestellte Charakter kann nur einen Platz erfüllen.</p>
      <div className="actions">
        <button type="button" disabled={!raidID} onClick={() => updateTemplate({ ...template, setup: defaultSetup() })}>ICC-Idealsetup laden</button>
        <button type="button" disabled={!raidID} onClick={() => updateTemplate({ ...template, setup: Array.from({ length: 5 }, () => Array(5).fill('Leer')) })}>Setup leeren</button>
      </div>
      <div className="table-scroll"><table className="table">
        <thead><tr>{template.setup.map((_, index) => <th key={index}>Gruppe {index + 1}</th>)}</tr></thead>
        <tbody>{Array.from({ length: 5 }, (_, row) => <tr key={row}>
          {template.setup.map((group, column) => {
            const slotIndex = column * 5 + row
            return <td key={column}>
              <select aria-label={`Gruppe ${column + 1}, Platz ${row + 1}`} value={group[row]} disabled={!raidID}
                onChange={(event) => updateTemplate({ ...template, setup: template.setup.map((oldGroup, index) => index === column ? oldGroup.map((value, position) => position === row ? event.target.value : value) : [...oldGroup]) })}>
                {setupOptions.map((option) => <option key={option} value={option}>{option}</option>)}
              </select>
              {group[row] !== 'Leer' && <span className={matchedIndices.has(slotIndex) ? 'badge ok' : 'badge'}>{matchedIndices.has(slotIndex) ? 'Besetzt' : 'Gesucht'}</span>}
            </td>
          })}
        </tr>)}</tbody>
      </table></div>
      <h3>Noch gesuchte Positionen</h3>
      {missingSlots.length ? <ul>{missingSlots.map((slot) => <li key={slot.index}>Gruppe {Math.floor(slot.index / 5) + 1}: {slot.label}</li>)}</ul> : <p>Alle geplanten Positionen sind besetzt.</p>}
    </section>
    <section className="card">
      <h2>Textvorlage</h2>
      <p className="muted">Platzhalter: {recruitmentRoles.map((role) => role.token).join(', ')} und {'{{SETUP}}'} für die konkret gesuchten Positionen. Alle anderen Wörter bleiben unverändert.</p>
      <textarea aria-label="Gesuchtext-Vorlage" rows={5} value={template.text} disabled={!raidID}
        onChange={(event) => updateTemplate({ ...template, text: event.target.value })} />
      <h3>Aktueller Text zum Kopieren</h3>
      <textarea aria-label="Fertiger Gesuchtext" readOnly rows={5} value={preview} />
      <div className="actions"><button type="button" disabled={!raidID || !!error} onClick={() => void copyText()}>Text kopieren</button><span role="status">{copyStatus}</span></div>
    </section>
  </div>
}
