import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { shadowmourneRaidTasks, shadowmourneStageLabels, shadowmourneTaskHints } from '../shadowmourne'
import { atLeast, type ShadowmourneEntry, type ShadowmourneRaidPlan, type ShadowmourneRaidTaskName } from '../types'
import { useSubmission } from '../useSubmission'

interface Props {
  raidID: number
  // Changes whenever the roster changes, so candidates are reloaded.
  rosterKey: string
}

function entryName(entry: ShadowmourneEntry): string {
  return `${entry.character_name} (${entry.player_name})`
}

// Shows who does the Shadowmourne steps that only one raid member can do per raid (ADR 086).
export function ShadowmourneRaidCard({ raidID, rosterKey }: Props) {
  const { player: me } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [plan, setPlan] = useState<ShadowmourneRaidPlan | null>(null)
  const [error, setError] = useState('')
  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false

  useEffect(() => {
    api.get<ShadowmourneRaidPlan>(`/api/v1/raids/${raidID}/shadowmourne`)
      .then((loaded) => { if (isActive()) setPlan(loaded) })
      .catch(() => { if (isActive()) setPlan(null) })
  }, [raidID, rosterKey, isActive])

  if (!plan || (plan.candidates.length === 0 && plan.tasks.every((task) => !task.assigned))) return null

  const assign = async (task: ShadowmourneRaidTaskName, value: string) => {
    await submit(async () => {
      setError('')
      try {
        const updated = await api.put<ShadowmourneRaidPlan>(`/api/v1/raids/${raidID}/shadowmourne/${task}`, { entry_id: value ? Number(value) : null })
        if (isActive()) setPlan(updated)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Die Einteilung konnte nicht gespeichert werden.')
      }
    })
  }

  const editable = canManage && plan.editable
  return <section className="subcard shadowmourne-raid" aria-label="Schattengram im Raid">
    <div className="section-heading"><h2>Schattengram im Raid</h2><Link to="/schattengram">Zur Liste</Link></div>
    <p className="muted">Diese Schritte kann pro Raid nur eine Person machen. Vorschlag: der höchste Listenplatz in der Aufstellung, der gerade bei diesem Schritt ist.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="shadowmourne-task-grid">
      {shadowmourneRaidTasks.map((name) => {
        const task = plan.tasks.find((item) => item.task === name)
        const chosen = task?.assigned ?? task?.suggested ?? null
        return <div key={name} className="shadowmourne-task">
          <p className="eyebrow">{shadowmourneStageLabels[name]} · {shadowmourneTaskHints[name]}</p>
          {chosen ? <p><strong>{chosen.character_name}</strong> <span className="muted">{chosen.player_name}</span>{name === 'SHARDS' && <span className="muted"> · {chosen.shards_collected} / {chosen.shard_target}</span>}</p> : <p className="muted">Niemand</p>}
          {chosen && <p className="muted">{task?.assigned ? 'Von der Raidleitung eingeteilt' : 'Vorschlag aus der Liste'}</p>}
          {editable && <label>Einteilung<select aria-label={`Einteilung ${shadowmourneStageLabels[name]}`} value={task?.assigned ? String(task.assigned.id) : ''} disabled={pending} onChange={(event) => void assign(name, event.target.value)}>
            <option value="">{task?.suggested ? `Vorschlag: ${task.suggested.character_name}` : 'Vorschlag (niemand)'}</option>
            {plan.candidates.map((entry) => <option key={entry.id} value={entry.id}>{entryName(entry)} · {shadowmourneStageLabels[entry.stage]}</option>)}
          </select></label>}
        </div>
      })}
    </div>
  </section>
}
