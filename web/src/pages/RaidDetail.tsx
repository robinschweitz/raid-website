import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { EpShortfallWarning, type EpShortfall } from '../components/EpShortfallWarning'
import { useAuth } from '../auth'
import { Link } from '../router'
import { RaidLootList } from '../components/RaidLootList'
import { RaidBanner } from '../components/RaidBanner'
import { RaidRosterBoard } from '../components/RaidRosterBoard'
import { RaidSignups } from '../components/RaidSignups'
import { atLeast, catalogModeLabels, catalogRaidSizes, type ArchiveResponse, type Absence, type Attendance, type AttendanceRole, type SignupOverview, type CatalogBoss, type CatalogItemDrop, type Character, type LootAward, type LootCatalog, type Player, type Raid, type RaidSeries, isDeclined } from '../types'
import { matchesCatalogItem } from '../catalog'
import { canonicalClass, useCharacterOptions } from '../characterOptions'
import { SpecSelect } from '../components/SpecSelect'
import { RaidQuickAdd } from '../components/RaidQuickAdd'
import { AddonInviteExport } from '../components/AddonInviteExport'
import { AddonLootImport } from '../components/AddonLootImport'
import { UnclaimedLoot } from '../components/UnclaimedLoot'
import { RaidKills } from '../components/RaidKills'
import { AddonRosterImport } from '../components/AddonRosterImport'
import { RaidRecruitment } from './Recruitment'
import { ShadowmourneRaidCard } from '../components/ShadowmourneRaidCard'
import { ErrorMessage } from '../components/ErrorMessage'

interface Props {
  raidID: number
  /** Opens a specific tab, e.g. from the "Meine nächsten Raids" links. */
  initialView?: 'signups'
}

type AwardMethod = 'MAIN' | 'SECOND' | 'ENHANCED' | 'BOE'
interface ArchivePreviewRow { player_id: number; balance: number; award: number; reason: 'award' | 'main_spec_loot' | 'already_awarded' }
type RecordedLoot = LootAward & { award_method?: AwardMethod; enhanced_cost?: number | null; catalog_boss_id?: string | null }
const dropKey = (item: CatalogItemDrop) => JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.mode])
const awardLabels: Record<AwardMethod, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const awardMethod = (row: RecordedLoot): AwardMethod => row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
// The list is already limited to one boss, difficulty and raid size, so the name is enough.
// The item ID is only added when two drops in that list share a name.
const catalogItemLabel = (item: CatalogItemDrop, siblings: CatalogItemDrop[]) => {
  const sameName = siblings.some((other) => other !== item && other.name === item.name)
  return `${item.name}${item.conditional ? ' · Bedingte Beute' : ''}${sameName ? ` · Item-ID ${item.item_id}` : ''}`
}

const attendanceFlags = [
  { key: 'is_bench', label: 'Ersatzbank' },
  { key: 'late_join', label: 'Später dabei' },
  { key: 'early_leave', label: 'Früher gegangen' },
] as const

const snapshotFields = ['character_name', 'class', 'spec'] as const
type SnapshotField = typeof snapshotFields[number]

function localStart(timestamp: string | null | undefined): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

function epShortfalls(err: unknown): EpShortfall[] | null {
  if (!(err instanceof ApiError) || err.code !== 'ep_shortfall') return null
  const data = err.data as { shortfalls?: EpShortfall[] } | undefined
  return data?.shortfalls?.length ? data.shortfalls : null
}

function raidError(err: unknown, fallback: string): string {
  if (!(err instanceof ApiError)) return fallback
  if (err.code === 'reason_required') {
    return 'Für eine Änderung im Archiv ist eine nichtleere Begründung erforderlich.'
  }
  if (err.code === 'raid_state') return 'Der Raid erlaubt diese Änderung nicht mehr. Bitte die Seite neu laden.'
  const messages: Record<string, string> = {
    series_required: 'Zum Archivieren muss zuerst eine Raidserie zugeordnet werden.',
    scheduled_start_required: 'Zum Archivieren fehlt der geplante Start. Bitte den Start in der Raidplanung eintragen und speichern.',
    series_not_found: 'Die Raidserie existiert nicht mehr. Bitte neu laden und eine gültige Serie wählen.',
    series_not_editable: 'Bei diesem Raid kann die Serie nicht mehr geändert werden.',
    schedule_not_editable: 'Die Planung eines abgeschlossenen, abgesagten oder archivierten Raids kann nicht mehr geändert werden. Bitte neu laden.',
    name_required: 'Bitte einen Raidnamen eintragen.',
    attendance_has_loot: 'Dieser Charakter hat in diesem Raid schon Loot erhalten. Bitte zuerst den Loot zurückziehen.',
    name_not_editable: 'Raidnamen aus einer Raidserie können nicht direkt geändert werden.',
    invalid_scheduled_start: 'Bitte einen gültigen geplanten Start angeben.',
    roster_incomplete: 'Die Teilnahme ist unvollständig. Bitte die betroffenen Zeilen korrigieren: gespeicherter Charaktername, gültige Klasse, tatsächlicher Spec und Rolle sind zum Archivieren erforderlich.',
    occurrence_conflict: 'Der nächste Serientermin existiert bereits. Das Archivieren wurde nicht gespeichert.',
    local_time_nonexistent: 'Die nächste Startzeit existiert wegen der Zeitumstellung nicht. Ein Admin muss die Serienzeit korrigieren.',
    local_time_ambiguous: 'Die nächste Startzeit ist wegen der Zeitumstellung mehrdeutig. Ein Admin muss die Serienzeit korrigieren.',
    schedule_range: 'Für diese Konfiguration konnte kein nächster Termin ermittelt werden.',
    raid_cancelled: 'Abgesagte Raids können nicht archiviert werden.',
    raid_not_archivable: 'Dieser Raid kann nicht archiviert werden. Bitte neu laden.',
    insufficient_balance: 'Der Empfänger hat keinen positiven EP-Stand. Enhanced-Loot wurde nicht gespeichert.',
    recipient_not_in_roster: 'Der Empfänger-Charakter steht nicht mehr im Raidroster. Bitte neu laden und erneut wählen.',
    invalid_award_method: 'Bitte Main-Spec, Second-Spec, Enhanced oder BoE wählen.',
    request_conflict: 'Der Speicherschlüssel gehört zu einem anderen Eintrag. Bitte den Entwurf prüfen; es wurde nichts überschrieben.',
    request_id_required: 'Für Enhanced-Loot fehlt der Speicherschlüssel. Bitte erneut speichern; der Entwurf bleibt erhalten.',
    catalog_item_invalid: 'Boss und Item passen nicht zur gespeicherten Raidinstanz oder Raidgröße. Bitte die Zuordnung prüfen und die Beute erneut wählen.',
    catalog_invalid: 'Diese Raidinstanz und Raidgröße sind derzeit nicht verfügbar. Bitte eine angebotene Kombination wählen.',
    catalog_context_required: 'Vor neuer Loot-Erfassung müssen Raidinstanz und Raidgröße zugeordnet und gespeichert sein.',
  }
  if (messages[err.code]) return messages[err.code]
  return err.message
}

export function RaidDetailPage({ raidID, initialView }: Props) {
  return <RaidDetail key={`${raidID}:${initialView ?? ''}`} raidID={raidID} initialView={initialView} />
}

