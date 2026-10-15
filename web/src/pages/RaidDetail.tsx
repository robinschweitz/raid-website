import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { RaidLootList } from '../components/RaidLootList'
import { RaidBanner } from '../components/RaidBanner'
import { RaidRosterBoard } from '../components/RaidRosterBoard'
import { RaidSignups } from '../components/RaidSignups'
import { atLeast, catalogModeLabels, catalogRaidSizes, type ArchiveResponse, type Attendance, type AttendanceRole, type CatalogBoss, type CatalogItemDrop, type Character, type LootAward, type LootCatalog, type Player, type Raid, type RaidSeries } from '../types'
import { matchesCatalogItem } from '../catalog'
import { canonicalClass, useCharacterOptions } from '../characterOptions'
import { SpecSelect } from '../components/SpecSelect'
import { RaidQuickAdd } from '../components/RaidQuickAdd'
import { AddonInviteExport } from '../components/AddonInviteExport'
import { AddonLootImport } from '../components/AddonLootImport'
import { AddonRosterImport } from '../components/AddonRosterImport'

interface Props {
  raidID: number
}

type AwardMethod = 'MAIN' | 'SECOND' | 'ENHANCED' | 'BOE'
type RecordedLoot = LootAward & { award_method?: AwardMethod; enhanced_cost?: number | null; catalog_boss_id?: string | null }
const dropKey = (item: CatalogItemDrop) => JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.mode])
const awardLabels: Record<AwardMethod, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const awardMethod = (row: RecordedLoot): AwardMethod => row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
const catalogItemLabel = (item: CatalogItemDrop) => `${item.name} · Item-ID ${item.item_id} · ${item.raid_size} Spieler · ${catalogModeLabels[item.mode]}${item.conditional ? ' · Bedingte Beute' : ''}`

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

export function RaidDetailPage({ raidID }: Props) {
  return <RaidDetail key={raidID} raidID={raidID} />
}

