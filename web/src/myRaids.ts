import { canonicalClass } from './characterOptions'
import type { Character, Raid, SignupOverview } from './types'

export type MyRaidState = 'absent' | 'tentative' | 'accepted' | 'signed' | 'open' | 'closed'

export interface MyRaidStatus {
  state: MyRaidState
  /** Short German status for the badge. */
  label: string
  /** The signed-up or accepted characters, e.g. "Funkenflug (Feuer)". */
  detail: string
  reason: string
}

/** How the signed-in player stands for one raid, from that raid's signup overview. */
export function myRaidStatus(overview: SignupOverview, playerID: number): MyRaidStatus {
  const absence = (overview.absences ?? []).find((row) => row.player_id === playerID)
  const own = (overview.entries ?? []).filter((row) => row.player_id === playerID && !row.is_withdrawn)
  const accepted = own.filter((row) => row.is_accepted)
  const describe = (rows: typeof own) => rows.map((row) => `${row.character_name} (${row.spec})`).join(', ')
  if (absence && (absence.status ?? 'DECLINED') === 'DECLINED') return { state: 'absent', label: 'Abgesagt', detail: describe(own), reason: absence.reason }
  if (absence) return { state: 'tentative', label: 'Unsicher', detail: describe(own), reason: absence.reason }
  if (accepted.length) return { state: 'accepted', label: 'Bestätigt', detail: describe(accepted), reason: '' }
  if (own.length) return { state: 'signed', label: 'Angemeldet', detail: describe(own), reason: '' }
  if (overview.is_open) return { state: 'open', label: 'Noch nicht angemeldet', detail: '', reason: '' }
  return { state: 'closed', label: 'Anmeldung geschlossen', detail: '', reason: '' }
}

/** The character offered for one-click signup: the main, else the first active one. */
export function quickSignupChoice(characters: Character[], classSpecs: Record<string, string[]>): { character: Character; spec: string } | null {
  const active = characters.filter((character) => character.active)
  const character = active.find((row) => row.is_main) ?? active[0]
  if (!character) return null
  const specs = classSpecs[canonicalClass(character.class)] ?? []
  if (!specs.length) return null
  return { character, spec: specs.includes(character.main_spec) ? character.main_spec : specs[0] }
}

/** Active raids that have not started yet, soonest first. */
export function upcomingRaids(raids: Raid[], now: number, limit: number): Raid[] {
  return raids
    .filter((raid) => !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN') && !!raid.scheduled_start && Date.parse(raid.scheduled_start) > now)
    .sort((a, b) => Date.parse(a.scheduled_start!) - Date.parse(b.scheduled_start!) || a.id - b.id)
    .slice(0, limit)
}
