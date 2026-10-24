import { canonicalClass } from './characterOptions'
import type { Attendance, Character, Raid, SignupOverview } from './types'

export type MyRaidState = 'absent' | 'tentative' | 'accepted' | 'signed' | 'open' | 'closed'

export interface MyRaidStatus {
  state: MyRaidState
  /** Short German status for the badge. */
  label: string
  /** The signed-up or accepted characters, e.g. "Funkenflug (Feuer)". */
  detail: string
  reason: string
}

/**
 * How the signed-in player stands for one raid, from that raid's signup overview
 * and roster. A roster row counts even without an own signup, because staff may
 * add a character directly.
 */
export function myRaidStatus(overview: SignupOverview, playerID: number, attendance: Attendance[] = []): MyRaidStatus {
  const absence = (overview.absences ?? []).find((row) => row.player_id === playerID)
  const own = (overview.entries ?? []).filter((row) => row.player_id === playerID && !row.is_withdrawn)
  const rostered = attendance.filter((row) => row.player_id === playerID)
  const describe = (rows: { character_name?: string; spec?: string }[]) => rows.map((row) => `${row.character_name ?? 'Unbekannt'} (${row.spec ?? '?'})`).join(', ')
  const accepted = own.filter((row) => row.is_accepted)
  const placed = rostered.length ? describe(rostered) : describe(accepted)
  if (absence && (absence.status ?? 'DECLINED') === 'DECLINED') return { state: 'absent', label: 'Abgesagt', detail: placed || describe(own), reason: absence.reason }
  if (absence) return { state: 'tentative', label: 'Unsicher', detail: placed || describe(own), reason: absence.reason }
  if (placed) return { state: 'accepted', label: 'Bestätigt', detail: placed, reason: '' }
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

export type SignupGroupState = 'pending' | 'accepted' | 'tentative' | 'declined'

export interface SignupListRow {
  playerID: number
  playerName: string
  state: SignupGroupState
  reason: string
  /** Live signups of this player, accepted first. */
  entries: SignupOverview['entries']
  /** Roster rows of this player that no signup covers, e.g. added directly by staff. */
  directRoster: Attendance[]
}

const groupOrder: SignupGroupState[] = ['pending', 'accepted', 'tentative', 'declined']

/**
 * One row per player who signed up, stands in the roster, or sent an absence,
 * ordered by state (pending first, so staff see open work at the top) and then by name.
 */
export function signupList(overview: SignupOverview, attendance: Attendance[], playerName: (id: number) => string): SignupListRow[] {
  const entries = (overview.entries ?? []).filter((row) => !row.is_withdrawn)
  const absences = overview.absences ?? []
  const ids = new Set<number>([...entries.map((row) => row.player_id), ...attendance.map((row) => row.player_id), ...absences.map((row) => row.player_id)])
  const rows = [...ids].map((playerID): SignupListRow => {
    const absence = absences.find((row) => row.player_id === playerID)
    const own = entries.filter((row) => row.player_id === playerID).sort((a, b) => Number(b.is_accepted) - Number(a.is_accepted) || a.character_name.localeCompare(b.character_name, 'de'))
    const roster = attendance.filter((row) => row.player_id === playerID)
    const state: SignupGroupState = absence ? ((absence.status ?? 'DECLINED') === 'DECLINED' ? 'declined' : 'tentative') : roster.length ? 'accepted' : 'pending'
    return {
      playerID, playerName: playerName(playerID), state, reason: absence?.reason ?? '', entries: own,
      directRoster: roster.filter((row) => !own.some((entry) => entry.character_id === row.character_id)),
    }
  })
  return rows.sort((a, b) => groupOrder.indexOf(a.state) - groupOrder.indexOf(b.state) || a.playerName.localeCompare(b.playerName, 'de') || a.playerID - b.playerID)
}
