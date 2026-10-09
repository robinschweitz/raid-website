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

export interface PlayerSummary {
  id: number
  name: string
  guild_rank_id: number | null
  active: boolean
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

export interface ExternalCharacter {
  profile_id: string
  name: string
  class: string
  main_spec: string
  second_spec: string
  level: number
  race: string
  faction: string
  profile_url: string
}

export interface RegistrationRequest {
  id: number
  requester_player_id: number | null
  status: string
  purpose: string
  external_source: string
  external_profile_id: string
  external_profile_url: string
  character_name: string
  class: string
  main_spec: string
  second_spec: string
  level: number
  race: string
  faction: string
  created_at: string
  updated_at: string
}

export interface RegistrationCode {
  id: number
  request_id: number
  created_by_player_id: number
  created_at: string
  expires_at: string
  used_at: string | null
  used_by_player_id: number | null
  revoked_at: string | null
}

export interface GeneratedRegistrationCode {
  code_record: RegistrationCode
  code: string
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
  series_id?: number | null
  previous_raid_id?: number | null
  catalog_instance_id?: string | null
  raid_size?: number | null
  notes: string
  created_at: string
  updated_at: string
}

export type AttendanceRole = 'TANK' | 'HEAL' | 'DD'

export interface RaidSignup {
  id: number
  player_id: number
  character_id: number
  character_name: string
  class: string
  spec: string
  is_withdrawn: boolean
  is_accepted: boolean
  attendance_id: number
}

export interface SignupOverview {
  is_enabled: boolean
  is_open: boolean
  entries: RaidSignup[]
  class_specs: Record<string, string[]>
}

export interface Attendance {
  id: number
  raid_id: number
  player_id: number
  character_id: number
  character_name?: string
  class?: string
  spec?: string
  role: AttendanceRole
  is_substitute: boolean
  is_bench: boolean
  late_join: boolean
  early_leave: boolean
  notes: string
  created_at: string
  updated_at: string
}

export interface RaidSeries {
  id: number
  name: string
  name_template: string
  next_raid_number: number
  raid_leader_id: number
  weekdays: number[]
  interval_weeks: number
  start_time: string
  timezone: string
  catalog_instance_id?: string | null
  raid_size?: number | null
}

export interface CatalogSource {
  id: string
  revision: string
  url: string
  license: string
  attribution: string
}

export interface CatalogInstance { id: string; name: string }
export interface CatalogBoss { id: string; instance_id: string; name: string }
export interface CatalogItem {
  item_id: string;
  name: string;
  icon?: string;
  display_id?: number;
  icon_name?: string;
  binding?: number;
  binding_type?: string;
  quality?: number;
  item_level?: number;
  required_level?: number;
  slot?: string;
  type?: string;
  class_mask?: number;
  subclass?: number;
  set_name?: string;
  armor?: number;
  stats?: Record<string, number>;
  sockets?: string[];
  socket_bonus?: { text: string; stats?: Record<string, number> };
  effects?: string[];
  requirements?: string[];
  unique?: boolean;
  weapon?: { min_damage?: number; max_damage?: number; speed?: number; dps?: number };
  locales?: Record<string, { name: string; icon?: string; tooltip_html?: string; tooltip_text?: string; fallback_locale?: string; fetch_error?: string }>;
  stamina?: number;
  strength?: number;
  agility?: number;
  intellect?: number;
  spirit?: number;
  spell_power?: number;
  min_damage?: number;
  max_damage?: number;
  dps?: number;
}
export type CatalogMode = 'NORMAL' | 'HEROIC' | 'HARDMODE'
export const catalogModeLabels: Record<CatalogMode, string> = { NORMAL: 'Normal', HEROIC: 'Heroisch', HARDMODE: 'Hardmode' }
export interface CatalogItemDrop extends CatalogItem {
  boss_id: string
  raid_size: number
  mode: CatalogMode
  source_variants: { difficulty: string; loot_mode: number }[]
  conditional: boolean
}
export interface CatalogProvenance { source: CatalogSource; raw_hash: string; notices: string[] }
export interface LootCatalog {
  sources: CatalogProvenance[]
  instances: CatalogInstance[]
  bosses: CatalogBoss[]
  items: CatalogItem[]
  drops: { boss_id: string; item_id: string; raid_size: number; difficulty: string; loot_mode: number }[]
}

export function catalogRaidSizes(catalog: LootCatalog | null, instanceID: string): number[] {
  if (!catalog) return []
  const bosses = new Set(catalog.bosses.filter((boss) => boss.instance_id === instanceID).map((boss) => boss.id))
  return [...new Set(catalog.drops.filter((drop) => bosses.has(drop.boss_id)).map((drop) => drop.raid_size))].sort((a, b) => a - b)
}

export interface ArchiveResponse {
  data: Raid
  next_raid: Raid | null
}

export interface LootAward {
  id: number
  raid_id: number
  boss: string
  item_id: string
  item_name: string
  awarded_to_player_id: number
  awarded_to_character_id: number | null
  awarded_to_character_name: string
  awarded_by_player_id: number
  source: string
  is_main_spec: boolean
  award_method?: 'MAIN' | 'SECOND' | 'ENHANCED' | 'BOE'
  enhanced_cost?: number | null
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
export type PollType = 'TEXT' | 'CHOICE'

export interface PollOption {
  id: number
  poll_id: number
  label: string
  sort_order: number
}

export interface Poll {
  id: number
  title: string
  description: string
  poll_type: PollType
  allow_multiple: boolean
  status: PollStatus
  closes_at: string | null
  created_by: number
  created_at: string
  updated_at: string
  options: PollOption[]
}

export interface WishlistWish {
  id: number
  player_id: number
  character_id: number
  item_id: string
  item_name: string
  priority: number
  notes: string
  created_at: string
  updated_at: string
}

export interface WishlistCharacterInterest {
  character_id: number
  character_name: string
  player_id: number
  player_name: string
}

export interface RaidWishlistItem {
  item_id: string
  characters: WishlistCharacterInterest[]
  count: number
}


export interface Vote {
  id: number
  poll_id: number
  player_id: number
  choice: string
  choices: string[]
  voted_at: string
}

export interface ImportCounts {
	series?: number
  players: number
  characters: number
  raids: number
  attendance: number
  loot: number
  tokens: number
  points: number
}

export interface ImportReport {
	archived_raids?: number
	setup?: { name: string; leader: string; scheduled_start: string; timezone: string; weekdays: number[]; interval_weeks: number; attendance: number; already_imported: boolean }
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
