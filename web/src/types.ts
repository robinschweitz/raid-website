export type Role = 'GUEST' | 'MEMBER' | 'RAIDASSISTENT' | 'RAIDLEADER' | 'ADMIN'

export const ROLE_RANK: Record<Role, number> = {
  GUEST: 0,
  MEMBER: 1,
  RAIDASSISTENT: 2,
  RAIDLEADER: 3,
  ADMIN: 4,
}

export function atLeast(role: Role, min: Role): boolean {
  return ROLE_RANK[role] >= ROLE_RANK[min]
}

export interface Player {
  id: number
  name: string
  guild_rank_id: number | null
  username: string | null
  role: Role
  active: boolean
  created_at: string
  updated_at: string
}

export interface Character {
  id: number
  player_id: number
  name: string
  name_changed_at: string | null
  class: string
  main_spec: string
  second_spec: string
  is_main: boolean
  active: boolean
  created_at: string
  updated_at: string
}

export interface NameHistory {
  id: number
  character_id: number
  old_name: string
  new_name: string
  actor_id: number
  reason: string
  created_at: string
}

export type RaidStatus = 'PLANNED' | 'OPEN' | 'COMPLETED' | 'CANCELLED'

export interface Raid {
  id: number
  name: string
  raid_leader_id: number
  status: RaidStatus
  scheduled_start: string | null
  scheduled_end: string | null
  started_at: string | null
  completed_at: string | null
  cancelled_at: string | null
  archived_at: string | null
  notes: string
  created_at: string
  updated_at: string
}

export type AttendanceRole = 'TANK' | 'HEAL' | 'DD'

export interface Attendance {
  id: number
  raid_id: number
  player_id: number
  character_id: number
  role: AttendanceRole
  is_substitute: boolean
  is_bench: boolean
  late_join: boolean
  early_leave: boolean
  notes: string
  created_at: string
  updated_at: string
}

export interface LootAward {
  id: number
  raid_id: number
  boss: string
  item_id: string
  item_name: string
  awarded_to_player_id: number
  awarded_to_character_id: number | null
  awarded_by_player_id: number
  source: string
  is_main_spec: boolean
  item_lock_id: number | null
  notes: string
  awarded_at: string
  created_at: string
  updated_at: string
  token_award?: boolean
}

export interface PointTransaction {
  id: number
  player_id: number
  raid_id: number | null
  amount: number
  type: string
  reason: string
  ref_type: string
  ref_id: string
  actor_id: number
  created_at: string
}

export interface TokenTransaction {
  id: number
  player_id: number
  raid_id: number | null
  loot_award_id: number | null
  amount: number
  type: string
  reason: string
  ref_type: string
  ref_id: string
  actor_id: number
  created_at: string
}

export interface PointsResponse {
  player_id: number
  balance: number
  transactions: PointTransaction[]
}

export interface TokensResponse {
  player_id: number
  total: number
  transactions: TokenTransaction[]
}

export type LockStatus = 'ACTIVE' | 'USED' | 'EXPIRED' | 'CANCELLED'

export interface ItemLock {
  id: number
  player_id: number
  raid_id: number
  item_id: string
  item_name: string
  boss: string
  status: LockStatus
  locked_at: string
  used_at: string | null
  expired_at: string | null
  cancelled_at: string | null
  actor_id: number
  notes: string
  created_at: string
  updated_at: string
}

export interface GuildRank {
  id: number
  name: string
  sort_order: number
  active: boolean
  created_at: string
  updated_at: string
}

export interface RaidGroup {
  id: number
  name: string
  color: string
  sort_order: number
  active: boolean
  created_at: string
  updated_at: string
}

export type PollStatus = 'OPEN' | 'CLOSED' | 'ARCHIVED'

export interface Poll {
  id: number
  title: string
  description: string
  status: PollStatus
  closes_at: string | null
  created_by: number
  created_at: string
  updated_at: string
}

export interface Vote {
  id: number
  poll_id: number
  player_id: number
  choice: string
  voted_at: string
}

export interface ImportCounts {
  players: number
  characters: number
  raids: number
  attendance: number
  loot: number
  tokens: number
  points: number
}

export interface ImportReport {
  mode: string
  source: string
  raw_hash: string
  rows_read: number
  players: number
  characters: number
  raids: number
  attendance: number
  loot: number
  tokens: number
  points_entries: number
  created: ImportCounts
  skipped: boolean
  warnings: string[] | null
  errors: string[] | null
}

export interface ImportRecord {
  id: number
  actor_id: number
  source: string
  source_url: string
  raw_hash: string
  dry_run_report: string
  status: string
  warnings_count: number
  errors_count: number
  created_count: number
  updated_count: number
  unchanged_count: number
  started_at: string | null
  completed_at: string | null
  notes: string
}

export interface RollResult {
  player_id: number
  cost: number
  roll: number
  min: number
  max: number
  balance_after: number
}
