import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import type { Attendance, Raid } from '../types'
import { defaultRecruitmentTemplate, emptyTargets, recruitmentNeeds, recruitmentRoles, renderRecruitmentText, type RecruitmentTargets } from '../recruitmentText'

interface StoredTemplate { text: string; targets: RecruitmentTargets }
const storageKey = (raidID: number) => `raid-recruitment-template-v1:${raidID}`

function loadTemplate(raidID: number): StoredTemplate {
  const fallback = { text: defaultRecruitmentTemplate, targets: emptyTargets() }
  try {
    const saved = window.localStorage.getItem(storageKey(raidID))
    if (!saved) return fallback
    const parsed: unknown = JSON.parse(saved)
    if (typeof parsed !== 'object' || parsed === null) return fallback
    const value = parsed as Partial<StoredTemplate>
    if (typeof value.text !== 'string' || typeof value.targets !== 'object' || value.targets === null) return fallback
    const targets = emptyTargets()
    for (const role of recruitmentRoles) {
      const count = value.targets[role.key]
      if (!Number.isSafeInteger(count) || count === undefined || count < 0) return fallback
      targets[role.key] = count
    }
    return { text: value.text, targets }
  } catch {
    return fallback
  }
}

export function RecruitmentPage() {
  const [raids, setRaids] = useState<Raid[]>([])
  const [raidID, setRaidID] = useState(0)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [template, setTemplate] = useState<StoredTemplate>({ text: defaultRecruitmentTemplate, targets: emptyTargets() })
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
    setTemplate(raidID ? loadTemplate(raidID) : { text: defaultRecruitmentTemplate, targets: emptyTargets() })
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

  const { counts, missing, unclassified } = recruitmentNeeds(attendance, template.targets)
  const preview = renderRecruitmentText(template.text, missing)
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
      <div className="configuration-grid">
        {recruitmentRoles.map((role) => <label key={role.key}>
          {role.label} (aktuell {counts[role.key]}, fehlen {missing[role.key]})
          <input type="number" min={0} step={1} disabled={!raidID} value={template.targets[role.key]}
            onChange={(event) => {
              const value = event.target.valueAsNumber
              if (!Number.isSafeInteger(value) || value < 0) return
              updateTemplate({ ...template, targets: { ...template.targets, [role.key]: value } })
            }} />
        </label>)}
      </div>
      {unclassified > 0 && <p className="error" role="status">{unclassified} DD(s) konnten aufgrund fehlender Klassen-/Spec-Informationen nicht Nah- oder Fernkampf zugeordnet werden. Bitte Aufstellung prüfen.</p>}
    </section>
    <section className="card">
      <h2>Textvorlage</h2>
      <p className="muted">Platzhalter: {recruitmentRoles.map((role) => role.token).join(', ')}. Alle anderen Wörter bleiben unverändert.</p>
      <textarea aria-label="Gesuchtext-Vorlage" rows={5} value={template.text} disabled={!raidID}
        onChange={(event) => updateTemplate({ ...template, text: event.target.value })} />
      <h3>Aktueller Text zum Kopieren</h3>
      <textarea aria-label="Fertiger Gesuchtext" readOnly rows={5} value={preview} />
      <div className="actions"><button type="button" disabled={!raidID || !!error} onClick={() => void copyText()}>Text kopieren</button><span role="status">{copyStatus}</span></div>
    </section>
  </div>
}
