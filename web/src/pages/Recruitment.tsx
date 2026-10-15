import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import type { Attendance } from '../types'
import { defaultRecruitmentTemplate, recruitmentRoles, recruitmentStorageKey, renderRecruitmentText } from '../recruitmentText'
import { iccIdealSetup, matchRaidSetup, setupOptions, type RaidSetup } from '../raidSetup'

interface Props {
  raidID: number
  seriesID?: number | null
  attendance: Attendance[]
}

interface StoredTemplate { text: string; setup: RaidSetup }
interface ServerTemplate extends StoredTemplate { updated_by: number; updated_at: string }
const defaultSetup = () => iccIdealSetup.map((group) => [...group])
const defaultTemplate = (): StoredTemplate => ({ text: defaultRecruitmentTemplate, setup: defaultSetup() })

function validSetup(setup: unknown): setup is RaidSetup {
  return Array.isArray(setup) && setup.length === 5 && setup.every((group) => Array.isArray(group) && group.length === 5 && group.every((slot) => typeof slot === 'string' && setupOptions.some((option) => option === slot)))
}

function asTemplate(value: unknown): StoredTemplate | null {
  if (typeof value !== 'object' || value === null) return null
  const { text, setup } = value as Partial<StoredTemplate>
  return typeof text === 'string' && validSetup(setup) ? { text, setup } : null
}

// Templates that were only saved in this browser before they were shared via the server.
function legacyTemplate(raidID: number, seriesID?: number | null): StoredTemplate | null {
  try {
    const saved = window.localStorage.getItem(recruitmentStorageKey(raidID, seriesID)) ?? window.localStorage.getItem(recruitmentStorageKey(raidID))
    return saved ? asTemplate(JSON.parse(saved)) : null
  } catch {
    return null
  }
}

const sameTemplate = (a: StoredTemplate, b: StoredTemplate) => a.text === b.text && JSON.stringify(a.setup) === JSON.stringify(b.setup)

export function RaidRecruitment({ raidID, seriesID, attendance: initialAttendance }: Props) {
  const [template, setTemplate] = useState<StoredTemplate>(defaultTemplate)
  const [saved, setSaved] = useState<StoredTemplate | null>(null)
  const [savedAt, setSavedAt] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [fromBrowser, setFromBrowser] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [copyStatus, setCopyStatus] = useState('')
  const [attendance, setAttendance] = useState(initialAttendance)

  useEffect(() => {
    let current = true
    setLoaded(false)
    setCopyStatus('')
    api.get<ServerTemplate | null>(`/api/v1/raids/${raidID}/recruitment-template`).then((value) => {
      if (!current) return
      const stored = asTemplate(value)
      const legacy = stored ? null : legacyTemplate(raidID, seriesID)
      setTemplate(stored ?? legacy ?? defaultTemplate())
      setSaved(stored)
      setSavedAt(stored && value ? new Date(value.updated_at).toLocaleString('de-DE') : '')
      setFromBrowser(!!legacy)
      setError('')
      setLoaded(true)
    }).catch((err: unknown) => {
      if (!current) return
      setTemplate(legacyTemplate(raidID, seriesID) ?? defaultTemplate())
      setError(err instanceof ApiError ? err.message : 'Die gespeicherte Vorlage konnte nicht geladen werden.')
      setLoaded(true)
    })
    return () => { current = false }
  }, [raidID, seriesID])

  // Only the roster is refreshed, so nothing else on the raid page is touched.
  useEffect(() => {
    let current = true
    const timer = window.setInterval(() => {
      api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`).then((rows) => { if (current) setAttendance(rows) }).catch(() => undefined)
    }, 15000)
    return () => { current = false; window.clearInterval(timer) }
  }, [raidID])

  useEffect(() => { setAttendance(initialAttendance) }, [initialAttendance])

  const updateTemplate = (next: StoredTemplate) => {
    setTemplate(next)
    setCopyStatus('')
  }

  const dirty = loaded && (!saved || !sameTemplate(saved, template))
  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const result = await api.put<ServerTemplate>(`/api/v1/raids/${raidID}/recruitment-template`, { text: template.text, setup: template.setup })
      setSaved({ text: result.text, setup: result.setup })
      setSavedAt(new Date(result.updated_at).toLocaleString('de-DE'))
      setFromBrowser(false)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Die Vorlage konnte nicht gespeichert werden.')
    } finally {
      setSaving(false)
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
    <p className="muted">Vorlage und Soll-Besetzung gelten {seriesID ? 'für die ganze Raidserie' : 'für diesen Raid'} und werden mit „Vorlage speichern“ für alle Raidleiter gespeichert.{savedAt ? ` Zuletzt gespeichert: ${savedAt}.` : ''}</p>
    {fromBrowser && <p className="field-hint">Diese Vorlage stammt aus diesem Browser und ist noch nicht für andere gespeichert.</p>}
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
      <div className="actions">
        <button type="button" className="primary-action" disabled={!dirty || saving} onClick={() => void save()}>Vorlage speichern</button>
        <button type="button" onClick={() => void copyText()}>Text kopieren</button>
        <span role="status">{copyStatus || (saving ? 'Wird gespeichert…' : dirty ? 'Ungespeicherte Änderungen.' : loaded && saved ? 'Gespeichert.' : '')}</span>
      </div>
    </section>
  </div>
}
