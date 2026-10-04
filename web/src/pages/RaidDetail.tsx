import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { characterClassName } from '../characterTheme'
import { atLeast, catalogRaidSizes, type ArchiveResponse, type Attendance, type AttendanceRole, type CatalogBoss, type CatalogItemDrop, type Character, type LootAward, type LootCatalog, type Player, type Raid, type RaidSeries } from '../types'

interface Props {
  raidID: number
}

type AwardMethod = 'MAIN' | 'SECOND' | 'ENHANCED' | 'BOE'
type RecordedLoot = LootAward & { award_method?: AwardMethod; enhanced_cost?: number | null; catalog_boss_id?: string | null }
const dropKey = (item: CatalogItemDrop) => JSON.stringify([item.boss_id, item.item_id, item.raid_size, item.difficulty, item.loot_mode])
const awardLabels: Record<AwardMethod, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const awardMethod = (row: RecordedLoot): AwardMethod => row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')

const attendanceFlags = [
  { key: 'is_substitute', label: 'Ersatz' },
  { key: 'is_bench', label: 'Bank' },
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
    return 'Für eine Korrektur ist eine nichtleere Begründung erforderlich.'
  }
  if (err.code === 'raid_state') return 'Der Raid erlaubt diese Änderung nicht mehr. Bitte die Seite neu laden.'
  const messages: Record<string, string> = {
    series_required: 'Zum Archivieren muss zuerst eine Raidserie zugeordnet werden.',
    scheduled_start_required: 'Zum Archivieren fehlt der geplante Start. Bitte den Start in der Raidplanung eintragen und speichern.',
    series_not_found: 'Die Raidserie existiert nicht mehr. Bitte neu laden und eine gültige Serie wählen.',
    series_not_editable: 'Bei diesem Raid kann die Serie nicht mehr geändert werden.',
    schedule_not_editable: 'Die Planung eines abgeschlossenen, abgesagten oder archivierten Raids kann nicht mehr geändert werden. Bitte neu laden.',
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
    catalog_item_invalid: 'Boss und Item passen nicht zur gespeicherten Raidinstanz oder Raidgröße. Bitte die Zuordnung prüfen und den Drop erneut aus dem Katalog wählen.',
    catalog_invalid: 'Diese Instanz und Raidgröße sind im verfügbaren Katalog nicht belegt. Bitte eine angebotene Kombination wählen.',
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
  const snapshotDirty = useRef(new Set<SnapshotField>())
  const lootFormVersion = useRef(0)
  const lootEditorVersion = useRef(0)
  const lootRequest = useRef<{ payload: string; id: string } | null>(null)
  const catalogVersion = useRef(0)
  const associationVersion = useRef(0)
  const persistedCatalogContext = useRef('')
  const summarySection = useRef<HTMLElement>(null)
  const rosterSection = useRef<HTMLElement>(null)
  const lootSection = useRef<HTMLElement>(null)
  const { player: me } = useAuth()
  const [raid, setRaid] = useState<Raid | null>(null)
  const [attendance, setAttendance] = useState<Attendance[]>([])
  const [loot, setLoot] = useState<RecordedLoot[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [error, setError] = useState('')
  const [series, setSeries] = useState<RaidSeries[]>([])
  const [seriesID, setSeriesID] = useState('')
  const [scheduled, setScheduled] = useState('')
  const [archiveResult, setArchiveResult] = useState<ArchiveResponse | null>(null)

  const [attPlayer, setAttPlayer] = useState('')
  const [attCharacter, setAttCharacter] = useState('')
  const [attRole, setAttRole] = useState<AttendanceRole>('DD')
  const [characters, setCharacters] = useState<Character[]>([])
  const [attEditing, setAttEditing] = useState<Attendance | null>(null)
  const [attDetails, setAttDetails] = useState({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
  const [attReason, setAttReason] = useState('')
  const [attSnapshot, setAttSnapshot] = useState<{ character_name?: string; class?: string; spec?: string }>({})

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
  const [catalogDifficulty, setCatalogDifficulty] = useState('')
  const [catalogStatus, setCatalogStatus] = useState('')
  const [catalogSourceStatus, setCatalogSourceStatus] = useState('Loot-Katalog wird geladen.')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  const isActiveRaid = !!raid && !raid.archived_at && (raid.status === 'PLANNED' || raid.status === 'OPEN')
  const canWrite = canManage && !!raid && !raid.archived_at && (isActiveRaid || raid.status === 'COMPLETED')
  const attNeedsReason = attEditing !== null || raid?.status === 'COMPLETED'
  const lootNeedsReason = lootEditing !== null || raid?.status === 'COMPLETED'
  const renderedCharacterVersion = characterVersion.current
  const renderedLootVersion = lootFormVersion.current
  const renderedLootEditorVersion = lootEditorVersion.current
  const renderedCatalogVersion = catalogVersion.current
  const renderedAssociationVersion = associationVersion.current
  const catalogInstances = catalog?.instances ?? []
  const catalogSourcesAvailable = !!catalog?.sources.length
  const associated = !!raid?.catalog_instance_id
  const catalogContext = JSON.stringify([raid?.catalog_instance_id ?? '', raid?.raid_size ?? null])
  const sizes = catalogRaidSizes(catalog, associationInstance)
  const associationChanged = associationInstance !== (raid?.catalog_instance_id ?? '') || associationSize !== (raid?.raid_size == null ? '' : String(raid.raid_size))
  const associationValid = catalogSourcesAvailable && catalogInstances.some((instance) => instance.id === associationInstance) && sizes.includes(Number(associationSize))
  const hasAttCharacter = characters.some((c) => c.id === Number(attCharacter) && c.player_id === Number(attPlayer))
  const lootRecipient = attendance.find((a) => a.character_id === Number(lootCharacter) && a.player_id === Number(lootPlayer) && a.raid_id === raidID)
  const unchangedLootRecipient = !!lootEditing && lootEditing.awarded_to_player_id === Number(lootPlayer)
    && (lootEditing.awarded_to_character_id ? String(lootEditing.awarded_to_character_id) : '') === lootCharacter
  const hasLootRecipient = !!lootRecipient || unchangedLootRecipient
  const unchangedLootIdentity = !!lootEditing && catalogBoss === (lootEditing.catalog_boss_id ?? '') && lootItemID === lootEditing.item_id
  const selectedDrop = catalogItems.find((item) => dropKey(item) === catalogItem && item.boss_id === catalogBoss && item.item_id === lootItemID && (raid?.raid_size == null || item.raid_size === raid.raid_size))
  const hasCatalogIdentity = !associated || (!!lootEditing && unchangedLootIdentity && unchangedLootRecipient)
    || (catalogSourcesAvailable && !!selectedDrop && catalogBosses.some((boss) => boss.id === catalogBoss && boss.instance_id === raid?.catalog_instance_id))
  const catalogDifficulties = [...new Set(catalogItems.map((item) => item.difficulty))].sort()
  const visibleCatalogItems = catalogItems.filter((item) => !catalogDifficulty || item.difficulty === catalogDifficulty)
  const lootGroups = [...new Set(loot.map((row) => row.boss))]

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [r, a, l, p, s] = await Promise.all([
        api.get<Raid>(`/api/v1/raids/${raidID}`),
        api.get<Attendance[]>(`/api/v1/raids/${raidID}/attendance`),
        api.get<RecordedLoot[]>(`/api/v1/raids/${raidID}/loot`),
        api.get<Player[]>('/api/v1/players'),
        api.get<RaidSeries[]>('/api/v1/raid-series'),
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
        setSeries(s)
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
      setCatalogSourceStatus(value.instances.length ? '' : 'Loot-Katalog ist leer. Keine Boss- oder Itemlisten verfügbar.')
    }).catch(() => {
      if (isActive() && !obsolete) setCatalogSourceStatus('Loot-Katalog derzeit nicht verfügbar. Ein Admin muss die Quelle prüfen.')
    })
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
    if (!canWrite || !raid?.catalog_instance_id) return
    setCatalogStatus('Bosse der gespeicherten Raidinstanz werden geladen.')
    api.get<CatalogBoss[]>(`/api/v1/loot-catalog/bosses?instance_id=${encodeURIComponent(raid.catalog_instance_id)}`).then((rows) => {
      if (!isActive() || obsolete || context !== persistedCatalogContext.current) return
      setCatalogBosses(rows.filter((boss) => boss.instance_id === raid.catalog_instance_id))
      setCatalogStatus(rows.length ? '' : 'Keine Bosse für die gespeicherte Instanz verfügbar. Ein Admin muss den Katalog prüfen; historische Einträge bleiben erhalten.')
    }).catch(() => {
      if (isActive() && !obsolete && context === persistedCatalogContext.current) setCatalogStatus('Bosse konnten nicht geladen werden. Ein Admin muss den Katalog prüfen; keine freie Ersatzeingabe für diesen zugeordneten Raid.')
    })
    return () => { obsolete = true }
  }, [raid?.catalog_instance_id, raid?.raid_size, catalogContext, canWrite, isActive])

  useEffect(() => {
    let obsolete = false
    const version = characterVersion.current
    if (!attPlayer) {
      setCharacters([])
      return
    }
    api
      .get<Character[]>(`/api/v1/players/${attPlayer}/characters`)
      .then((cs) => {
        if (!isActive() || obsolete || version !== characterVersion.current) return
        setCharacters(cs)
        setAttCharacter((selected) => selected || (cs.length > 0 ? String(cs[0].id) : ''))
        if (!attEditing || attEditing.player_id !== Number(attPlayer)) {
          const selected = cs[0]
          if (selected) {
            const defaults = { character_name: selected.name, class: selected.class, spec: selected.main_spec }
            setAttSnapshot((previous) => {
              if (version !== characterVersion.current) return previous
              return {
                character_name: snapshotDirty.current.has('character_name') ? previous.character_name : defaults.character_name,
                class: snapshotDirty.current.has('class') ? previous.class : defaults.class,
                spec: snapshotDirty.current.has('spec') ? previous.spec : defaults.spec,
              }
            })
          }
        }
      })
      .catch((err) => {
        if (!isActive() || obsolete || version !== characterVersion.current) return
        setCharacters([])
        setAttCharacter('')
        setError(err instanceof ApiError ? err.message : 'Charaktere laden fehlgeschlagen')
      })
    return () => { obsolete = true }
  }, [attPlayer, attEditing, isActive])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`

  const editAttendance = (row: Attendance | null) => {
    if (isPending() || !isActive() || !canWrite || (row && row.raid_id !== raidID)) return
    ++characterVersion.current
    snapshotDirty.current.clear()
    setAttEditing(row)
    setAttPlayer(row ? String(row.player_id) : '')
    setAttCharacter(row ? String(row.character_id) : '')
    setCharacters([])
    setAttRole(row?.role ?? 'DD')
    setAttDetails({
      is_substitute: row?.is_substitute ?? false, is_bench: row?.is_bench ?? false,
      late_join: row?.late_join ?? false, early_leave: row?.early_leave ?? false, notes: row?.notes ?? '',
    })
    setAttReason('')
    setAttSnapshot(row ? { character_name: row.character_name || undefined, class: row.class || undefined, spec: row.spec || undefined } : {})
  }

  const changeSnapshot = (field: SnapshotField, value: string) => {
    if (!canWrite || !isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
    snapshotDirty.current.add(field)
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
  }

  const chooseCatalogBoss = (id: string) => changeLoot(() => {
    if (renderedCatalogVersion !== catalogVersion.current) return
    const version = ++catalogVersion.current
    const selected = catalogBosses.find((boss) => boss.id === id && boss.instance_id === raid?.catalog_instance_id)
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
      setCatalogItems(rows.filter((item) => item.boss_id === id && (raid?.raid_size == null || item.raid_size === raid.raid_size)))
      setCatalogStatus(rows.length ? '' : 'Keine Items für diesen Boss im Katalog verfügbar.')
    }).catch(() => {
      if (isActive() && version === catalogVersion.current && editorVersion === lootEditorVersion.current) setCatalogStatus('Items konnten nicht geladen werden. Ein Admin muss die Dropdaten prüfen; keine freie Ersatzeingabe für diesen zugeordneten Raid.')
    })
  })

  const addAttendance = async (event: FormEvent) => {
    event.preventDefault()
    if (!canWrite || renderedCharacterVersion !== characterVersion.current
      || !hasAttCharacter) return
    if (attNeedsReason && !attReason.trim()) {
      setError('Für eine Korrektur ist eine nichtleere Begründung erforderlich.')
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
      setError('Gespeicherter Charaktername, Klasse und tatsächlicher Spec dürfen nicht leer sein.')
      return
    }
    await submit(async () => {
      setError('')
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
        snapshotDirty.current.clear()
        setAttPlayer('')
        setCharacters([])
        setAttCharacter('')
        setAttEditing(null)
        setAttRole('DD')
        setAttDetails({ is_substitute: false, is_bench: false, late_join: false, early_leave: false, notes: '' })
        setAttReason('')
        setAttSnapshot({})
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
      setError('Für eine Korrektur ist eine nichtleere Begründung erforderlich.')
      return
    }
    if (!hasCatalogIdentity) {
      setError('Bitte Boss und Drop passend zur gespeicherten Raidinstanz aus dem Katalog wählen. Historische Identitäten können bei unverändertem Empfänger erhalten bleiben.')
      return
    }
    if (!lootItem.trim() || !hasLootRecipient || !Object.keys(awardLabels).includes(lootMethod)) {
      setError('Bitte ein Item und einen Empfänger-Charakter aus diesem Raid wählen.')
      return
    }
    await submit(async () => {
      setError('')
      try {
        const body = {
          ...(!lootEditing || lootItemID !== lootEditing.item_id ? { item_id: lootItemID } : {}),
          ...(catalogBoss && (!lootEditing || catalogBoss !== (lootEditing.catalog_boss_id ?? '')) ? { catalog_boss_id: catalogBoss } : {}),
          ...(!associated && (!lootEditing || lootBoss !== lootEditing.boss) ? { boss: lootBoss } : {}),
          ...(!associated && (!lootEditing || lootItem !== lootEditing.item_name) ? { item_name: lootItem } : {}),
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
          const payload = JSON.stringify(body)
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
        } else {
          const result = await api.post<Raid>(`/api/v1/raids/${raidID}/cancel`)
          if (!isActive()) return
          ++characterVersion.current
          ++lootFormVersion.current
          ++lootEditorVersion.current
          ++catalogVersion.current
          setRaid(result)
        }
        await load()
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Aktion fehlgeschlagen'))
      }
    })
  }

  const saveSchedule = async () => {
    if (!canManage || !isActiveRaid || !raid || !scheduled || (raid.series_id && !seriesID)) return
    const changes: { series_id?: number; scheduled_start?: string } = {}
    if (seriesID && Number(seriesID) !== raid.series_id) changes.series_id = Number(seriesID)
    if (scheduled !== localStart(raid.scheduled_start)) {
      const date = new Date(scheduled)
      if (Number.isNaN(date.getTime())) { setError('Bitte einen gültigen geplanten Start angeben.'); return }
      changes.scheduled_start = date.toISOString()
    }
    if (Object.keys(changes).length === 0) return
    await submit(async () => {
      setError('')
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
          ++associationVersion.current
          setAssociationInstance(updated.catalog_instance_id ?? '')
          setAssociationSize(updated.raid_size == null ? '' : String(updated.raid_size))
          setSeriesID(updated.series_id ? String(updated.series_id) : '')
          setScheduled(localStart(updated.scheduled_start))
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
      } catch (err) {
        if (isActive()) setError(raidError(err, 'Katalogzuordnung speichern fehlgeschlagen.'))
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
      <h1>
        {raid.name} <span className="badge">{raid.archived_at ? 'ARCHIVIERT' : raid.status}</span>
      </h1>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}
      <nav className="actions raid-section-nav" aria-label="Raidbereiche">
        <button type="button" onClick={() => summarySection.current?.scrollIntoView({ behavior: 'smooth' })}>Übersicht</button>
        <button type="button" onClick={() => rosterSection.current?.scrollIntoView({ behavior: 'smooth' })}>Roster ({attendance.length})</button>
        <button type="button" onClick={() => lootSection.current?.scrollIntoView({ behavior: 'smooth' })}>Loot ({loot.length})</button>
      </nav>
      <section ref={summarySection} className="card raid-summary" aria-label="Raidübersicht">
      <h2>Übersicht</h2>
      <p>{attendance.length} gespeicherte Teilnehmer · {loot.length} aufgezeichnete Loot-Vergaben</p>
      {raid.archived_at && <p className="muted">Archivierte Historie: Roster und Loot sind schreibgeschützt.</p>}
      <p>Geplanter Start: {raid.scheduled_start ? new Date(raid.scheduled_start).toLocaleString() : 'Nicht verfügbar'}. Serie: {series.find((s) => s.id === raid.series_id)?.name ?? 'Nicht verfügbar'}.</p>
      <p>Gespeicherte Kataloginstanz: {raid.catalog_instance_id ? catalogInstances.find((instance) => instance.id === raid.catalog_instance_id)?.name ?? raid.catalog_instance_id : 'Keine Zuordnung (manuelle Loot-Erfassung)'}. Raidgröße: {raid.raid_size == null ? 'Nicht zugeordnet' : `${raid.raid_size} Spieler`}.</p>
      <section aria-label="Katalogquellen" className="subcard catalog-sources">
        <h3>Katalogquellen</h3>
        {catalogSourceStatus && <p className="muted" role="status">{catalogSourceStatus}</p>}
        {catalog && <>
          <p className="muted">Nur die importierten Dropdaten sind verfügbar. Eine vollständige Boss- oder Itemabdeckung ist nicht zugesichert; Quellenhinweise zu Teilbeständen beachten.</p>
          {!catalogSourcesAvailable && <p className="muted">Keine geprüfte Katalogquelle verfügbar. Eine neue Katalogzuordnung oder Katalogvergabe ist nicht möglich.</p>}
          <details><summary>Quellen, Lizenz und Abdeckung anzeigen</summary>
          {catalog.sources.map((entry) => <div key={`${entry.source.id}-${entry.raw_hash}`}>
            <p>{entry.source.id} · Revision {entry.source.revision} · {entry.source.license} · {entry.source.attribution}</p>
            {entry.notices.map((notice, index) => <p className="muted" key={index}>{notice}</p>)}
          </div>)}
          </details>
        </>}
      </section>
      {raid.previous_raid_id && <p><Link to={`/raids/${raid.previous_raid_id}`}>Vorheriger Raid</Link></p>}
      {archiveResult && <p role="status">Raid archiviert und weiterhin in der Historie sichtbar. {archiveResult.next_raid
        ? <Link to={`/raids/${archiveResult.next_raid.id}`}>Nächster Raid: {archiveResult.next_raid.name}{archiveResult.next_raid.scheduled_start ? ` (${new Date(archiveResult.next_raid.scheduled_start).toLocaleString()})` : ''}</Link>
        : 'Kein Nachfolger erstellt.'}</p>}
      {canWrite && <p className="muted">Änderungen bestehender Einträge erfordern eine Begründung und werden mit Vorher/Nachher protokolliert. Bei abgeschlossenen Raids gilt das auch für Ergänzungen; der Server gleicht betroffene EP und Tokens aus.</p>}

      {canManage && isActiveRaid && <section className="card">
        <h2>Raidplanung</h2>
        <select aria-label="Raidserie zuordnen" disabled={pending} value={seriesID} onChange={(e) => setSeriesID(e.target.value)}>
          <option value="">Serie auswählen</option>{series.map((s) => <option key={s.id} value={s.id}>{s.name} ({s.start_time}, {s.timezone})</option>)}
        </select>
        <label>Geplanter Start ({Intl.DateTimeFormat().resolvedOptions().timeZone}, wird als UTC gespeichert)
          <input aria-label="Geplanter Start" disabled={pending} required={!raid.scheduled_start} type="datetime-local" value={scheduled} onChange={(e) => setScheduled(e.target.value)} />
        </label>
        <button disabled={pending || !scheduled || (!!raid.series_id && !seriesID) || (Number(seriesID) === (raid.series_id ?? 0) && scheduled === localStart(raid.scheduled_start))} onClick={saveSchedule}>Planung speichern</button>
        {!raid.series_id && <p className="muted">Vor dem Archivieren muss eine Serie zugeordnet sein.</p>}
        {!raid.scheduled_start && <p className="muted">Vor dem Archivieren muss ein geplanter Start eingetragen und gespeichert sein.</p>}
        <p className="muted">Nur geänderte Werte werden gespeichert. Vorhandene Werte können nicht geleert werden.</p>
      </section>}
      {canManage && isActiveRaid && <section className="subcard" aria-label="Raid-Katalogzuordnung">
        <h3>Katalogzuordnung des Raids</h3>
        <p className="muted">Instanz und Größe zuerst ausdrücklich speichern. Loot-Auswahl verwendet ausschließlich die gespeicherte Zuordnung, nicht diesen Entwurf. Bestehende Werte können nicht geleert werden.</p>
        <label>Raidinstanz<select aria-label="Raid-Kataloginstanz" disabled={pending || !catalogSourcesAvailable} value={associationInstance} onChange={(e) => {
          if (!isActive() || isPending() || renderedAssociationVersion !== associationVersion.current || (raid.catalog_instance_id && !e.target.value)) return
          ++associationVersion.current
          setAssociationInstance(e.target.value)
          setAssociationSize('')
        }}>
          <option value="" disabled={!!raid.catalog_instance_id}>Keine neue Zuordnung</option>
          {associationInstance && !catalogInstances.some((instance) => instance.id === associationInstance) && <option value={associationInstance}>{associationInstance} (Quelle nicht verfügbar)</option>}
          {catalogInstances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}
        </select></label>
        <label>Raidgröße aus Dropdaten<select aria-label="Raid-Kataloggröße" disabled={pending || !catalogSourcesAvailable || sizes.length === 0} value={associationSize} onChange={(e) => {
          if (!isActive() || isPending() || renderedAssociationVersion !== associationVersion.current || !sizes.includes(Number(e.target.value))) return
          ++associationVersion.current
          setAssociationSize(e.target.value)
        }}>
          <option value="" disabled={raid.raid_size != null}>Größe wählen</option>
          {associationSize && !sizes.includes(Number(associationSize)) && <option value={associationSize}>{associationSize} Spieler (Quelle nicht verfügbar)</option>}
          {sizes.map((size) => <option key={size} value={size}>{size} Spieler</option>)}
        </select></label>
        <button type="button" disabled={pending || !associationChanged || !associationValid} onClick={saveAssociation}>Katalogzuordnung speichern</button>
      </section>}
      {canManage && !raid.archived_at && raid.status !== 'CANCELLED' && (
        <section>
        <p className="muted">{isActiveRaid ? 'Archivieren beendet den Raid endgültig: +10 EP pro Teilnehmer ohne Main-Spec-Loot, ungenutzte Locks verfallen und der nächste zukünftige Serientermin wird angelegt. Der Raid bleibt als schreibgeschützte Historie sichtbar.' : 'Dieser historische abgeschlossene Raid wird nur archiviert. Keine erneute EP-Vergabe und kein Nachfolger.'}</p>
        <div className="actions">
          {isActiveRaid && <button disabled={pending} onClick={() => action('cancel')}>Absagen</button>}
          <button disabled={pending || (isActiveRaid && (!raid.series_id || !raid.scheduled_start))} onClick={() => action('archive')}>Archivieren</button>
        </div>
        </section>
      )}
      </section>

      <section ref={rosterSection} className="card raid-roster" aria-label="Raidroster">
        <h2>Roster · Teilnahme ({attendance.length})</h2>
        <div className="grid roster-role-cards">
          {(['TANK', 'HEAL', 'DD'] as const).map((role) => <div className="subcard" key={role}>
            <h3>{role === 'TANK' ? 'Tank' : role === 'HEAL' ? 'Heilung' : 'Schaden'}</h3>
            <p>{attendance.filter((a) => a.role === role).length} Teilnehmer</p>
          </div>)}
        </div>
        <p className="muted">Gespeicherte Namen, Klasse, Rolle und tatsächlich gespielter Spec dieses Raids. Bank und Ersatz folgen derselben EP-Regel.</p>
        <div className="table-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>Spieler</th>
              <th>Charakter</th>
              <th>Tatsächliche Rolle</th>
              <th>Klasse</th>
              <th>Tatsächlicher Spec</th>
              {canWrite && <th></th>}
            </tr>
          </thead>
          <tbody>
            {attendance.map((a) => (
              <tr key={a.id}>
                <td>{playerName(a.player_id)}</td>
                <td><span className={`character-name ${characterClassName(a.class ?? '')}`}>{a.character_name?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</span>
                  {attendanceFlags.filter(({ key }) => a[key]).map(({ key, label }) => <span className="badge" key={key}>{label}</span>)}</td>
                <td>{a.role}</td>
                <td>{a.class?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</td>
                <td>{a.spec?.trim() || 'Nicht verfügbar (historisch nicht gespeichert)'}</td>
                {canWrite && <td>
                  {isActiveRaid && (!a.character_name?.trim() || !a.class?.trim() || !a.spec?.trim()) && <p className="error">Teilnahme #{a.id}: {[
                    !a.character_name?.trim() && 'Charaktername', !a.class?.trim() && 'Klasse', !a.spec?.trim() && 'tatsächlicher Spec',
                  ].filter(Boolean).join(', ')} fehlt. Vor dem Archivieren korrigieren.</p>}
                  <button disabled={pending} onClick={() => editAttendance(a)}>Teilnahme korrigieren</button>
                </td>}
              </tr>
            ))}
            {attendance.length === 0 && (
              <tr>
                <td colSpan={canWrite ? 6 : 5} className="muted">
                  Keine Einträge.
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
        {canWrite && (
          <div className="subcard">
            <h3>{attEditing ? `Teilnahme #${attEditing.id} korrigieren` : raid.status === 'COMPLETED' ? 'Teilnahme nachtragen' : 'Teilnahme hinzufügen'}</h3>
            {attEditing && <p className="muted">Vorher: {playerName(attEditing.player_id)}, Charakter {attEditing.character_name || 'Nicht verfügbar'}, Klasse {attEditing.class || 'Nicht verfügbar'}, tatsächlicher Spec {attEditing.spec || 'Nicht verfügbar'}, Rolle {attEditing.role}. {attendanceFlags.map(({ key, label }) => `${label}: ${attEditing[key] ? 'ja' : 'nein'}`).join(', ')}. Notizen: {attEditing.notes || 'keine'}</p>}
            <form className="inline-form" onSubmit={addAttendance}>
              <select aria-label="Teilnahme-Spieler" disabled={pending} value={attPlayer} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                snapshotDirty.current.clear()
                setAttPlayer(e.target.value)
                setCharacters([])
                setAttCharacter('')
                setAttSnapshot({})
              }} required>
                <option value="">Spieler…</option>
                {players.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <select aria-label="Teilnahme-Charakter" disabled={pending || characters.length === 0} value={attCharacter} onChange={(e) => {
                if (!isActive() || isPending() || renderedCharacterVersion !== characterVersion.current) return
                ++characterVersion.current
                snapshotDirty.current.clear()
                setAttCharacter(e.target.value)
                const selected = characters.find((c) => c.id === Number(e.target.value))
                setAttSnapshot(selected ? { character_name: selected.name, class: selected.class, spec: selected.main_spec } : {})
              }} required>
                <option value="">Charakter…</option>
                {characters.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
              <label>Gespeicherter Charaktername<input disabled={pending} placeholder="Gespeicherter Charaktername" value={attSnapshot.character_name ?? ''} onChange={(e) => changeSnapshot('character_name', e.target.value)} /></label>
              <label>Gespeicherte Klasse<input disabled={pending} placeholder="Gespeicherte Klasse" value={attSnapshot.class ?? ''} onChange={(e) => changeSnapshot('class', e.target.value)} /></label>
              <label>Tatsächlicher Spec<input disabled={pending} placeholder="Tatsächlicher Spec" value={attSnapshot.spec ?? ''} onChange={(e) => changeSnapshot('spec', e.target.value)} /></label>
              {characters.find((c) => c.id === Number(attCharacter))?.second_spec && <button disabled={pending} type="button" onClick={() => changeSnapshot('spec', characters.find((c) => c.id === Number(attCharacter))!.second_spec)}>Second-Spec übernehmen</button>}
              <select aria-label="Teilnahme-Rolle" disabled={pending} value={attRole} onChange={(e) => setAttRole(e.target.value as AttendanceRole)}>
                <option value="TANK">Tank</option>
                <option value="HEAL">Heal</option>
                <option value="DD">DD</option>
              </select>
              {attendanceFlags.map(({ key, label }) => <label className="check" key={key}>
                <input type="checkbox" disabled={pending} checked={attDetails[key]} onChange={(e) => {
                  const checked = e.target.checked
                  setAttDetails((previous) => ({ ...previous, [key]: checked }))
                }} /> {label}
              </label>)}
              <label>Notizen<input disabled={pending} placeholder="Teilnahme-Notizen" value={attDetails.notes} onChange={(e) => {
                const notes = e.target.value
                setAttDetails((previous) => ({ ...previous, notes }))
              }} /></label>
              {attNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Teilnahme-Begründung" value={attReason} onChange={(e) => setAttReason(e.target.value)} required /></label>}
              <button type="submit" disabled={pending || !hasAttCharacter || (attNeedsReason && !attReason.trim())}>{attEditing ? 'Teilnahme-Korrektur speichern' : 'Hinzufügen'}</button>
              {attEditing && <button type="button" disabled={pending} onClick={() => editAttendance(null)}>Bearbeitung abbrechen</button>}
            </form>
          </div>
        )}
      </section>

      <section ref={lootSection} className="card raid-loot" aria-label="Raidloot">
        <h2>Loot ({loot.length})</h2>
        <p className="muted">Außerhalb der Website entschiedene Gewinner erfassen. Hier wird weder gewürfelt noch ein frei wählbarer EP-Betrag ausgegeben.</p>
        <p className="muted">Main-Spec und Enhanced zählen als Main-Spec-Loot. Second-Spec und Spieler-BoE verhindern die +10 EP beim Archivieren nicht.</p>
        {loot.length === 0 && <p className="muted">Kein Loot.</p>}
        {lootGroups.map((boss) => <div key={boss} className="subcard loot-boss-group">
        <h3>{boss || 'Boss nicht gespeichert (historisch)'}</h3>
        <div className="table-scroll">
        <table className="table">
          <thead>
            <tr>
              <th>Item</th>
              <th>Boss</th>
              <th>Empfänger</th>
              <th>Vergabemethode</th>
              <th>EP-Kosten</th>
              <th>Notizen</th>
              {canWrite && <th></th>}
            </tr>
          </thead>
          <tbody>
            {loot.filter((l) => l.boss === boss).map((l) => (
              <tr key={l.id}>
                <td>{l.item_name}</td>
                <td>{l.boss || '—'}</td>
                <td>{(() => {
                  const saved = attendance.find((a) => a.character_id === l.awarded_to_character_id && a.player_id === l.awarded_to_player_id)
                  return <><span className={`character-name ${characterClassName(saved?.class ?? '')}`}>{saved?.character_name?.trim() || 'Charaktername historisch nicht gespeichert'}</span><br />{playerName(l.awarded_to_player_id)}</>
                })()}</td>
                <td>{awardLabels[awardMethod(l)]}{l.item_lock_id && <span className="badge">Lock</span>}</td>
                <td>{awardMethod(l) === 'ENHANCED' ? l.enhanced_cost == null ? 'Historisch nicht gespeichert' : `${l.enhanced_cost} EP` : '0 EP'}</td>
                <td>{l.notes || '—'}</td>
                {canWrite && <td><button disabled={pending} onClick={() => editLoot(l)}>Loot korrigieren</button></td>}
              </tr>
            ))}
          </tbody>
        </table>
        </div>
        </div>)}
        {canWrite && (
          <div className="subcard">
            <h3>{lootEditing ? `Loot #${lootEditing.id} korrigieren` : raid.status === 'COMPLETED' ? 'Loot nachtragen' : 'Loot erfassen'}</h3>
            {catalogStatus && <p className="muted" role="status">{catalogStatus}</p>}
            <p className="muted">{associated ? 'Boss und Drop müssen zur gespeicherten Raidinstanz und Raidgröße passen. Fehlende Katalogdaten erlauben keine freie Ersatzeingabe; vorhandene historische Einträge bleiben erhalten.' : 'Ohne gespeicherte Katalogzuordnung können Boss und gedropptes Item ausdrücklich manuell eingetragen werden. Vorhandene historische Namen bleiben erhalten.'}</p>
            {attendance.length === 0 && <p className="muted">Keine Empfänger verfügbar: zuerst Charaktere in den Raidroster aufnehmen.</p>}
            {lootEditing && <p className="muted">Vorher: {lootEditing.item_name} (Item-ID: {lootEditing.item_id || 'keine'}), Boss: {lootEditing.boss || 'keiner'}, Empfänger: {playerName(lootEditing.awarded_to_player_id)}, Charakter: {lootEditing.awarded_to_character_id ? `#${lootEditing.awarded_to_character_id}` : 'keiner'}, {awardLabels[awardMethod(lootEditing)]}. Notizen: {lootEditing.notes || 'keine'}</p>}
            <form className="inline-form" onSubmit={addLoot}>
              {associated ? <>
                <p>Gespeicherter Eintrag: {lootBoss || 'Kein Boss gewählt'} · {lootItem || 'Kein Drop gewählt'}{lootItemID ? ` (Item-ID ${lootItemID})` : ''}</p>
                <label>Katalogboss<select aria-label="Loot-Katalogboss" disabled={pending || !catalogSourcesAvailable || catalogBosses.length === 0} value={catalogBoss} onChange={(e) => chooseCatalogBoss(e.target.value)}>
                  <option value="">{lootEditing ? 'Gespeicherte Loot-Identität beibehalten' : 'Boss wählen'}</option>
                  {catalogBoss && !catalogBosses.some((boss) => boss.id === catalogBoss) && <option value={catalogBoss}>Gespeicherter Boss (Quelle nicht verfügbar)</option>}
                  {catalogBosses.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
                </select></label>
                <label>Quelldifficulty<select aria-label="Loot-Katalogdifficulty" disabled={pending || catalogDifficulties.length === 0} value={catalogDifficulty} onChange={(e) => changeLoot(() => {
                  if (renderedCatalogVersion !== catalogVersion.current) return
                  setCatalogDifficulty(e.target.value)
                  if (selectedDrop && e.target.value && selectedDrop.difficulty !== e.target.value) { setCatalogItem(''); setLootItem(''); setLootItemID('') }
                })}>
                  <option value="">Alle Quelldifficulties</option>
                  {catalogDifficulties.map((difficulty) => <option key={difficulty} value={difficulty}>{difficulty}</option>)}
                </select></label>
                <label>Gedropptes Katalogitem<select aria-label="Loot-Katalogitem" disabled={pending || catalogItems.length === 0} value={catalogItem} onChange={(e) => changeLoot(() => {
                  if (renderedCatalogVersion !== catalogVersion.current) return
                  const item = visibleCatalogItems.find((drop) => dropKey(drop) === e.target.value)
                  setCatalogItem(item ? dropKey(item) : '')
                  setLootItem(item?.name ?? '')
                  setLootItemID(item?.item_id ?? '')
                })}>
                  <option value="">Item und Variante wählen</option>
                  {visibleCatalogItems.map((item) => <option key={dropKey(item)} value={dropKey(item)}>{item.name} · {item.raid_size} Spieler · Difficulty {item.difficulty} · Lootmodus {item.loot_mode}</option>)}
                </select></label>
              </> : <>
              <label>Boss<input disabled={pending} placeholder="Boss" value={lootBoss} onChange={(e) => changeLoot(() => { setCatalogBoss(''); setCatalogItem(''); setCatalogItems([]); ++catalogVersion.current; if (catalogInstances.length) setCatalogStatus(''); setLootBoss(e.target.value) })} /></label>
              <label>Gedropptes Item<input disabled={pending} placeholder="Item" value={lootItem} onChange={(e) => changeLoot(() => { if (catalogItem) setLootItemID(''); setCatalogItem(''); setLootItem(e.target.value) })} required /></label>
              <label>Item-ID (optional)<input disabled={pending} placeholder="Item-ID" value={lootItemID} onChange={(e) => changeLoot(() => { setCatalogItem(''); setLootItemID(e.target.value) })} /></label>
              </>}
              <label>Empfänger-Charakter aus dem Raidroster<select aria-label="Loot-Empfänger" disabled={pending || (attendance.length === 0 && !lootEditing)} value={lootCharacter} onChange={(e) => changeLoot(() => {
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
              {lootEditing && <p className="muted">Unveränderter Enhanced-Gewinner behält seine historischen Kosten. Ein Methoden- oder Empfängerwechsel wird vom Server samt EP, Tokens und Audit atomar ausgeglichen.</p>}
              <label>Notizen<input disabled={pending} placeholder="Loot-Notizen" value={lootNotes} onChange={(e) => changeLoot(() => setLootNotes(e.target.value))} /></label>
              {lootNeedsReason && <label>Begründung (Pflicht)<input disabled={pending} placeholder="Loot-Begründung" value={lootReason} onChange={(e) => changeLoot(() => setLootReason(e.target.value))} required /></label>}
              <button type="submit" disabled={pending || !lootItem.trim() || !hasLootRecipient || !hasCatalogIdentity || (lootNeedsReason && !lootReason.trim())}>{lootEditing ? 'Loot-Korrektur speichern' : 'Vergeben'}</button>
              {lootEditing && <button type="button" disabled={pending} onClick={() => editLoot(null)}>Bearbeitung abbrechen</button>}
            </form>
          </div>
        )}
      </section>
    </div>
  )
}
