import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import type { AddonInviteExport as InviteExport } from '../types'

interface Props {
  raidID: number
  rosterKey: string
}

export function AddonInviteExport({ raidID, rosterKey }: Props) {
  const [includeBench, setIncludeBench] = useState(false)
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let active = true
    setCopied(false)
    api.get<InviteExport>(`/api/v1/raids/${raidID}/addon-invite-export${includeBench ? '?bench=1' : ''}`)
      .then((result) => { if (active) { setText(result.text); setError('') } })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Export konnte nicht geladen werden.') })
    return () => { active = false }
  }, [raidID, includeBench, rosterKey])

  const memberCount = text ? text.split('\n').length - 1 : 0
  const copy = async () => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
  }

  return <section className="subcard" aria-label="Einladungen für Addon">
    <h3>Einladungen für Addon</h3>
    <p className="muted">In ForgeLootLedger unter „RAID“ (oder /forgeloot setup) einfügen und „EINLESEN“ klicken. Das Addon lädt diese Charaktere ingame in den Raid ein.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <label className="check"><input type="checkbox" checked={includeBench} onChange={(e) => setIncludeBench(e.target.checked)} /> Ersatzbank einbeziehen</label>
    <textarea className="addon-code-export" aria-label="Einladungs-Export" readOnly value={text} rows={Math.min(8, Math.max(3, memberCount + 1))} />
    <div className="actions">
      <button type="button" disabled={memberCount === 0} onClick={() => void copy()}>Für Addon kopieren</button>
      <span className="muted">{memberCount} Charaktere</span>
      {copied && <span className="muted" role="status">Kopiert.</span>}
    </div>
  </section>
}