function RaidDetail({ raidID, initialView }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const characterVersion = useRef(0)
  const lootFormVersion = useRef(0)
  const lootEditorVersion = useRef(0)
  const lootItemInput = useRef<HTMLInputElement>(null)
  const attCharacterInput = useRef<HTMLInputElement>(null)
  const focusAttendanceInput = useRef(false)
  const lootRecipientInput = useRef<HTMLInputElement>(null)
  const focusLootItem = useRef(false)
  const lootRequest = useRef<{ payload: string; id: string } | null>(null)
  const catalogVersion = useRef(0)
  const associationVersion = useRef(0)
  const persistedCatalogContext = useRef('')
  const [view, setView] = useState<'roster' | 'loot' | 'planning' | 'signups' | 'recruitment'>(initialView ?? 'roster')
  const viewDefaulted = useRef(!!initialView)
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<RecordedLoot[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')
  const [epWarning, setEpWarning] = useState<{ shortfalls: EpShortfall[]; retry: () => void } | null>(null)
  const [message, setMessage] = useState('')
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [raidName, setRaidName] = useState('')
  const [seriesID, setSeriesID] = useState('')
  const [scheduled, setScheduled] = useState('')
  const [archiveResult, setArchiveResult] = useState<ArchiveResponse | null>(null)

  const [attPlayer, setAttPlayer] = useState('')
  const [attCharacter, setAttCharacter] = useState('')
  const [attRole, setAttRole] = useState<AttendanceRole>('DD')
  const [characters, setCharacters] = useState<Character[]>([])
  const [absences, setAbsences] = useState<Absence[]>([])
  const [archiveReview, setArchiveReview] = useState(false)
  const [archivePreview, setArchivePreview] = useState<ArchivePreviewRow[] | null>(null)
  const [withPoints, setWithPoints] = useState<number[]>([])
  const [characterSearch, setCharacterSearch] = useState('')
  const [quickAdd, setQuickAdd] = useState(false)
  const [quickAddName, setQuickAddName] = useState('')
  const { options: classSpecs, error: optionsError, reload: reloadOptions } = useCharacterOptions()
  const [attEditing, setAttEditing] = useState<Attendance | null>(null)
  const [attDetails, setAttDetails] = useState({ is_bench: false, late_join: false, early_leave: false })
  const [attReason, setAttReason] = useState('')
  const [attSnapshot, setAttSnapshot] = useState<{ character_name?: string; class?: string; spec?: string }>({})
  const [rosterSearch, setRosterSearch] = useState('')
  const [rosterRole, setRosterRole] = useState<AttendanceRole | ''>('')
  const [rosterClass, setRosterClass] = useState('')
  const [rosterBench, setRosterBench] = useState<'ALL' | 'RAID' | 'BENCH'>('ALL')
  const [rosterIncomplete, setRosterIncomplete] = useState(false)

  const [lootItem, setLootItem] = useState('')
  const [lootItemID, setLootItemID] = useState('')
  const [lootBoss, setLootBoss] = useState('')
  const [lootPlayer, setLootPlayer] = useState('')
  const [lootMethod, setLootMethod] = useState<AwardMethod>('MAIN')
  const [lootEditing, setLootEditing] = useState<RecordedLoot | null>(null)
  const [lootCharacter, setLootCharacter] = useState('')
  const [lootRecipientSearch, setLootRecipientSearch] = useState('')
  const [lootNotes, setLootNotes] = useState('')
  const [lootReason, setLootReason] = useState('')
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogBosses, setCatalogBosses] = useState<CatalogBoss[]>([])
  const [catalogItems, setCatalogItems] = useState<CatalogItemDrop[]>([])
  const [associationInstance, setAssociationInstance] = useState('')
  const [associationSize, setAssociationSize] = useState('')
  const [catalogBoss, setCatalogBoss] = useState('')
  const [unclaimedVersion, setUnclaimedVersion] = useState(0)
  const [lootEntryKind, setLootEntryKind] = useState<'award' | 'unclaimed'>('award')
  const [catalogItem, setCatalogItem] = useState('')
  const [catalogSearch, setCatalogSearch] = useState('')
  const [catalogDifficulty, setCatalogDifficulty] = useState('')
  const [catalogStatus, setCatalogStatus] = useState('')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  // Members come to a raid to sign up, so their first tab is "Anmeldungen" (ADR 081).
  useEffect(() => {
    if (viewDefaulted.current || !raid) return
    viewDefaulted.current = true
    if (!canManage && !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')) setView('signups')
  }, [raid, canManage])
  const isActiveRaid = !!raid && !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')
  const canWrite = canManage && !!raid && (isActiveRaid || raid.status === 'COMPLETED')
  const attNeedsReason = !!raid?.archived_at
  const lootNeedsReason = !!raid?.archived_at
  const renderedCharacterVersion = characterVersion.current
  const renderedLootVersion = lootFormVersion.current
  const renderedLootEditorVersion = lootEditorVersion.current
  const renderedCatalogVersion = catalogVersion.current
  const renderedAssociationVersion = associationVersion.current
  const catalogInstances = catalog?.instances ?? []
  const catalogSourcesAvailable = !!catalog?.sources.length
  const associated = !!raid?.catalog_instance_id && raid.raid_size != null
  const catalogContext = JSON.stringify([raid?.catalog_instance_id ?? '', raid?.raid_size ?? null])
  const sizes = catalogRaidSizes(catalog, associationInstance)
  const associationChanged = associationInstance !== (raid?.catalog_instance_id ?? '') || associationSize !== (raid?.raid_size == null ? '' : String(raid.raid_size))
  const associationValid = catalogSourcesAvailable && catalogInstances.some((instance) => instance.id === associationInstance) && sizes.includes(Number(associationSize))
  const hasAttCharacter = characters.some((c) => c.id === Number(attCharacter) && c.player_id === Number(attPlayer))
  const lootRecipient = attendance.find((a) => a.character_id === Number(lootCharacter) && a.player_id === Number(lootPlayer) && a.raid_id === raidID)
  const unchangedLootRecipient = !!lootEditing && lootEditing.awarded_to_player_id === Number(lootPlayer)
    && (lootEditing.awarded_to_character_id ? String(lootEditing.awarded_to_character_id) : '') === lootCharacter
  const hasLootRecipient = !!lootRecipient || unchangedLootRecipient
  const unchangedLootIdentity = !!lootEditing && catalogBoss === (lootEditing.catalog_boss_id ?? '') && lootItemID === lootEditing.item_id && lootItem === lootEditing.item_name && lootBoss === lootEditing.boss
  const selectedDrop = catalogItems.find((item) => dropKey(item) === catalogItem && item.boss_id === catalogBoss && item.item_id === lootItemID && item.raid_size === raid?.raid_size && item.mode === catalogDifficulty)
  const hasCatalogIdentity = (!!lootEditing && unchangedLootIdentity && unchangedLootRecipient)
    || (associated && catalogSourcesAvailable && !!selectedDrop && catalogBosses.some((boss) => boss.id === catalogBoss && boss.instance_id === raid?.catalog_instance_id))
  const catalogDifficulties = (Object.keys(catalogModeLabels) as (keyof typeof catalogModeLabels)[]).filter((mode) => catalogItems.some((item) => item.mode === mode))
  const modeCatalogItems = catalogItems.filter((item) => item.mode === catalogDifficulty)
  const awardedCount = (item: CatalogItemDrop) => loot.filter((row) => row.item_id === item.item_id).length
  const itemLabel = (item: CatalogItemDrop) => {
    const awarded = awardedCount(item)
    return `${catalogItemLabel(item, modeCatalogItems)}${awarded ? ` · bereits ${awarded}× vergeben` : ''}`
  }
  const visibleCatalogItems = modeCatalogItems.filter((item) => dropKey(item) === catalogItem || matchesCatalogItem(item, catalogSearch))

  // Only declined players count as absent for the bench button and the archive EP choice;
  // unsure players keep the normal EP rule (ADR 082).
  const absenceReasons = new Map(absences.filter(isDeclined).map((row) => [row.player_id, row.reason]))
  const tentativeReasons = new Map(absences.filter((row) => !isDeclined(row)).map((row) => [row.player_id, row.reason]))
  const declinedPlayers = [...new Set(attendance.filter((row) => absenceReasons.has(row.player_id)).map((row) => row.player_id))]

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [r, a, l, p, s, cs, ab] = await Promise.all([
        api.get<Raid>(`/api/v1/raids/${raidID}`),
        api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
        api.get<RecordedLoot[]>(`/api/v1/raids/${raidID}/loot`),
        api.get<Player[]>('/api/v1/players'),
        api.get<RaidSeries[]>('/api/v1/raid-series'),
        api.get<Character[]>('/api/v1/characters'),
        api.get<SignupOverview>(`/api/v1/raids/${raidID}/signups`).then((o) => o.absences ?? []).catch((): Absence[] => []),
      ])
      if (isActive() && version === loadVersion.current) {
        const context = JSON.stringify([r.catalog_instance_id ?? '', r.raid_size ?? null])
        if (persistedCatalogContext.current !== context) {
          persistedCatalogContext.current = context
          ++catalogVersion.current
          ++lootFormVersion.current
          ++lootEditorVersion.current
        }
        setRaid(r)
        ++associationVersion.current
        setAssociationInstance(r.catalog_instance_id ?? '')
        setAssociationSize(r.raid_size == null ? '' : String(r.raid_size))
        setAttendance(a)
        setLoot(l)
        setPlayers(p)
        setCharacters(cs)
        setAbsences(ab)
        setSeries(s)
        setRaidName(r.name)
        setSeriesID(r.series_id ? String(r.series_id) : '')
        setScheduled(localStart(r.scheduled_start))
      }
    } catch (err) {
      if (isActive() && version === loadVersion.current) {
        setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen')
      }
    }
  }, [raidID, isActive])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    let obsolete = false
    api.get<LootCatalog>('/api/v1/loot-catalog').then((value) => {
      if (!isActive() || obsolete) return
      setCatalog(value)
    }).catch(() => undefined)
    return () => { obsolete = true }
  }, [isActive])

  useEffect(() => {
    let obsolete = false
    ++catalogVersion.current
    const context = catalogContext
    setCatalogBosses([])
    setCatalogItems([])
    setCatalogBoss('')
    setCatalogItem('')
    setCatalogSearch('')
    setCatalogDifficulty('')
    setLootEditing(null)
    setLootBoss('')
    setLootItem('')
    setLootItemID('')
    setLootPlayer('')
    setLootCharacter('')
    setLootRecipientSearch('')
    setLootMethod('MAIN')
    setLootNotes('')
    setLootReason('')
    lootRequest.current = null
    focusLootItem.current = false
    if (!canWrite || !raid?.catalog_instance_id || raid.raid_size == null) return
    setCatalogStatus('Bosse der gespeicherten Raidinstanz werden geladen.')
    api.get<CatalogBoss[]>(`/api/v1/loot-catalog/bosses?instance_id=${encodeURIComponent(raid.catalog_instance_id)}`).then((rows) => {
      if (!isActive() || obsolete || context !== persistedCatalogContext.current) return
      setCatalogBosses(rows.filter((boss) => boss.instance_id === raid.catalog_instance_id))
      setCatalogStatus(rows.length ? '' : 'Keine Bosse für die gespeicherte Raidinstanz verfügbar. Historische Einträge bleiben erhalten.')
    }).catch(() => {
      if (isActive() && !obsolete && context === persistedCatalogContext.current) setCatalogStatus('Bosse konnten nicht geladen werden. Bitte später erneut versuchen; historische Einträge bleiben erhalten.')
    })
    return () => { obsolete = true }
  }, [raid?.catalog_instance_id, raid?.raid_size, catalogContext, canWrite, isActive])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`
  const raidRecipients = attendance.filter((a) => a.raid_id === raidID)
  const recipientCore = (a: Attendance) => `${a.character_name?.trim() || 'Name historisch nicht gespeichert'} · ${a.role} · ${playerName(a.player_id)}`
  const recipientLabel = (a: Attendance) => raidRecipients.filter((other) => recipientCore(other) === recipientCore(a)).length > 1 ? `${recipientCore(a)} · #${a.character_id}` : recipientCore(a)
  const selectedRecipient = raidRecipients.find((a) => String(a.character_id) === lootCharacter && String(a.player_id) === lootPlayer)
  const recipientText = selectedRecipient ? recipientLabel(selectedRecipient) : lootRecipientSearch
  const visibleRecipients = raidRecipients.filter((a) => a === selectedRecipient || recipientCore(a).toLowerCase().includes(lootRecipientSearch.trim().toLowerCase()))
  const lootBlockedHint = lootEditing || pending ? '' : !lootItem.trim() || !hasCatalogIdentity ? 'Zuerst ein Item wählen.' : !hasLootRecipient ? 'Empfänger-Charakter wählen.' : ''
  // The character ID is only shown when two characters would otherwise look identical.
  const characterCore = (character: Character) => `${character.name} · ${character.class} · ${playerName(character.player_id)}`
  const characterKey = (character: Character) => `${characterCore(character)}${character.active ? '' : ' (inaktiv)'}`
  const characterKeyCounts = new Map<string, number>()
  for (const character of characters) characterKeyCounts.set(characterKey(character), (characterKeyCounts.get(characterKey(character)) ?? 0) + 1)
  const characterLabel = (character: Character) => {
    const duplicate = (characterKeyCounts.get(characterKey(character)) ?? 0) > 1
    return `${characterCore(character)}${duplicate ? ` · #${character.id}` : ''}${character.active ? '' : ' (inaktiv)'}`
  }
  const visibleCharacters = characters.filter((character) => String(character.id) === attCharacter || characterLabel(character).toLocaleLowerCase('de').includes(characterSearch.toLocaleLowerCase('de')))
  const selectedAttendanceCharacter = characters.find((character) => character.id === Number(attCharacter) && character.player_id === Number(attPlayer))
  const rosterClasses = [...new Set(attendance.map((row) => row.class?.trim()).filter((value): value is string => !!value))].sort((a, b) => a.localeCompare(b, 'de'))
  const visibleAttendance = attendance.filter((row) => {
    if (row.id === attEditing?.id) return true
    const haystack = `${row.character_name ?? ''} ${row.class ?? ''} ${row.spec ?? ''} ${playerName(row.player_id)}`.toLocaleLowerCase('de')
    if (rosterSearch.trim() && !haystack.includes(rosterSearch.trim().toLocaleLowerCase('de'))) return false
    if (rosterRole && row.role !== rosterRole) return false
    if (rosterClass && row.class?.trim() !== rosterClass) return false
    if (rosterBench === 'RAID' && row.is_bench) return false
    if (rosterBench === 'BENCH' && !row.is_bench) return false
    return !rosterIncomplete || !row.character_name?.trim() || !row.class?.trim() || !row.spec?.trim()
  })

  const rosterFiltered = !!rosterSearch.trim() || !!rosterRole || !!rosterClass || rosterBench !== 'ALL' || rosterIncomplete

  useEffect(() => {
    setMessage('')
  }, [view])

  useEffect(() => {
    if (!focusAttendanceInput.current) return
    focusAttendanceInput.current = false
    attCharacterInput.current?.focus()
  }, [attendance])

  useEffect(() => {
    if (!message) return undefined
    const timer = setTimeout(() => setMessage(''), 8000)
    return () => clearTimeout(timer)
  }, [message])

  useEffect(() => {
    if (!focusLootItem.current || catalogItems.length === 0) return
    focusLootItem.current = false
    lootItemInput.current?.focus()
  }, [catalogItems])

  const editAttendance = (row: Attendance | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++characterVersion.current
    setAttEditing(row)
    setAttPlayer(row ? String(row.player_id) : '')
    setAttCharacter(row ? String(row.character_id) : '')
    const selected = row ? characters.find((character) => character.id === row.character_id) : undefined
    setCharacterSearch(selected ? characterLabel(selected) : '')
    setQuickAdd(false)
    setAttRole(row?.role ?? 'DD')
    setAttDetails({
      is_bench: row?.is_bench ?? false,
      late_join: row?.late_join ?? false, early_leave: row?.early_leave ?? false,
    })
    setAttReason('')
    setAttSnapshot(row ? { character_name: row.character_name || undefined, class: row.class || undefined, spec: row.spec || undefined } : {})
  }

  const benchAttendance = async (row: Attendance) => {
    if (isPending() || !isActive() || !canWrite || !isActiveRaid || row.raid_id !== raidID || row.is_bench) return
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        await api.patch<Attendance>(`/api/v1/raids/${raidID}/attendance/${row.id}`, { is_bench: true })
        if (!isActive()) return
        setMessage(`${row.character_name?.trim() || 'Teilnahme'} sitzt jetzt auf der Ersatzbank.`)
        await load()
      } catch (err) {
        const shortfalls = epShortfalls(err)
        if (shortfalls && isActive()) setEpWarning({ shortfalls, retry: () => void addAttendance(null, true) })
        else if (isActive()) setError(raidError(err, 'Teilnahme fehlgeschlagen'))
      }
    })
  }

  const removeAttendance = async (row: Attendance) => {
    if (isPending() || !isActive() || !canWrite || !isActiveRaid || row.raid_id !== raidID) return
    const name = row.character_name?.trim() || 'Diese Teilnahme'
    if (!window.confirm(`${name} wirklich aus dem Raid entfernen?`)) return
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        await api.del<void>(`/api/v1/raids/${raidID}/attendance/${row.id}`)
        if (!isActive()) return
        ++characterVersion.current
        setAttEditing(null)
        setAttPlayer('')
        setAttCharacter('')
        setCharacterSearch('')
        setAttRole('DD')
        setAttDetails({ is_bench: false, late_join: false, early_leave: false })
        setAttReason('')
        setAttSnapshot({})
        setMessage(`${name} wurde aus dem Raid entfernt.`)
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Entfernen fehlgeschlagen'))
      }
    })
  }

  const clearAttendanceCharacter = () => {
    if (isPending() || !isActive() || !canWrite || renderedCharacterVersion !== characterVersion.current) return
    ++characterVersion.current
    setAttCharacter('')
    setAttPlayer('')
    setCharacterSearch('')
    setAttSnapshot({})
  }

  const changeSnapshot = (field: SnapshotField, value: string) => {
    if (!canWrite || !isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
    setAttSnapshot((previous) => ({ ...previous, [field]: value }))
  }

  const changeLoot = (change: () => void) => {
    if (!canWrite || !isActive() || isPending() || renderedLootEditorVersion !== lootEditorVersion.current || catalogContext !== persistedCatalogContext.current) return
    ++lootFormVersion.current
    change()
  }

  const editLoot = (row: RecordedLoot | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++lootFormVersion.current
    ++lootEditorVersion.current
    setCatalogBoss(row?.catalog_boss_id ?? '')
    setCatalogItem('')
    setCatalogSearch('')
    setCatalogItems([])
    setCatalogDifficulty('')
    if (catalogBosses.length) setCatalogStatus('')
    lootRequest.current = null
    setLootEditing(row)
    setLootItem(row?.item_name ?? '')
    setLootItemID(row?.item_id ?? '')
    setLootBoss(row?.boss ?? '')
    setLootPlayer(row ? String(row.awarded_to_player_id) : '')
    setLootCharacter(row?.awarded_to_character_id ? String(row.awarded_to_character_id) : '')
    setLootRecipientSearch('')
    setLootMethod(row ? awardMethod(row) : 'MAIN')
    setLootNotes(row?.notes ?? '')
    setLootReason('')
    focusLootItem.current = false
  }

  const chooseCatalogBoss = (id: string) => changeLoot(() => {
    setCatalogSearch('')
    if (renderedCatalogVersion !== catalogVersion.current) return
    const version = ++catalogVersion.current
    const selected = catalogBosses.find((boss) => boss.id === id && boss.instance_id === raid?.catalog_instance_id)
    focusLootItem.current = !!selected
    const editorVersion = lootEditorVersion.current
    setCatalogBoss(selected ? id : lootEditing?.catalog_boss_id ?? '')
    setCatalogItem('')
    setCatalogItems([])
    setCatalogDifficulty('')
    setLootBoss(selected?.name ?? lootEditing?.boss ?? '')
    setLootItem(selected ? '' : lootEditing?.item_name ?? '')
    setLootItemID(selected ? '' : lootEditing?.item_id ?? '')
    if (!selected) { setCatalogStatus(''); return }
    setCatalogStatus('Items werden geladen.')
    const sizeFilter = raid?.raid_size == null ? '' : `&raid_size=${raid.raid_size}`
    api.get<CatalogItemDrop[]>(`/api/v1/loot-catalog/items?boss_id=${encodeURIComponent(id)}${sizeFilter}`).then((rows) => {
      if (!isActive() || version !== catalogVersion.current || editorVersion !== lootEditorVersion.current) return
      const eligible = [...new Map(rows.filter((item) => item.boss_id === id && item.raid_size === raid?.raid_size && item.mode in catalogModeLabels).map((item) => [dropKey(item), item])).values()]
      setCatalogItems(eligible)
      setCatalogDifficulty(eligible.some((item) => item.mode === 'NORMAL') ? 'NORMAL' : eligible[0]?.mode ?? '')
      setCatalogStatus(eligible.length ? '' : 'Keine Beute für diesen Boss und diese Raidgröße verfügbar.')
    }).catch(() => {
      if (isActive() && version === catalogVersion.current && editorVersion === lootEditorVersion.current) setCatalogStatus('Items konnten nicht geladen werden. Bitte später erneut versuchen; historische Einträge bleiben erhalten.')
    })
  })

  const addAttendance = async (event: FormEvent | null, confirmShortfall = false) => {
    event?.preventDefault()
    if (!canWrite || renderedCharacterVersion !== characterVersion.current
      || !hasAttCharacter) return
    if (attNeedsReason && !attReason.trim()) {
      setError('Für eine Änderung im Archiv ist eine nichtleere Begründung erforderlich.')
      return
    }
    const unchangedRefs = attEditing && attEditing.player_id === Number(attPlayer) && attEditing.character_id === Number(attCharacter)
    const snapshotChanges: Partial<Record<SnapshotField, string>> = {}
    for (const field of snapshotFields) {
      const value = attSnapshot[field]
      if (value !== undefined && (!unchangedRefs || value !== (attEditing[field] || undefined))) {
        snapshotChanges[field] = value
      }
    }
    if (Object.values(snapshotChanges).some((value) => !value.trim())) {
      setError('Klasse und tatsächlicher Spec dürfen nicht leer sein.')
      return
    }
    await submit(async () => {
      setError('')
      setMessage('')
      setEpWarning(null)
      try {
        const body = {
          player_id: Number(attPlayer),
          character_id: Number(attCharacter),
          role: attRole,
          ...attDetails,
          ...snapshotChanges,
          ...(attNeedsReason ? { reason: attReason.trim() } : {}),
          ...(confirmShortfall ? { confirm_ep_shortfall: true } : {}),
        }
        if (attEditing) {
          await api.patch<Attendance>(`/api/v1/raids/${raidID}/attendance/${attEditing.id}`, body)
        } else {
          await api.post<Attendance>(`/api/v1/raids/${raidID}/attendance`, body)
        }
        if (!isActive()) return
        ++characterVersion.current
        setAttPlayer('')
        setCharacterSearch('')
        setAttCharacter('')
        setAttEditing(null)
        setAttRole('DD')
        setAttDetails({ is_bench: false, late_join: false, early_leave: false })
        setAttReason('')
        setAttSnapshot({})
        focusAttendanceInput.current = !attEditing
        setMessage(attEditing ? 'Teilnahme-Korrektur gespeichert.' : 'Teilnahme hinzugefügt.')
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Teilnahme fehlgeschlagen'))
      }
    })
  }

  const addLoot = async (event: FormEvent | null, confirmShortfall = false) => {
    event?.preventDefault()
    if (!canWrite || renderedLootVersion !== lootFormVersion.current || catalogContext !== persistedCatalogContext.current) return
    if (lootNeedsReason && !lootReason.trim()) {
      setError('Für eine Änderung im Archiv ist eine nichtleere Begründung erforderlich.')
      return
    }
    if (!hasCatalogIdentity) {
      setError('Bitte Boss und Item passend zur gespeicherten Raidinstanz wählen. Historische Identitäten können bei unverändertem Empfänger erhalten bleiben.')
      return
    }
    if (!lootItem.trim() || !hasLootRecipient || !Object.keys(awardLabels).includes(lootMethod)) {
      setError('Bitte ein Item und einen Empfänger-Charakter aus diesem Raid wählen.')
      return
    }
    await submit(async () => {
      setError('')
      setMessage('')
      setEpWarning(null)
      try {
        const body = {
          ...(!lootEditing || lootItemID !== lootEditing.item_id ? { item_id: lootItemID } : {}),
          ...(catalogBoss && (!lootEditing || catalogBoss !== (lootEditing.catalog_boss_id ?? '')) ? { catalog_boss_id: catalogBoss } : {}),
          ...(!lootEditing || lootMethod !== awardMethod(lootEditing) ? { award_method: lootMethod } : {}),
          ...(!lootEditing || !unchangedLootRecipient ? {
            awarded_to_player_id: lootRecipient!.player_id,
            awarded_to_character_id: lootRecipient!.character_id,
          } : {}),
          notes: lootNotes,
          ...(lootNeedsReason ? { reason: lootReason.trim() } : {}),
        }
        if (lootEditing) {
          await api.patch<LootAward>(`/api/v1/raids/${raidID}/loot/${lootEditing.id}`, { ...body, ...(confirmShortfall ? { confirm_ep_shortfall: true } : {}) })
        } else {
           const payload = JSON.stringify({ flow: 'catalog-new', body })
          if (lootRequest.current?.payload !== payload) {
            lootRequest.current = { payload, id: crypto.randomUUID() }
          }
          await api.post<RecordedLoot>(`/api/v1/raids/${raidID}/loot`, { ...body, request_id: lootRequest.current.id, ...(confirmShortfall ? { confirm_ep_shortfall: true } : {}) })
        }
        if (!isActive()) return
        ++lootFormVersion.current
        ++lootEditorVersion.current
        ++catalogVersion.current
        // Neue Vergabe: Boss, Schwierigkeit und Dropliste bleiben für das nächste Item stehen.
        if (lootEditing) {
          setCatalogBoss('')
          setCatalogItems([])
          setCatalogDifficulty('')
          setLootBoss('')
        }
        setCatalogItem('')
        setCatalogSearch('')
        if (catalogBosses.length) setCatalogStatus('')
        lootRequest.current = null
        setLootItem('')
        setLootItemID('')
        setLootPlayer('')
        setLootEditing(null)
        setLootCharacter('')
        setLootRecipientSearch('')
    setLootRecipientSearch('')
        setLootMethod('MAIN')
        setLootNotes('')
        setLootReason('')
        setMessage(lootEditing ? 'Loot-Korrektur gespeichert.' : 'Loot vergeben.')
        await load()
      } catch (err) {
        const shortfalls = epShortfalls(err)
        if (shortfalls && isActive()) setEpWarning({ shortfalls, retry: () => void addLoot(null, true) })
        else if (isActive()) setError(raidError(err, 'Loot fehlgeschlagen'))
      }
    })
  }

  const action = async (name: 'cancel' | 'archive') => {
    if (!canManage || !raid || raid.archived_at || raid.status === 'CANCELLED' || (name !== 'archive' && !isActiveRaid)) return
    if (name === 'archive' && isActiveRaid && (!raid.series_id || !raid.scheduled_start)) return
    if (name === 'cancel' && !window.confirm('Raid wirklich absagen? Es werden keine EP vergeben.')) return
    if (name === 'archive' && isActiveRaid && !archiveReview) {
      await submit(async () => {
        setError('')
        try {
          const rows = await api.get<ArchivePreviewRow[]>(`/api/v1/raids/${raidID}/archive-preview`)
          if (!isActive()) return
          setArchivePreview(rows)
          setArchiveReview(true)
        } catch (err) {
          if (isActive()) setError(raidError(err, 'Die EP-Übersicht konnte nicht geladen werden.'))
        }
      })
      return
    }
    if (name === 'archive' && isActiveRaid && !window.confirm('Raid jetzt archivieren? Das beendet den Raid endgültig, vergibt EP und legt den nächsten Termin an.')) return
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        if (name === 'archive') {
          const result = await api.postEnvelope<ArchiveResponse>(`/api/v1/raids/${raidID}/archive`, archiveReview ? { without_points: declinedPlayers.filter((id) => !withPoints.includes(id)) } : undefined)
          if (!isActive()) return
          ++characterVersion.current
          ++lootFormVersion.current
          ++lootEditorVersion.current
          ++catalogVersion.current
          setArchiveResult(result)
          setRaid(result.data)
          setMessage('Raid archiviert.')
        } else {
          const result = await api.post<Raid>(`/api/v1/raids/${raidID}/cancel`)
          if (!isActive()) return
          ++characterVersion.current
          ++lootFormVersion.current
          ++lootEditorVersion.current
          ++catalogVersion.current
          setRaid(result)
          setMessage('Raid abgesagt.')
        }
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Aktion fehlgeschlagen'))
      }
    })
  }

  const saveSchedule = async () => {
    if (!canManage || !isActiveRaid || !raid || (raid.series_id && !seriesID)) return
    const changes: { name?: string; series_id?: number; scheduled_start?: string } = {}
    if (!raid.series_id && !seriesID && raidName.trim() !== raid.name) {
      if (!raidName.trim()) { setError('Bitte einen Raidnamen eintragen.'); return }
      changes.name = raidName.trim()
    }
    if (seriesID && Number(seriesID) !== raid.series_id) changes.series_id = Number(seriesID)
    if (scheduled && scheduled !== localStart(raid.scheduled_start)) {
      const date = new Date(scheduled)
      if (Number.isNaN(date.getTime())) { setError('Bitte einen gültigen geplanten Start angeben.'); return }
      changes.scheduled_start = date.toISOString()
    }
    if (Object.keys(changes).length === 0) return
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        const updated = await api.patch<Raid>(`/api/v1/raids/${raidID}`, changes)
        if (isActive()) {
          const context = JSON.stringify([updated.catalog_instance_id ?? '', updated.raid_size ?? null])
          if (persistedCatalogContext.current !== context) {
            persistedCatalogContext.current = context
            ++catalogVersion.current
            ++lootFormVersion.current
            ++lootEditorVersion.current
          }
          setRaid(updated)
          setRaidName(updated.name)
          ++associationVersion.current
          setAssociationInstance(updated.catalog_instance_id ?? '')
          setAssociationSize(updated.raid_size == null ? '' : String(updated.raid_size))
          setSeriesID(updated.series_id ? String(updated.series_id) : '')
          setScheduled(localStart(updated.scheduled_start))
          setMessage('Raidplanung gespeichert.')
        }
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Planung speichern fehlgeschlagen'))
      }
    })
  }

  const saveAssociation = async () => {
    if (!canManage || !isActiveRaid || !raid || !isActive() || renderedAssociationVersion !== associationVersion.current
      || !associationChanged || !associationValid) return
    const changes = {
      ...(associationInstance !== raid.catalog_instance_id ? { catalog_instance_id: associationInstance } : {}),
      ...(Number(associationSize) !== raid.raid_size ? { raid_size: Number(associationSize) } : {}),
    }
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        const updated = await api.patch<Raid>(`/api/v1/raids/${raidID}`, changes)
        if (!isActive()) return
        persistedCatalogContext.current = JSON.stringify([updated.catalog_instance_id ?? '', updated.raid_size ?? null])
        ++associationVersion.current
        ++catalogVersion.current
        ++lootFormVersion.current
        ++lootEditorVersion.current
        lootRequest.current = null
        setRaid(updated)
        setAssociationInstance(updated.catalog_instance_id ?? '')
        setAssociationSize(updated.raid_size == null ? '' : String(updated.raid_size))
        setMessage('Raidinstanz und Größe gespeichert.')
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Raidinstanz und Größe speichern fehlgeschlagen.'))
      }
    })
  }

	const withdrawLoot = async (confirmShortfall = false) => {
		if (!lootEditing || !canWrite || (lootNeedsReason && !lootReason.trim())) return
		if (!confirmShortfall && !window.confirm(`Loot „${lootEditing.item_name}“ wirklich zurückziehen?`)) return
		await submit(async () => {
			setError('')
			setMessage('')
			setEpWarning(null)
			try {
				await api.post<LootAward>(`/api/v1/raids/${raidID}/loot/${lootEditing.id}/withdraw`, {
					...(lootNeedsReason ? { reason: lootReason.trim() } : {}),
					...(confirmShortfall ? { confirm_ep_shortfall: true } : {}),
				})
				if (!isActive()) return
				editLoot(null)
				setMessage('Loot zurückgezogen.')
				await load()
			} catch (err) {
				const shortfalls = epShortfalls(err)
				if (shortfalls && isActive()) setEpWarning({ shortfalls, retry: () => void withdrawLoot(true) })
				else if (isActive()) setError(raidError(err, 'Loot konnte nicht zurückgezogen werden.'))
			}
		})
	}

  if (!raid) {
    return (
      <div>
        <p>
          <Link to="/raids">← Raids</Link>
        </p>
        {error ? <ErrorMessage>{error}</ErrorMessage> : <p className="muted">Lade…</p>}
      </div>
    )
  }

  return (
    <div className="raid-detail">
      <p>
        <Link to="/raids">← Raids</Link>
      </p>
      <RaidBanner catalogInstanceId={raid.catalog_instance_id}>
        <p className="eyebrow">Raidaufstellung · {raid.archived_at ? 'Historie' : 'Planung & Teilnahme'}</p>
        <h1>{raid.name}</h1>
        <p className="setup-date">{raid.scheduled_start ? new Date(raid.scheduled_start).toLocaleString('de-DE', { dateStyle: 'full', timeStyle: 'short' }) : 'Start noch nicht geplant'}</p>
        <div className="actions setup-meta">
          <span className="badge info">{raid.archived_at ? 'Archiviert' : ({ PLANNED: 'Geplant', OPEN: 'Offen', COMPLETED: 'Abgeschlossen', CANCELLED: 'Abgesagt' })[raid.status]}</span>
          {raid.raid_size != null && <span className="badge">{raid.raid_size} Spieler</span>}
          <span className="badge">Raidleitung: {playerName(raid.raid_leader_id)}</span>
          <span className="badge">{attendance.length} Teilnehmer · {loot.length} Loot-Vergaben</span>
        </div>
        {raid.notes && <p className="setup-note">{raid.notes}</p>}
        {raid.archived_at && <p className="muted">Archivierte Historie: Änderungen an Roster und Loot erfordern eine Begründung.</p>}
      </RaidBanner>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {epWarning && <EpShortfallWarning shortfalls={epWarning.shortfalls} pending={pending} onConfirm={epWarning.retry} onCancel={() => setEpWarning(null)} />}
      {message && <p className="success-message" role="status"><span>{message}</span><button type="button" aria-label="Meldung schließen" onClick={() => setMessage('')}>Schließen</button></p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}
      <nav className="workspace-tabs" aria-label="Raidbereiche">
        <button type="button" aria-pressed={view === 'signups'} aria-controls="raid-signups" onClick={() => setView('signups')}>Anmeldungen</button>
        <button type="button" aria-pressed={view === 'roster'} aria-controls="raid-roster" onClick={() => setView('roster')}>Aufstellung ({attendance.length})</button>
        <button type="button" aria-pressed={view === 'loot'} aria-controls="raid-loot" onClick={() => setView('loot')}>Loot ({loot.length})</button>
        {isActiveRaid && canManage && <button type="button" aria-pressed={view === 'recruitment'} aria-controls="raid-recruitment" onClick={() => setView('recruitment')}>Gesuchtext</button>}
        <button type="button" aria-pressed={view === 'planning'} aria-controls="raid-planning" onClick={() => setView('planning')}>Planung & Details</button>
      </nav>
      {view === 'signups' && <div id="raid-signups"><RaidSignups raid={raid} attendance={attendance} players={players} onRosterChange={load} /></div>}
      {isActiveRaid && canManage && <section id="raid-recruitment" hidden={view !== 'recruitment'} className="raid-recruitment" aria-label="Raidgesuchtext">{view === 'recruitment' && <RaidRecruitment raidID={raidID} seriesID={raid.series_id} attendance={attendance} />}</section>}
      <section id="raid-planning" hidden={view !== 'planning'} className="card raid-summary" aria-label="Raidübersicht">
      <h2>Planung & Details</h2>
      {raid.archived_at && <p className="muted">Archivierte Historie: Änderungen erfordern eine Begründung und werden protokolliert.</p>}
      {raid.previous_raid_id && <p><Link to={`/raids/${raid.previous_raid_id}`}>Vorheriger Raid</Link></p>}
      {archiveResult && <p role="status">Raid archiviert und weiterhin in der Historie sichtbar. {archiveResult.next_raid
        ? <Link to={`/raids/${archiveResult.next_raid.id}`}>Nächster Raid: {archiveResult.next_raid.name}{archiveResult.next_raid.scheduled_start ? ` (${new Date(archiveResult.next_raid.scheduled_start).toLocaleString()})` : ''}</Link>
        : 'Kein Nachfolger erstellt.'}</p>}
      {canWrite && <p className="muted">Änderungen werden mit Vorher/Nachher protokolliert. Nur im Archiv ist eine Begründung erforderlich; betroffene EP werden automatisch ausgeglichen; Tokens folgen dem Loot.</p>}

      {canManage && isActiveRaid && <section className="card">
        <h2>Raidplanung</h2>
        {!raid.series_id && <label>Raidname
          <input aria-label="Raidname" disabled={pending} value={raidName} onChange={(e) => setRaidName(e.target.value)} required />
        </label>}
        <select aria-label="Raidserie zuordnen" disabled={pending} value={seriesID} onChange={(e) => setSeriesID(e.target.value)}>
          <option value="">Serie auswählen</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name} ({s.start_time}, {s.timezone})</option>)}
        </select>
        <label>Geplanter Start ({Intl.DateTimeFormat().resolvedOptions().timeZone}, wird als UTC gespeichert)
          <input aria-label="Geplanter Start" disabled={pending} required={!raid.scheduled_start} type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
        </label>
        <button className="primary-action" disabled={pending || (!!raid.series_id && !seriesID) || (!raid.series_id && !raidName.trim()) || (Number(seriesID) === (raid.series_id ?? 0) && scheduled === localStart(raid.scheduled_start) && (!!raid.series_id || !!seriesID || raidName.trim() === raid.name))} onClick={saveSchedule}>Planung speichern</button>
        {!raid.series_id && <p className="muted">Vor dem Archivieren muss eine Serie zugeordnet sein.</p>}
        {!raid.scheduled_start && <p className="muted">Vor dem Archivieren muss ein geplanter Start eingetragen und gespeichert sein.</p>}
        <p className="muted">Nur geänderte Werte werden gespeichert. Vorhandene Werte können nicht geleert werden.</p>
      </section>}
      {canManage && isActiveRaid && <section className="subcard" aria-label="Raidinstanz und Größe">
        <h3>Raidinstanz und Größe</h3>
        <p className="muted">Instanz und Größe zuerst ausdrücklich speichern. Die Loot-Auswahl verwendet ausschließlich die gespeicherten Werte, nicht diesen Entwurf. Bestehende Werte können nicht geleert werden.</p>
        <label>Raidinstanz<select aria-label="Raidinstanz" disabled={pending || !catalogSourcesAvailable} value={associationInstance} onChange={(e) => {
          if (!isActive() || isPending() || renderedAssociationVersion !== associationVersion.current || (raid.catalog_instance_id && !e.target.value)) return
          ++associationVersion.current
          setAssociationInstance(e.target.value)
          setAssociationSize('')
        }}>
          <option value="" disabled={!!raid.catalog_instance_id}>Keine neue Zuordnung</option>
          {associationInstance && !catalogInstances.some((instance) => instance.id === associationInstance) && <option value={associationInstance}>{associationInstance} (derzeit nicht verfügbar)</option>}
          {catalogInstances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}
        </select></label>
        <label>Raidgröße<select aria-label="Raidgröße" disabled={pending || !catalogSourcesAvailable || sizes.length === 0} value={associationSize} onChange={(e) => {
          if (!isActive() || isPending() || renderedAssociationVersion !== associationVersion.current || !sizes.includes(Number(e.target.value))) return
          ++associationVersion.current
          setAssociationSize(e.target.value)
        }}>
          <option value="" disabled={raid.raid_size != null}>Größe wählen</option>
          {associationSize && !sizes.includes(Number(associationSize)) && <option value={associationSize}>{associationSize} Spieler (derzeit nicht verfügbar)</option>}
          {sizes.map((size) => <option key={size} value={size}>{size} Spieler</option>)}
        </select></label>
        <button className="primary-action" type="button" disabled={pending || !associationChanged || !associationValid} onClick={saveAssociation}>Raidinstanz und Größe speichern</button>
      </section>}
      {canManage && !raid.archived_at && raid.status !== 'CANCELLED' && (
        <section>
        <p className="muted">{isActiveRaid ? 'Archivieren beendet den Raid endgültig: +10 EP pro Teilnehmer ohne Main-Spec-Loot, ungenutzte Locks verfallen und der nächste zukünftige Serientermin wird angelegt. Spätere Korrekturen erfordern eine Begründung.' : 'Dieser historische abgeschlossene Raid wird nur archiviert. Keine erneute EP-Vergabe und kein Nachfolger.'}</p>
        <div className="actions">
          {isActiveRaid && <button disabled={pending} onClick={() => action('cancel')}>Absagen</button>}
          <button className="archive-action" disabled={pending || (isActiveRaid && (!raid.series_id || !raid.scheduled_start))} onClick={() => action('archive')}>{archiveReview ? 'Jetzt archivieren' : 'Archivieren'}</button>
          {archiveReview && <button type="button" disabled={pending} onClick={() => { setArchiveReview(false); setArchivePreview(null); setWithPoints([]) }}>Zurück</button>}
        </div>
        {archiveReview && archivePreview && <section className="subcard archive-review" aria-label="EP-Übersicht">
          <h3>EP-Übersicht vor dem Archivieren</h3>
          <p className="muted">So würden die EP verteilt. Wer ein Main-Spec-Item bekommen hat, erhält keine EP. Abgesagte Spieler erhalten standardmäßig keine EP; ankreuzen, damit sie trotzdem +10 EP bekommen.</p>
          <table className="table">
            <thead><tr><th>Spieler</th><th>Aktuell</th><th>Neu</th><th>Danach</th><th>Hinweis</th></tr></thead>
            <tbody>{archivePreview.map((row) => {
              const declined = absenceReasons.has(row.player_id)
              const skipped = declined && row.award > 0 && !withPoints.includes(row.player_id)
              const gain = skipped ? 0 : row.award
              const items = loot.filter((l) => l.awarded_to_player_id === row.player_id && l.is_main_spec).map((l) => l.item_name).join(', ')
              return <tr key={row.player_id}>
                <td>{playerName(row.player_id)}</td>
                <td>{row.balance}</td>
                <td>{gain > 0 ? `+${gain}` : '0'}</td>
                <td><strong>{row.balance + gain}</strong></td>
                <td>
                  {row.reason === 'main_spec_loot' && `Keine EP: Main-Spec-Loot${items ? ` (${items})` : ''}`}
                  {row.reason === 'already_awarded' && 'EP wurden bereits vergeben'}
                  {declined && row.award > 0 && <label className="check"><input type="checkbox" checked={!skipped} disabled={pending} onChange={(e) => setWithPoints((prev) => e.target.checked ? [...prev, row.player_id] : prev.filter((x) => x !== row.player_id))} /> Hat abgesagt{absenceReasons.get(row.player_id) ? ` (${absenceReasons.get(row.player_id)})` : ''} – trotzdem EP geben</label>}
                  {declined && row.award === 0 && <span className="badge bad"> Hat abgesagt</span>}
                </td>
              </tr>
            })}</tbody>
          </table>
        </section>}
        </section>
      )}
      </section>

      <section id="raid-roster" hidden={view !== 'roster'} className="card raid-roster" aria-label="Raidroster">
        <div className="section-heading"><h2>Raidaufstellung</h2><span className="muted">{attendance.filter((row) => !row.is_bench).length} im Raid · {attendance.filter((row) => row.is_bench).length} auf der Ersatzbank</span></div>
        <div className="roster-filters" aria-label="Aufstellung filtern">
          <label className="filter-search">Aufstellung filtern<input type="search" value={rosterSearch} onChange={(event) => setRosterSearch(event.target.value)} placeholder="Charakter oder Spieler" /></label>
          <label>Rolle<select value={rosterRole} onChange={(event) => setRosterRole(event.target.value as AttendanceRole | '')}>
            <option value="">Alle Rollen</option><option value="TANK">Tank</option><option value="HEAL">Heal</option><option value="DD">DD</option>
          </select></label>
          <label>Klasse<select value={rosterClass} onChange={(event) => setRosterClass(event.target.value)}><option value="">Alle Klassen</option>{rosterClasses.map((characterClass) => <option key={characterClass} value={characterClass}>{characterClass}</option>)}</select></label>
          <label>Platz<select value={rosterBench} onChange={(event) => setRosterBench(event.target.value as 'ALL' | 'RAID' | 'BENCH')}><option value="ALL">Raid und Ersatzbank</option><option value="RAID">Nur im Raid</option><option value="BENCH">Nur Ersatzbank</option></select></label>
          <label className="check"><input type="checkbox" checked={rosterIncomplete} onChange={(event) => setRosterIncomplete(event.target.checked)} /> Nur unvollständige</label>

        </div>
        {rosterFiltered && <div className="roster-filter-result"><span className="muted" role="status">{visibleAttendance.length} von {attendance.length} Teilnehmern angezeigt.</span><button type="button" onClick={() => { setRosterSearch(''); setRosterRole(''); setRosterClass(''); setRosterBench('ALL'); setRosterIncomplete(false) }}>Filter zurücksetzen</button></div>}
        <RaidRosterBoard onBench={canWrite && isActiveRaid ? benchAttendance : undefined} absences={absenceReasons} tentative={tentativeReasons} attendance={visibleAttendance} filtered={rosterFiltered} playerName={playerName} canWrite={canWrite} pending={pending} onEdit={editAttendance} editingID={attEditing?.id} isAddOpen={quickAdd && quickAddName !== ''}>
        {canWrite && (
          <div>
            {optionsError && <ErrorMessage>{optionsError} <button type="button" onClick={reloadOptions}>Erneut laden</button></ErrorMessage>}
            {!attEditing && !quickAdd && <button type="button" disabled={pending} onClick={() => { setQuickAddName(''); setQuickAdd(true) }}>Neuen Charakter anlegen</button>}
            {quickAdd ? <RaidQuickAdd key={quickAddName} initialQuery={quickAddName} raidID={raidID} players={players} options={classSpecs} needsReason={attNeedsReason} pending={pending} submit={submit} isActive={isActive} onCancel={() => setQuickAdd(false)} onAdded={async () => { setQuickAdd(false); await load() }} /> :
            <form className="inline-form" onSubmit={addAttendance}>
              <label className="picker-field">Charakter auswählen<span className="picker-row"><input ref={attCharacterInput} type="search" list="attendance-character-options" aria-label="Teilnahme-Charakter" autoComplete="off" autoFocus={attEditing != null} disabled={pending || characters.length === 0} value={characterSearch} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                setCharacterSearch(e.target.value)
                const selected = characters.find((c) => characterLabel(c) === e.target.value || String(c.id) === e.target.value)
                setAttCharacter(selected ? String(selected.id) : '')
                setAttPlayer(selected ? String(selected.player_id) : '')
                setAttSnapshot(selected ? { character_name: selected.name, class: selected.class, spec: selected.main_spec } : {})
              }} placeholder="Charaktername oder Spieler" required />{selectedAttendanceCharacter && <button type="button" aria-label="Ausgewählten Charakter entfernen" disabled={pending} onClick={clearAttendanceCharacter}>Entfernen</button>}</span></label>
              <datalist id="attendance-character-options">
                {visibleCharacters.map((character) => <option key={character.id} value={characterLabel(character)} />)}
              </datalist>
              {characterSearch.trim() && !selectedAttendanceCharacter && visibleCharacters.length === 0 && <p className="field-hint">Kein passender Charakter gefunden.</p>}
              <label>Gespeicherte Klasse<select aria-label="Gespeicherte Klasse" disabled={pending || hasAttCharacter} value={attSnapshot.class ?? ''} onChange={(e) => { changeSnapshot('class', e.target.value); changeSnapshot('spec', '') }}>
                <option value="">Klasse wählen…</option>
                {attSnapshot.class && !classSpecs[attSnapshot.class] && <option value={attSnapshot.class}>{attSnapshot.class} (gespeichert)</option>}
                {Object.keys(classSpecs).sort().map((value) => <option key={value}>{value}</option>)}
              </select></label>
              <SpecSelect label="Tatsächlicher Spec" disabled={pending} value={attSnapshot.spec ?? ''} specs={classSpecs[canonicalClass(attSnapshot.class ?? '')] ?? []} onChange={(value) => changeSnapshot('spec', value)} />
              {characters.find((c) => c.id === Number(attCharacter))?.second_spec && <button disabled={pending} type="button" onClick={() => changeSnapshot('spec', characters.find((c) => c.id === Number(attCharacter))!.second_spec)}>Second-Spec übernehmen</button>}
              <label>Rolle<select aria-label="Teilnahme-Rolle" disabled={pending} value={attRole} onChange={(e) => setAttRole(e.target.value as AttendanceRole)}>
                <option value="TANK">Tank</option>
                <option value="HEAL">Heal</option>
                <option value="DD">DD</option>
              </select></label>
              <fieldset className="attendance-flags"><legend>Teilnahmestatus</legend>{attendanceFlags.map(({ key, label }) => <label className="check" key={key}>
                <input type="checkbox" disabled={pending} checked={attDetails[key]} onChange={(e) => {
                  const checked = e.target.checked
                  setAttDetails((previous) => ({ ...previous, [key]: checked }))
                }} /> {label}
              </label>)}</fieldset>
              {attNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Teilnahme-Begründung" value={attReason} onChange={(e) => setAttReason(e.target.value)} required /></label>}
              {attEditing && error && <ErrorMessage>{error}</ErrorMessage>}
              {!pending && (!hasAttCharacter ? <p className="field-hint">Zuerst einen Charakter wählen.</p> : attNeedsReason && !attReason.trim() ? <p className="field-hint">Begründung eintragen.</p> : null)}
              <button className="primary-action" type="submit" disabled={pending || !hasAttCharacter || (attNeedsReason && !attReason.trim())}>{attEditing ? 'Teilnahme-Korrektur speichern' : 'Hinzufügen'}</button>
              {attEditing && <button type="button" disabled={pending} onClick={() => editAttendance(null)}>Bearbeitung abbrechen</button>}
              {attEditing && isActiveRaid && <button type="button" disabled={pending} onClick={() => void removeAttendance(attEditing)}>Aus dem Raid entfernen</button>}
            </form>}
          </div>
        )}
        </RaidRosterBoard>
        <p className="muted setup-caption">Gespeicherte Aufstellung dieses Raids. Die Ersatzbank folgt derselben EP-Regel.</p>
        {raid.status !== 'CANCELLED' && <ShadowmourneRaidCard raidID={raidID} rosterKey={attendance.map((row) => row.character_id).join(',')} />}
        {canManage && attendance.length > 0 && <AddonInviteExport raidID={raidID} rosterKey={attendance.map((row) => `${row.id}:${row.character_id}:${row.is_bench}`).join(',')} />}
        {canWrite && isActiveRaid && <AddonRosterImport raidID={raidID} onImported={load} onQuickAdd={(name) => { editAttendance(null); setQuickAddName(name); setQuickAdd(true) }} />}
      </section>

      <section id="raid-loot" hidden={view !== 'loot'} className="card raid-loot" aria-label="Raidloot">
        <h2>Loot ({loot.length})</h2>
        <p className="muted">Außerhalb der Website entschiedene Gewinner erfassen. Hier wird weder gewürfelt noch ein frei wählbarer EP-Betrag ausgegeben.</p>
        <p className="muted">Main-Spec und Enhanced zählen als Main-Spec-Loot. Second-Spec und Spieler-BoE verhindern die +10 EP beim Archivieren nicht.</p>
        <RaidLootList loot={loot} attendance={attendance} playerName={playerName} canWrite={canWrite} pending={pending} editingID={lootEditing?.id} onEdit={editLoot}>
        {canWrite && (
          <div>
            {catalogStatus && <p className="muted" role="status">{catalogStatus}</p>}
            <p className="muted">{associated ? 'Boss und Item müssen zur gespeicherten Raidinstanz und Raidgröße passen. Vorhandene historische Einträge bleiben erhalten.' : 'Vor neuer Loot-Erfassung bitte Raidinstanz und Raidgröße oben wählen und speichern. Historische Einträge können mit unveränderter Identität und unverändertem Empfänger korrigiert werden.'}</p>
            {!lootEditing && <label>Art der Erfassung<select aria-label="Loot-Art" value={lootEntryKind} onChange={(e) => setLootEntryKind(e.target.value as 'award' | 'unclaimed')}>
              <option value="award">An Spieler vergeben</option>
              <option value="unclaimed">Entzaubert / Gildenbank / Niemand wollte es</option>
            </select></label>}
            {!lootEditing && lootEntryKind === 'unclaimed' ? <UnclaimedLoot raidID={raidID} canWrite={canWrite} needsReason={lootNeedsReason} bosses={catalogBosses} version={unclaimedVersion} mode="form" onChanged={() => setUnclaimedVersion((value) => value + 1)} /> : <>
            {attendance.length === 0 && <p className="muted">Keine Empfänger verfügbar: zuerst Charaktere in den Raidroster aufnehmen.</p>}
            <form className="inline-form" onSubmit={addLoot}>
              {associated ? <>
                {lootEditing && <p>Gespeicherter Eintrag: {lootBoss || 'Kein Boss gewählt'} · {lootItem || 'Kein Drop gewählt'}{lootItemID ? ` (Item-ID ${lootItemID})` : ''}</p>}
                <label>Boss / Begegnung<select aria-label="Loot-Boss" disabled={pending || !catalogSourcesAvailable || catalogBosses.length === 0} value={catalogBoss} onChange={(e) => chooseCatalogBoss(e.target.value)}>
                  <option value="">{lootEditing ? 'Gespeicherte Loot-Identität beibehalten' : 'Boss wählen'}</option>
                  {catalogBoss && !catalogBosses.some((boss) => boss.id === catalogBoss) && <option value={catalogBoss}>Gespeicherter Boss (derzeit nicht verfügbar)</option>}
                  {catalogBosses.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
                </select></label>
                <label>Modus<select aria-label="Loot-Modus" disabled={pending || catalogDifficulties.length === 0} value={catalogDifficulty} onChange={(e) => changeLoot(() => {
                  if (renderedCatalogVersion !== catalogVersion.current) return
                  if (!catalogDifficulties.includes(e.target.value as keyof typeof catalogModeLabels)) return
                  setCatalogDifficulty(e.target.value)
                  if (selectedDrop && selectedDrop.mode !== e.target.value) { setCatalogItem(''); setCatalogSearch(''); setLootItem(''); setLootItemID('') }
                })}>
                  {catalogDifficulties.length === 0 && <option value="">Zuerst Boss wählen</option>}
                  {catalogDifficulties.map((difficulty) => <option key={difficulty} value={difficulty}>{catalogModeLabels[difficulty]}</option>)}
                </select></label>
                <label className="picker-field">Item auswählen<span className="picker-row"><input ref={lootItemInput} type="search" list="loot-item-options" aria-label="Loot-Item" autoComplete="off" disabled={pending || catalogItems.length === 0} value={catalogSearch} onChange={(e) => changeLoot(() => {
                  if (renderedCatalogVersion !== catalogVersion.current) return
                  const typed = e.target.value
                  const typedID = typed.trim()
                  const item = modeCatalogItems.find((drop) => itemLabel(drop) === typed || dropKey(drop) === typed)
                    ?? (/^\d+$/.test(typedID) ? modeCatalogItems.find((drop) => drop.item_id === typedID) : undefined)
                  setCatalogSearch(item ? itemLabel(item) : typed)
                  setCatalogItem(item ? dropKey(item) : '')
                  setLootItem(item?.name ?? '')
                  setLootItemID(item?.item_id ?? '')
                })} onKeyDown={(e) => { if (e.key === 'Enter' && selectedDrop) { e.preventDefault(); lootRecipientInput.current?.focus() } }} placeholder="Name oder Item-ID" required />{selectedDrop && <button type="button" aria-label="Ausgewähltes Item entfernen" disabled={pending} onClick={() => changeLoot(() => { setCatalogItem(''); setCatalogSearch(''); setLootItem(''); setLootItemID('') })}>Entfernen</button>}</span></label>
                <datalist id="loot-item-options">
                  {visibleCatalogItems.map((item) => <option key={dropKey(item)} value={itemLabel(item)} />)}
                </datalist>
                {catalogSearch.trim() && !selectedDrop && visibleCatalogItems.length === 0 && <p className="field-hint">Kein passendes Item für diesen Boss und Modus gefunden.</p>}
                {selectedDrop?.conditional && <p className="muted">Bedingte Beute.</p>}
              </> : <>
              {lootEditing && <p>Gespeicherte Identität: {lootBoss || 'Boss historisch nicht gespeichert'} · {lootItem} · Item-ID {lootItemID || 'historisch nicht gespeichert'}</p>}
              </>}
              <label className="picker-field">Empfänger-Charakter aus dem Raidroster<span className="picker-row"><input ref={lootRecipientInput} type="search" list="loot-recipient-options" aria-label="Loot-Empfänger" autoComplete="off" autoFocus={lootEditing != null} disabled={pending || (attendance.length === 0 && !lootEditing)} value={recipientText} placeholder={lootEditing && !lootEditing.awarded_to_character_id ? 'Historischen Empfänger unverändert lassen' : 'Name tippen oder wählen'} onChange={(e) => changeLoot(() => {
                const typed = e.target.value
                const typedID = typed.trim()
                const selected = raidRecipients.find((a) => recipientLabel(a) === typed)
                  ?? (/^\d+$/.test(typedID) ? raidRecipients.find((a) => String(a.character_id) === typedID) : undefined)
                setLootCharacter(selected ? String(selected.character_id) : '')
                setLootPlayer(selected ? String(selected.player_id) : '')
                setLootRecipientSearch(selected ? '' : typed)
              })} required={!lootEditing} />{selectedRecipient && <button type="button" aria-label="Ausgewählten Empfänger entfernen" disabled={pending} onClick={() => changeLoot(() => { setLootCharacter(''); setLootPlayer(''); setLootRecipientSearch('') })}>Entfernen</button>}</span></label>
              <datalist id="loot-recipient-options">
                {visibleRecipients.map((a) => <option key={a.id} value={recipientLabel(a)} />)}
              </datalist>
              <label>Vergabemethode<select aria-label="Loot-Vergabemethode" disabled={pending} value={lootMethod} onChange={(e) => changeLoot(() => setLootMethod(e.target.value as AwardMethod))}>
                {Object.entries(awardLabels).map(([method, label]) => <option key={method} value={method}>{label}</option>)}
              </select></label>
              {lootMethod === 'ENHANCED' && <p className="muted">Betrag wird aus dem aktuellen EP-Stand ermittelt, maximal 50 EP; nur der Gewinner zahlt. Ein positiver EP-Stand ist erforderlich. Der Server prüft und bucht atomar beim Speichern.</p>}
              {lootEditing && <p className="muted">Unveränderter Enhanced-Gewinner behält seine historischen Kosten. Ein Methoden- oder Empfängerwechsel wird vom Server samt EP, abgeleiteter Token-Sicht und Audit atomar ausgeglichen.</p>}
              <label>Notizen<input disabled={pending} placeholder="Loot-Notizen" value={lootNotes} onChange={(e) => changeLoot(() => setLootNotes(e.target.value))} /></label>
              {lootNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Loot-Begründung" value={lootReason} onChange={(e) => changeLoot(() => setLootReason(e.target.value))} required /></label>}
              {lootEditing && error && <ErrorMessage>{error}</ErrorMessage>}
                {lootBlockedHint && <p className="field-hint loot-submit-hint">{lootBlockedHint}</p>}
                <button className="primary-action" type="submit" disabled={pending || !lootItem.trim() || !hasLootRecipient || !hasCatalogIdentity || (lootNeedsReason && !lootReason.trim())}>{lootEditing ? 'Loot-Korrektur speichern' : 'Vergeben'}</button>
				{lootEditing && <button type="button" disabled={pending || (lootNeedsReason && !lootReason.trim())} onClick={() => void withdrawLoot(false)}>Loot zurückziehen</button>}
               {lootEditing && <button type="button" disabled={pending} onClick={() => editLoot(null)}>Bearbeitung abbrechen</button>}
            </form>
            </>}
          </div>
        )}
        </RaidLootList>
        <RaidKills raidID={raidID} canWrite={canWrite} needsReason={lootNeedsReason} bosses={catalogBosses} catalog={catalog} raidSize={raid.raid_size ?? null} />
        <UnclaimedLoot raidID={raidID} canWrite={canWrite} needsReason={lootNeedsReason} bosses={catalogBosses} version={unclaimedVersion} mode="list" onChanged={() => setUnclaimedVersion((value) => value + 1)} />
        {canManage && isActiveRaid && associated && <AddonLootImport raidID={raidID} onImported={async () => { await load(); setUnclaimedVersion((value) => value + 1) }} />}
      </section>
    </div>
  )
}
