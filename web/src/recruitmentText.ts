import { rosterGroup } from './raidRoster'
import type { Attendance } from './types'

export const recruitmentRoles = [
  { key: 'TANK', label: 'Tanks', token: '{{TANK}}' },
  { key: 'HEAL', label: 'Heiler', token: '{{HEAL}}' },
  { key: 'MELEE', label: 'MDDs', token: '{{MDD}}' },
  { key: 'RANGED', label: 'RDDs', token: '{{RDD}}' },
] as const

export type RecruitmentRole = typeof recruitmentRoles[number]['key']
export type RecruitmentTargets = Record<RecruitmentRole, number>

export const emptyTargets = (): RecruitmentTargets => ({ TANK: 0, HEAL: 0, MELEE: 0, RANGED: 0 })

export function recruitmentNeeds(attendance: Attendance[], targets: RecruitmentTargets) {
  const counts = emptyTargets()
  let unclassified = 0
  for (const row of attendance) {
    if (row.is_bench) continue
    const group = rosterGroup(row)
    if (group === 'UNKNOWN') unclassified++
    else counts[group]++
  }
  const missing = emptyTargets()
  for (const role of recruitmentRoles) missing[role.key] = Math.max(0, targets[role.key] - counts[role.key])
  return { counts, missing, unclassified }
}

export function renderRecruitmentText(template: string, missing: RecruitmentTargets): string {
  return template.replace(/{{(TANK|HEAL|MDD|RDD)}}/g, (_, token: string) => {
    const role = token === 'MDD' ? 'MELEE' : token === 'RDD' ? 'RANGED' : token
    return String(missing[role as RecruitmentRole])
  })
}

export const defaultRecruitmentTemplate = 'Wir suchen noch {{TANK}} Tanks, {{HEAL}} Heiler, {{RDD}} RDDs und {{MDD}} MDDs.'

// Recurring raids create a new raid every week, so a template belongs to the
// series; raids without a series keep their own template.
export const recruitmentStorageKey = (raidID: number, seriesID?: number | null) =>
  seriesID ? `raid-recruitment-template-v2:series:${seriesID}` : `raid-recruitment-template-v1:${raidID}`
