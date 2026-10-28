import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { catalogModeLabels, type BossKill, type CatalogBoss, type CatalogMode, type LootCatalog } from '../types'
import { ErrorMessage } from './ErrorMessage'

interface Props {
  raidID: number
  canWrite: boolean
  needsReason: boolean
  bosses: CatalogBoss[]
  catalog: LootCatalog | null
  raidSize: number | null
}

// Difficulties the catalog offers for a boss at this raid size (FR-29.6).
function difficultiesFor(catalog: LootCatalog | null, bossID: string, raidSize: number | null): CatalogMode[] {
  const modes = new Set(catalog?.drops.filter((drop) => drop.boss_id === bossID && drop.raid_size === raidSize).map((drop) => drop.difficulty))
  return (Object.keys(catalogModeLabels) as CatalogMode[]).filter((mode) => modes.has(mode))
}

// The kills of a raid (FR-29). Everyone signed in reads them; staff manage them.
export function RaidKills({ raidID, canWrite, needsReason, bosses, catalog, raidSize }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [kills, setKills] = useState<BossKill[] | null>(null)
  const [error, setError] = useState('')
  const [bossID, setBossID] = useState('')
  const [difficulty, setDifficulty] = useState('')
  const [reason, setReason] = useState('')

  const load = async () => {
    const rows = await api.get<BossKill[]>(`/api/v1/raids/${raidID}/kills`)
    if (isActive()) setKills(rows)
  }

  useEffect(() => {
    void load().catch(() => { if (isActive()) setError('Bosskills konnten nicht geladen werden.') })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [raidID])

  const write = (action: () => Promise<unknown>, failure: string) => submit(async () => {
    setError('')
    try {
      await action()
      setReason('')
      await load()
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : failure)
    }
  })

  const record = (event: FormEvent) => {
    event.preventDefault()
    void write(async () => {
      await api.post(`/api/v1/raids/${raidID}/kills`, { catalog_boss_id: bossID, difficulty, reason })
      setBossID('')
      setDifficulty('')
    }, 'Kill konnte nicht gespeichert werden.')
  }

  const open = bosses.filter((boss) => !kills?.some((kill) => kill.catalog_boss_id === boss.id))
  const offered = difficultiesFor(catalog, bossID, raidSize)
  const reasonMissing = needsReason && !reason.trim()

  return <section className="subcard raid-kills" aria-label="Bosskills">
    <h3>Bosskills</h3>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {kills === null ? (error ? null : <p className="muted" role="status">Bosskills werden geladen&hellip;</p>)
      : kills.length === 0 ? <p className="muted">Für diesen Raid sind keine Kills erfasst.</p>
        : <div className="table-scroll"><table className="table small">
          <thead><tr><th>Boss</th><th>Schwierigkeit</th>{canWrite && <th></th>}</tr></thead>
          <tbody>{kills.map((kill) => <tr key={kill.id}>
            <td><strong>{kill.boss}</strong></td>
            <td>{canWrite
              ? <select aria-label={`Schwierigkeit ${kill.boss}`} disabled={pending || reasonMissing} value={kill.difficulty}
                onChange={(e) => void write(() => api.patch(`/api/v1/raids/${raidID}/kills/${kill.id}`, { difficulty: e.target.value, reason }), 'Schwierigkeit konnte nicht geändert werden.')}>
                {[...new Set([kill.difficulty, ...difficultiesFor(catalog, kill.catalog_boss_id, raidSize)])].map((mode) => <option key={mode} value={mode}>{catalogModeLabels[mode]}</option>)}
              </select>
              : <span className="badge">{catalogModeLabels[kill.difficulty]}</span>}</td>
            {canWrite && <td><button type="button" disabled={pending || reasonMissing} onClick={() => void write(() => api.post(`/api/v1/raids/${raidID}/kills/${kill.id}/remove`, { reason }), 'Kill konnte nicht entfernt werden.')}>Entfernen</button></td>}
          </tr>)}</tbody>
        </table></div>}
    {canWrite && needsReason && <label>Grund (Pflicht im archivierten Raid)<input value={reason} onChange={(e) => setReason(e.target.value)} /></label>}
    {canWrite && <details className="editor-disclosure"><summary>Kill eintragen</summary>
      <form onSubmit={record}>
        <label>Boss<select value={bossID} onChange={(e) => { setBossID(e.target.value); setDifficulty('') }} required>
          <option value="">Boss wählen</option>
          {open.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
        </select></label>
        <label>Schwierigkeit<select value={difficulty} onChange={(e) => setDifficulty(e.target.value)} required disabled={!bossID}>
          <option value="">Schwierigkeit wählen</option>
          {offered.map((mode) => <option key={mode} value={mode}>{catalogModeLabels[mode]}</option>)}
        </select></label>
        <div className="actions"><button type="submit" disabled={pending || !bossID || !difficulty || reasonMissing}>Eintragen</button></div>
      </form>
    </details>}
  </section>
}