function RaidDetail({ raidID }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const characterVersion = useRef(0)
  const lootFormVersion = useRef(0)
  const lootEditorVersion = useRef(0)
  const lootItemInput = useRef<HTMLInputElement>(null)
  const focusLootItem = useRef(false)
  const lootRequest = useRef<{ payload: string; id: string } | null>(null)
  const catalogVersion = useRef(0)
  const associationVersion = useRef(0)
  const persistedCatalogContext = useRef('')
  const [view, setView] = useState<'roster' | 'loot' | 'planning' | 'signups'>('roster')
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<RecordedLoot[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')
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
  const [lootNotes, setLootNotes] = useState('')
  const [lootReason, setLootReason] = useState('')
  const [catalog, setCatalog] = useState<LootCatalog | null>(null)
  const [catalogBosses, setCatalogBosses] = useState<CatalogBoss[]>([])
  const [catalogItems, setCatalogItems] = useState<CatalogItemDrop[]>([])
  const [associationInstance, setAssociationInstance] = useState('')
  const [associationSize, setAssociationSize] = useState('')
  const [catalogBoss, setCatalogBoss] = useState('')
  const [catalogItem, setCatalogItem] = useState('')
  const [catalogSearch, setCatalogSearch] = useState('')
  const [catalogDifficulty, setCatalogDifficulty] = useState('')
  const [catalogStatus, setCatalogStatus] = useState('')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
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
  const visibleCatalogItems = catalogItems.filter((item) => item.mode === catalogDifficulty && (dropKey(item) === catalogItem || matchesCatalogItem(item, catalogSearch)))

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [r, a, l, p, s, cs] = await Promise.all([
        api.get<Raid>(`/api/v1/raids/${raidID}`),
        api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
        api.get<RecordedLoot[]>(`/api/v1/raids/${raidID}/loot`),
        api.get<Player[]>('/api/v1/players'),
        api.get<RaidSeries[]>('/api/v1/raid-series'),
        api.get<Character[]>('/api/v1/characters'),
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
  const characterLabel = (character: Character) => `${character.name} · ${character.class} · ${playerName(character.player_id)} · #${character.id}${!character.active ? ' (inaktiv)' : ''}`
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

  const addAttendance = async (event: FormEvent) => {
    event.preventDefault()
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
      try {
        const body = {
          player_id: Number(attPlayer),
          character_id: Number(attCharacter),
          role: attRole,
          ...attDetails,
          ...snapshotChanges,
          ...(attNeedsReason ? { reason: attReason.trim() } : {}),
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
        setMessage(attEditing ? 'Teilnahme-Korrektur gespeichert.' : 'Teilnahme hinzugefügt.')
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Teilnahme fehlgeschlagen'))
      }
    })
  }

  const addLoot = async (event: FormEvent) => {
    event.preventDefault()
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
          await api.patch<LootAward>(`/api/v1/raids/${raidID}/loot/${lootEditing.id}`, body)
        } else {
           const payload = JSON.stringify({ flow: 'catalog-new', body })
          if (lootRequest.current?.payload !== payload) {
            lootRequest.current = { payload, id: crypto.randomUUID() }
          }
          await api.post<RecordedLoot>(`/api/v1/raids/${raidID}/loot`, { ...body, request_id: lootRequest.current.id })
        }
        if (!isActive()) return
        ++lootFormVersion.current
        ++lootEditorVersion.current
        ++catalogVersion.current
        setCatalogBoss('')
        setCatalogItem('')
        setCatalogSearch('')
        setCatalogItems([])
        setCatalogDifficulty('')
        if (catalogBosses.length) setCatalogStatus('')
        lootRequest.current = null
        setLootItem('')
        setLootItemID('')
        setLootBoss('')
        setLootPlayer('')
        setLootEditing(null)
        setLootCharacter('')
        setLootMethod('MAIN')
        setLootNotes('')
        setLootReason('')
        setMessage(lootEditing ? 'Loot-Korrektur gespeichert.' : 'Loot vergeben.')
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Loot fehlgeschlagen'))
      }
    })
  }

  const action = async (name: 'cancel' | 'archive') => {
    if (!canManage || !raid || raid.archived_at || raid.status === 'CANCELLED' || (name !== 'archive' && !isActiveRaid)) return
    if (name === 'archive' && isActiveRaid && (!raid.series_id || !raid.scheduled_start)) return
    await submit(async () => {
      setError('')
      setMessage('')
      try {
        if (name === 'archive') {
          const result = await api.postEnvelope<ArchiveResponse>(`/api/v1/raids/${raidID}/archive`)
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

	const withdrawLoot = async () => {
		if (!lootEditing || !canWrite || (lootNeedsReason && !lootReason.trim())) return
		await submit(async () => {
			setError('')
			setMessage('')
			try {
				await api.post<LootAward>(`/api/v1/raids/${raidID}/loot/${lootEditing.id}/withdraw`, {
					...(lootNeedsReason ? { reason: lootReason.trim() } : {}),
				})
				if (!isActive()) return
				editLoot(null)
				setMessage('Loot zurückgezogen.')
				await load()
			} catch (err) {
				if (isActive()) setError(raidError(err, 'Loot konnte nicht zurückgezogen werden.'))
			}
		})
	}

  if (!raid) {
    return (
      <div>
        <p>
          <Link to="/raids">← Raids</Link>
        </p>
        {error ? <p className="error">{error}</p> : <p className="muted">Lade…</p>}
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
      {error && <p className="error">{error}</p>}
      {message && <p className="success-message" role="status">{message}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}
      <nav className="workspace-tabs" aria-label="Raidbereiche">
        <button type="button" aria-pressed={view === 'signups'} aria-controls="raid-signups" onClick={() => setView('signups')}>Anmeldungen</button>
        <button type="button" aria-pressed={view === 'roster'} aria-controls="raid-roster" onClick={() => setView('roster')}>Aufstellung ({attendance.length})</button>
        <button type="button" aria-pressed={view === 'loot'} aria-controls="raid-loot" onClick={() => setView('loot')}>Loot ({loot.length})</button>
        <button type="button" aria-pressed={view === 'planning'} aria-controls="raid-planning" onClick={() => setView('planning')}>Planung & Details</button>
      </nav>
      {view === 'signups' && <div id="raid-signups"><RaidSignups raid={raid} attendance={attendance} players={players} onRosterChange={load} /></div>}
      <section id="raid-planning" hidden={view !== 'planning'} className="card raid-summary" aria-label="Raidübersicht">
      <h2>Übersicht</h2>
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
          <button className="archive-action" disabled={pending || (isActiveRaid && (!raid.series_id || !raid.scheduled_start))} onClick={() => action('archive')}>Archivieren</button>
        </div>
        </section>
      )}
      </section>

      <section id="raid-roster" hidden={view !== 'roster'} className="card raid-roster" aria-label="Raidroster">
        <div className="section-heading"><h2>Raidaufstellung</h2><span className="muted">{attendance.filter((row) => !row.is_bench).length} im Raid · {attendance.filter((row) => row.is_bench).length} auf der Ersatzbank</span></div>
        <div className="roster-filters" aria-label="Aufstellung filtern">
          <label>Aufstellung filtern<input type="search" value={rosterSearch} onChange={(event) => setRosterSearch(event.target.value)} placeholder="Charakter oder Spieler" /></label>
          <label>Rolle<select value={rosterRole} onChange={(event) => setRosterRole(event.target.value as AttendanceRole | '')}>
            <option value="">Alle Rollen</option><option value="TANK">Tank</option><option value="HEAL">Heal</option><option value="DD">DD</option>
          </select></label>
          <label>Klasse<select value={rosterClass} onChange={(event) => setRosterClass(event.target.value)}><option value="">Alle Klassen</option>{rosterClasses.map((characterClass) => <option key={characterClass} value={characterClass}>{characterClass}</option>)}</select></label>
          <label>Platz<select value={rosterBench} onChange={(event) => setRosterBench(event.target.value as 'ALL' | 'RAID' | 'BENCH')}><option value="ALL">Raid und Ersatzbank</option><option value="RAID">Nur im Raid</option><option value="BENCH">Nur Ersatzbank</option></select></label>
          <label className="check"><input type="checkbox" checked={rosterIncomplete} onChange={(event) => setRosterIncomplete(event.target.checked)} /> Unvollständige</label>
          {(rosterSearch || rosterRole || rosterClass || rosterBench !== 'ALL' || rosterIncomplete) && <button type="button" onClick={() => { setRosterSearch(''); setRosterRole(''); setRosterClass(''); setRosterBench('ALL'); setRosterIncomplete(false) }}>Filter zurücksetzen</button>}
        </div>
        {visibleAttendance.length !== attendance.length && <p className="muted roster-filter-result">{visibleAttendance.length} von {attendance.length} Teilnehmern angezeigt.</p>}
        <RaidRosterBoard attendance={visibleAttendance} playerName={playerName} canWrite={canWrite} pending={pending} onEdit={editAttendance} editingID={attEditing?.id} isAddOpen={quickAdd && quickAddName !== ''}>
        {canWrite && (
          <div>
            {optionsError && <p className="error">{optionsError} <button type="button" onClick={reloadOptions}>Erneut laden</button></p>}
            {!attEditing && !quickAdd && <button type="button" disabled={pending} onClick={() => { setQuickAddName(''); setQuickAdd(true) }}>Neuen Charakter anlegen</button>}
            {quickAdd ? <RaidQuickAdd key={quickAddName} initialQuery={quickAddName} raidID={raidID} players={players} options={classSpecs} needsReason={attNeedsReason} pending={pending} submit={submit} isActive={isActive} onCancel={() => setQuickAdd(false)} onAdded={async () => { setQuickAdd(false); await load() }} /> :
            <form className="inline-form" onSubmit={addAttendance}>
              <label>Charakter auswählen<input type="search" list="attendance-character-options" aria-label="Teilnahme-Charakter" autoComplete="off" autoFocus={attEditing != null} disabled={pending || characters.length === 0} value={characterSearch} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                setCharacterSearch(e.target.value)
                const selected = characters.find((c) => characterLabel(c) === e.target.value || String(c.id) === e.target.value)
                setAttCharacter(selected ? String(selected.id) : '')
                setAttPlayer(selected ? String(selected.player_id) : '')
                setAttSnapshot(selected ? { character_name: selected.name, class: selected.class, spec: selected.main_spec } : {})
              }} placeholder="Charaktername oder Spieler" required /></label>
              <datalist id="attendance-character-options">
                {visibleCharacters.map((character) => <option key={character.id} value={characterLabel(character)} />)}
              </datalist>
              {characterSearch.trim() && !selectedAttendanceCharacter && visibleCharacters.length === 0 && <p className="field-hint">Kein passender Charakter gefunden.</p>}
              {selectedAttendanceCharacter && <span className="selection-chip character-selection"><span>Ausgewählt: {selectedAttendanceCharacter.name} · {playerName(selectedAttendanceCharacter.player_id)}</span><button type="button" aria-label="Ausgewählten Charakter entfernen" disabled={pending} onClick={clearAttendanceCharacter}>Entfernen</button></span>}
              <label>Gespeicherte Klasse<select aria-label="Gespeicherte Klasse" disabled={pending || hasAttCharacter} value={attSnapshot.class ?? ''} onChange={(e) => { changeSnapshot('class', e.target.value); changeSnapshot('spec', '') }}>
                <option value="">Klasse wählen…</option>
                {attSnapshot.class && !classSpecs[attSnapshot.class] && <option value={attSnapshot.class}>{attSnapshot.class} (gespeichert)</option>}
                {Object.keys(classSpecs).sort().map((value) => <option key={value}>{value}</option>)}
              </select></label>
              <SpecSelect label="Tatsächlicher Spec" disabled={pending} value={attSnapshot.spec ?? ''} specs={classSpecs[canonicalClass(attSnapshot.class ?? '')] ?? []} onChange={(value) => changeSnapshot('spec', value)} />
              {characters.find((c) => c.id === Number(attCharacter))?.second_spec && <button disabled={pending} type="button" onClick={() => changeSnapshot('spec', characters.find((c) => c.id === Number(attCharacter))!.second_spec)}>Second-Spec übernehmen</button>}
              <select aria-label="Teilnahme-Rolle" disabled={pending} value={attRole} onChange={(e) => setAttRole(e.target.value as AttendanceRole)}>
                <option value="TANK">Tank</option>
                <option value="HEAL">Heal</option>
                <option value="DD">DD</option>
              </select>
              <fieldset className="attendance-flags"><legend>Teilnahmestatus</legend>{attendanceFlags.map(({ key, label }) => <label className="check" key={key}>
                <input type="checkbox" disabled={pending} checked={attDetails[key]} onChange={(e) => {
                  const checked = e.target.checked
                  setAttDetails((previous) => ({ ...previous, [key]: checked }))
                }} /> {label}
              </label>)}</fieldset>
              {attNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Teilnahme-Begründung" value={attReason} onChange={(e) => setAttReason(e.target.value)} required /></label>}
              {attEditing && error && <p className="error" role="alert">{error}</p>}
              <button className="primary-action" type="submit" disabled={pending || !hasAttCharacter || (attNeedsReason && !attReason.trim())}>{attEditing ? 'Teilnahme-Korrektur speichern' : 'Hinzufügen'}</button>
              {attEditing && <button type="button" disabled={pending} onClick={() => editAttendance(null)}>Bearbeitung abbrechen</button>}
            </form>}
          </div>
        )}
        </RaidRosterBoard>
        <p className="muted setup-caption">Gespeicherte Aufstellung dieses Raids. Die Ersatzbank folgt derselben EP-Regel.</p>
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
            {attendance.length === 0 && <p className="muted">Keine Empfänger verfügbar: zuerst Charaktere in den Raidroster aufnehmen.</p>}
            <form className="inline-form" onSubmit={addLoot}>
              {associated ? <>
                <p>Gespeicherter Eintrag: {lootBoss || 'Kein Boss gewählt'} · {lootItem || 'Kein Drop gewählt'}{lootItemID ? ` (Item-ID ${lootItemID})` : ''}</p>
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
                <label>Item auswählen<input ref={lootItemInput} type="search" list="loot-item-options" aria-label="Loot-Item" autoComplete="off" disabled={pending || catalogItems.length === 0} value={catalogSearch} onChange={(e) => changeLoot(() => {
                  if (renderedCatalogVersion !== catalogVersion.current) return
                  setCatalogSearch(e.target.value)
                  const item = catalogItems.find((drop) => drop.mode === catalogDifficulty && (catalogItemLabel(drop) === e.target.value || dropKey(drop) === e.target.value))
                  setCatalogItem(item ? dropKey(item) : '')
                  setLootItem(item?.name ?? '')
                  setLootItemID(item?.item_id ?? '')
                })} placeholder="Name oder Item-ID" required /></label>
                <datalist id="loot-item-options">
                  {visibleCatalogItems.map((item) => <option key={dropKey(item)} value={catalogItemLabel(item)} />)}
                </datalist>
                {catalogSearch.trim() && !selectedDrop && visibleCatalogItems.length === 0 && <p className="field-hint">Kein passendes Item für diesen Boss und Modus gefunden.</p>}
                {selectedDrop && <span className="selection-chip"><span>Ausgewählt: {selectedDrop.name} · Item-ID {selectedDrop.item_id}</span><button type="button" aria-label="Ausgewähltes Item entfernen" disabled={pending} onClick={() => changeLoot(() => { setCatalogItem(''); setCatalogSearch(''); setLootItem(''); setLootItemID('') })}>Entfernen</button></span>}
                {selectedDrop?.conditional && <p className="muted">Bedingte Beute.</p>}
              </> : <>
              {lootEditing && <p>Gespeicherte Identität: {lootBoss || 'Boss historisch nicht gespeichert'} · {lootItem} · Item-ID {lootItemID || 'historisch nicht gespeichert'}</p>}
              </>}
              <label>Empfänger-Charakter aus dem Raidroster<select autoFocus={lootEditing != null} aria-label="Loot-Empfänger" disabled={pending || (attendance.length === 0 && !lootEditing)} value={lootCharacter} onChange={(e) => changeLoot(() => {
                const selected = attendance.find((a) => a.character_id === Number(e.target.value) && a.raid_id === raidID)
                setLootCharacter(selected ? String(selected.character_id) : '')
                setLootPlayer(selected ? String(selected.player_id) : '')
              })} required={!lootEditing}>
                <option value="">{lootEditing && !lootEditing.awarded_to_character_id ? 'Historischen Empfänger unverändert lassen' : 'Charakter wählen…'}</option>
                {lootEditing?.awarded_to_character_id && !attendance.some((a) => a.character_id === lootEditing.awarded_to_character_id && a.player_id === lootEditing.awarded_to_player_id) && <option value={lootEditing.awarded_to_character_id}>Historischen Empfänger unverändert lassen</option>}
                {attendance.filter((a) => a.raid_id === raidID).map((a) => <option key={a.id} value={a.character_id}>{a.character_name?.trim() || 'Name historisch nicht gespeichert'} · {a.role} · {playerName(a.player_id)}</option>)}
              </select></label>
              <label>Vergabemethode<select aria-label="Loot-Vergabemethode" disabled={pending} value={lootMethod} onChange={(e) => changeLoot(() => setLootMethod(e.target.value as AwardMethod))}>
                {Object.entries(awardLabels).map(([method, label]) => <option key={method} value={method}>{label}</option>)}
              </select></label>
              {lootMethod === 'ENHANCED' && <p className="muted">Betrag wird aus dem aktuellen EP-Stand ermittelt, maximal 50 EP; nur der Gewinner zahlt. Ein positiver EP-Stand ist erforderlich. Der Server prüft und bucht atomar beim Speichern.</p>}
              {lootEditing && <p className="muted">Unveränderter Enhanced-Gewinner behält seine historischen Kosten. Ein Methoden- oder Empfängerwechsel wird vom Server samt EP, abgeleiteter Token-Sicht und Audit atomar ausgeglichen.</p>}
              <label>Notizen<input disabled={pending} placeholder="Loot-Notizen" value={lootNotes} onChange={(e) => changeLoot(() => setLootNotes(e.target.value))} /></label>
              {lootNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Loot-Begründung" value={lootReason} onChange={(e) => changeLoot(() => setLootReason(e.target.value))} required /></label>}
              {lootEditing && error && <p className="error" role="alert">{error}</p>}
                <button className="primary-action" type="submit" disabled={pending || !lootItem.trim() || !hasLootRecipient || !hasCatalogIdentity || (lootNeedsReason && !lootReason.trim())}>{lootEditing ? 'Loot-Korrektur speichern' : 'Vergeben'}</button>
				{lootEditing && <button type="button" disabled={pending || (lootNeedsReason && !lootReason.trim())} onClick={() => void withdrawLoot()}>Loot zurückziehen</button>}
               {lootEditing && <button type="button" disabled={pending} onClick={() => editLoot(null)}>Bearbeitung abbrechen</button>}
            </form>
          </div>
        )}
        </RaidLootList>
        {canManage && isActiveRaid && associated && <AddonLootImport raidID={raidID} onImported={load} />}
      </section>
    </div>
  )
}
