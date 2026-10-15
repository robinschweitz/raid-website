import { useEffect, useState } from 'react'
import type { Attendance } from '../types'
import { defaultRecruitmentTemplate, recruitmentRoles, renderRecruitmentText } from '../recruitmentText'
import { iccIdealSetup, matchRaidSetup, setupOptions, type RaidSetup } from '../raidSetup'

interface Props {
  raidID: number
  attendance: Attendance[]
  onRefresh: () => Promise<void>
}

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

export function RaidRecruitment({ raidID, attendance, onRefresh }: Props) {
  const [template, setTemplate] = useState<StoredTemplate>({ text: defaultRecruitmentTemplate, setup: defaultSetup() })
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')

  useEffect(() => {
    setTemplate(loadTemplate(raidID))
    setCopyStatus('')
  }, [raidID])

  useEffect(() => {
    const timer = window.setInterval(() => { void onRefresh() }, 15000)
    return () => window.clearInterval(timer)
  }, [onRefresh])

  const updateTemplate = (next: StoredTemplate) => {
    setTemplate(next)
    setCopyStatus('')
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
    <p className="muted">Die Zahlen werden aus der gespeicherten Raidaufstellung berechnet und alle 15 Sekunden aktualisiert. Ersatzbank-Spieler zählen nicht als besetzte Plätze.</p>
    <p className="muted">Vorlage und Soll-Besetzung werden pro Raid in diesem Browser gespeichert, nicht auf dem Server.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <section className="card">
      <h2>Raid und Soll-Besetzung</h2>
      <p className="muted">Das ICC-Idealsetup ist vorbelegt. Jede Gruppe hat fünf Positionen; „Leer“ deaktiviert einen Platz. Jeder aufgestellte Charakter kann nur einen Platz erfüllen.</p>
      <div className="actions">
        <button type="button" onClick={() => updateTemplate({ ...template, setup: defaultSetup() })}>ICC-Idealsetup laden</button>
        <button type="button" onClick={() => updateTemplate({ ...template, setup: Array.from({ length: 5 }, () => Array(5).fill('Leer')) })}>Setup leeren</button>
      </div>
      <div className="recruitment-setup" aria-label="Soll-Besetzung">
        {template.setup.map((group, column) => <section className="recruitment-group" key={column}>
          <header>Gruppe {column + 1}</header>
          <div className="recruitment-slots">{group.map((slot, row) => {
            const slotIndex = column * 5 + row
            return <div className="recruitment-slot" key={row}>
              <label>Platz {row + 1}
                <select aria-label={`Gruppe ${column + 1}, Platz ${row + 1}`} value={slot}
                  onChange={(event) => updateTemplate({ ...template, setup: template.setup.map((oldGroup, index) => index === column ? oldGroup.map((value, position) => position === row ? event.target.value : value) : [...oldGroup]) })}>
                  {setupOptions.map((option) => <option key={option} value={option}>{option}</option>)}
                </select>
              </label>
              {slot !== 'Leer' && <span className={matchedIndices.has(slotIndex) ? 'badge ok' : 'badge'}>{matchedIndices.has(slotIndex) ? 'Besetzt' : 'Gesucht'}</span>}
            </div>
          })}</div>
        </section>)}
      </div>
      <h3>Noch gesuchte Positionen</h3>
      {missingSlots.length ? <ul className="recruitment-missing">{missingSlots.map((slot) => <li key={slot.index}>Gruppe {Math.floor(slot.index / 5) + 1}: {slot.label}</li>)}</ul> : <p>Alle geplanten Positionen sind besetzt.</p>}
    </section>
    <section className="card">
      <h2>Textvorlage</h2>
      <p className="muted">Platzhalter: {recruitmentRoles.map((role) => role.token).join(', ')} und {'{{SETUP}}'} für die konkret gesuchten Positionen. Alle anderen Wörter bleiben unverändert.</p>
      <textarea aria-label="Gesuchtext-Vorlage" rows={5} value={template.text}
        onChange={(event) => updateTemplate({ ...template, text: event.target.value })} />
      <h3>Aktueller Text zum Kopieren</h3>
      <textarea aria-label="Fertiger Gesuchtext" readOnly rows={5} value={preview} />
      <div className="actions"><button type="button" disabled={!!error} onClick={() => void copyText()}>Text kopieren</button><span role="status">{copyStatus}</span></div>
    </section>
  </div>
}
