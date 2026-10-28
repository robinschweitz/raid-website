import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { shadowmourneRaidTasks, shadowmourneStageLabels, shadowmourneTaskHints } from '../shadowmourne'
import { atLeast, type ShadowmourneRaidCharacter, type ShadowmourneRaidPlan, type ShadowmourneRaidTaskName } from '../types'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from './ErrorMessage'

interface Props {
  raidID: number
  // Changes whenever the roster changes, so the options are reloaded.
  rosterKey: string
}

function characterLabel(character: ShadowmourneRaidCharacter): string {
  return `${character.character_name} (${character.player_name})`
}

// Shows who does the Shadowmourne steps that only one raid member can do per raid (ADR 086, ADR 087).
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

  if (!plan) return null
  const editable = canManage && plan.editable
  const hasChoice = plan.tasks.some((task) => task.assigned || task.suggested)
  const hasOptions = plan.tasks.some((task) => task.options.length > 0)
  if (!hasChoice && !(editable && hasOptions)) return null

  const assign = async (task: ShadowmourneRaidTaskName, value: string) => {
    await submit(async () => {
      setError('')
      try {
        const updated = await api.put<ShadowmourneRaidPlan>(`/api/v1/raids/${raidID}/shadowmourne/${task}`, { character_id: value ? Number(value) : null })
        if (isActive()) setPlan(updated)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Die Einteilung konnte nicht gespeichert werden.')
      }
    })
  }

  return <section className="subcard shadowmourne-raid" aria-label="Schattengram im Raid">
    <div className="section-heading"><h2>Schattengram im Raid</h2><Link to="/schattengram">Zur Liste</Link></div>
    <p className="muted">Diese Schritte kann pro Raid nur eine Person machen. Vorschlag: der höchste Listenplatz in der Aufstellung, der gerade bei diesem Schritt ist. Die Raidleitung kann auch Krieger, Paladine und Todesritter aus der Aufstellung wählen, die nicht auf der Liste stehen.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <div className="shadowmourne-task-grid">
      {shadowmourneRaidTasks.map((name) => {
        const task = plan.tasks.find((item) => item.task === name)
        const chosen = task?.assigned ?? task?.suggested ?? null
        const listed = task?.options.filter((option) => option.entry) ?? []
        const unlisted = task?.options.filter((option) => !option.entry) ?? []
        return <div key={name} className="shadowmourne-task">
          <p className="eyebrow">{shadowmourneStageLabels[name]} · {shadowmourneTaskHints[name]}</p>
          {chosen ? <p><strong>{chosen.character_name}</strong> <span className="muted">{chosen.player_name}</span>{name === 'SHARDS' && chosen.entry && <span className="muted"> · {chosen.entry.shards_collected} / {chosen.entry.shard_target}</span>}</p> : <p className="muted">Niemand</p>}
          {chosen && <p className="muted">{task?.assigned ? 'Von der Raidleitung eingeteilt' : 'Vorschlag aus der Liste'}</p>}
          {editable && <label>Einteilung<select aria-label={`Einteilung ${shadowmourneStageLabels[name]}`} value={task?.assigned ? String(task.assigned.character_id) : ''} disabled={pending} onChange={(event) => void assign(name, event.target.value)}>
            <option value="">{task?.suggested ? `Vorschlag: ${task.suggested.character_name}` : 'Vorschlag (niemand)'}</option>
            {listed.length > 0 && <optgroup label={`Liste: bei ${shadowmourneStageLabels[name]}`}>{listed.map((option) => <option key={option.character_id} value={option.character_id}>{characterLabel(option)}</option>)}</optgroup>}
            {unlisted.length > 0 && <optgroup label="Nicht auf der Liste">{unlisted.map((option) => <option key={option.character_id} value={option.character_id}>{characterLabel(option)} · {option.character_class}</option>)}</optgroup>}
          </select></label>}
        </div>
      })}
    </div>
  </section>
}
