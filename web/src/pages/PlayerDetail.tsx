import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useSubmission } from '../useSubmission'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import QRCode from 'qrcode'
import {
  atLeast,
  type Character,
  type ExternalCharacter,
  type ItemLock,
  type Player,
  type PointsResponse,
  type GuildRank,
  type LootAward,
  type Role,
} from '../types'
import { EnhancePointsCard } from '../components/EnhancePointsCard'
import { characterClassName } from '../characterTheme'
import { canonicalClass, useCharacterOptions } from '../characterOptions'
import { SpecSelect } from '../components/SpecSelect'
import { browserSupportsWebAuthn, createWebAuthnCredential, type WebAuthnBeginResponse } from '../webauthn'

const CLASSES = [
  'Todesritter',
  'Druide',
  'Jäger',
  'Magier',
  'Paladin',
  'Priester',
  'Schamane',
  'Schurke',
  'Hexer',
  'Krieger',
]

const ROLES: Role[] = ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']
const LOOT_METHOD_LABELS = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }

interface MFAStatus {
  totp_enabled: boolean
  security_keys: { id: number; name: string; created_at: string; last_used_at?: string }[]
}

interface Props {
  playerID: number
}

export function PlayerDetailPage({ playerID }: Props) {
  return <PlayerDetail key={playerID} playerID={playerID} />
}

function PlayerDetail({ playerID }: Props) {
  const { pending, submit, isActive, isPending } = useSubmission()
  const loadVersion = useRef(0)
  const { player: me } = useAuth()
  const [player, setPlayer] = useState<Player | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [points, setPoints] = useState<PointsResponse | null>(null)
  const [loot, setLoot] = useState<LootAward[]>([])
  const [locks, setLocks] = useState<ItemLock[]>([])
  const [ranks, setRanks] = useState<GuildRank[]>([])
  const [rankError, setRankError] = useState('')
  const [rankLoading, setRankLoading] = useState(true)
  const [error, setError] = useState('')
  const [view, setView] = useState<'points' | 'loot' | 'characters' | 'locks' | 'account'>('points')
  const [lootFilter, setLootFilter] = useState<'all' | 'tokens'>('all')

  const [charQuery, setCharQuery] = useState('')
  const [charResults, setCharResults] = useState<ExternalCharacter[]>([])
  const [charSelected, setCharSelected] = useState<ExternalCharacter | null>(null)
  const [charSearching, setCharSearching] = useState(false)
  const { options: classSpecs, error: optionsError, reload: reloadOptions } = useCharacterOptions()
  const [charIsMain, setCharIsMain] = useState(false)
  const [editingCharacter, setEditingCharacter] = useState<Character | null>(null)
  const [characterDraft, setCharacterDraft] = useState({ name: '', class: '', main_spec: '', second_spec: '', is_main: false, active: true })
  const characterEditVersion = useRef(0)
  const renderedCharacterEditVersion = characterEditVersion.current
  const [transferCharacter, setTransferCharacter] = useState<Character | null>(null)
  const [transferPlayerID, setTransferPlayerID] = useState('')
  const [transferStatus, setTransferStatus] = useState('')

  const [lockRaid, setLockRaid] = useState('')
  const [lockItem, setLockItem] = useState('')
  const [lockItemID, setLockItemID] = useState('')
  const [lockBoss, setLockBoss] = useState('')

  const [claimQuery, setClaimQuery] = useState('')
  const [claimResults, setClaimResults] = useState<ExternalCharacter[]>([])
  const [claimSelected, setClaimSelected] = useState<ExternalCharacter | null>(null)
  const [claimSearching, setClaimSearching] = useState(false)
  const [claimCode, setClaimCode] = useState('')
  const [claimStatus, setClaimStatus] = useState('')

  const [accUser, setAccUser] = useState('')
  const [accPass, setAccPass] = useState('')
  const [accRole, setAccRole] = useState<Role>('MEMBER')
  const [mergePlayers, setMergePlayers] = useState<Player[]>([])
  const [mergeSourceID, setMergeSourceID] = useState('')
  const [mergeReason, setMergeReason] = useState('')
  const [mergeCharacterTarget, setMergeCharacterTarget] = useState('')
  const [mergeCharacterSource, setMergeCharacterSource] = useState('')
  const [mergeCharacterReason, setMergeCharacterReason] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordStatus, setPasswordStatus] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [totpURI, setTotpURI] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [securityKeyStatus, setSecurityKeyStatus] = useState('')
  const [mfaPassword, setMfaPassword] = useState('')
  const [mfaPasswordPrompt, setMfaPasswordPrompt] = useState<'totp_setup' | 'totp_disable' | 'security_key' | `security_key_delete:${number}` | ''>('')
  const [mfaStatus, setMfaStatus] = useState<MFAStatus>({ totp_enabled: false, security_keys: [] })
  const [qrDataURL, setQrDataURL] = useState('')
  const [copyStatus, setCopyStatus] = useState('')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false
  const isOwnProfile = me ? me.id === playerID : false
  const isOwnMember = me ? isOwnProfile && atLeast(me.role, 'MEMBER') : false

  const load = useCallback(async () => {
    if (!isActive()) return
    const version = ++loadVersion.current
    setError('')
    try {
      const [p, cs, pts, playerLoot, lk] = await Promise.all([
        api.get<Player>(`/api/v1/players/${playerID}`),
        api.get<Character[]>(`/api/v1/players/${playerID}/characters`),
        api.get<PointsResponse>(`/api/v1/players/${playerID}/points`),
        api.get<LootAward[]>(`/api/v1/players/${playerID}/loot`),
        api.get<ItemLock[]>(`/api/v1/players/${playerID}/locks`),
      ])
      if (isActive() && version === loadVersion.current) {
        setPlayer(p)
        setCharacters(cs)
        setPoints(pts)
        setLoot(playerLoot)
        setLocks(lk)
        setAccUser(p.username ?? '')
        if (p.role) setAccRole(p.role)
      }
    } catch (err) {
      if (isActive() && version === loadVersion.current) {
        setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen')
      }
    }
  }, [playerID, isActive])

  useEffect(() => { void load() }, [load])

  const loadMFAStatus = useCallback(async () => {
    if (!isOwnProfile) return
    try {
      const status = await api.get<MFAStatus>('/api/v1/auth/mfa')
      if (isActive()) setMfaStatus({ totp_enabled: status.totp_enabled, security_keys: status.security_keys ?? [] })
    } catch {
      if (isActive()) setError('MFA-Status konnte nicht geladen werden.')
    }
  }, [isOwnProfile, isActive])

  useEffect(() => { void loadMFAStatus() }, [loadMFAStatus])

  useEffect(() => {
    if (!isAdmin) return
    let active = true
    api.get<Player[]>('/api/v1/players').then((rows) => {
      if (active) setMergePlayers(rows.filter((row) => row.active && row.id !== playerID))
    }).catch(() => {
      if (active) setError('Spielerliste für Kontozusammenführung konnte nicht geladen werden.')
    })
    return () => { active = false }
  }, [isAdmin, playerID])

  useEffect(() => {
    let active = true
    if (!totpURI) { setQrDataURL(''); return () => { active = false } }
    QRCode.toDataURL(totpURI, { margin: 1, width: 180 }).then((url) => { if (active) setQrDataURL(url) }).catch(() => { if (active) setQrDataURL('') })
    return () => { active = false }
  }, [totpURI])

  useEffect(() => {
    let active = true
    api.get<GuildRank[]>('/api/v1/guild-ranks').then((rows) => { if (active) setRanks(rows) })
      .catch(() => { if (active) setRankError('Gildenrang konnte nicht geladen werden.') })
      .finally(() => { if (active) setRankLoading(false) })
    return () => { active = false }
  }, [])

  const ownedCharacters = characters.filter((c) => c.player_id === playerID)
    .sort((a, b) => Number(b.is_main) - Number(a.is_main) || a.name.localeCompare(b.name, 'de') || a.id - b.id)
  const preferredCharacter = ownedCharacters[0]
  const visibleLoot = lootFilter === 'tokens' ? loot.filter((row) => row.token_award) : loot

  const createCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !charSelected) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Character>(`/api/v1/players/${playerID}/characters`, {
          profile_id: charSelected.profile_id,
          is_main: charIsMain,
        })
        if (!isActive()) return
        setCharQuery('')
        setCharResults([])
        setCharSelected(null)
        setCharIsMain(false)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter anlegen fehlgeschlagen')
      }
    })
  }

  const searchNewCharacters = async () => {
    if (!isAdmin || charQuery.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setCharSelected(null)
      setCharSearching(true)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(charQuery.trim())}`)
        if (isActive()) setCharResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können angelegt werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen')
      } finally {
        if (isActive()) setCharSearching(false)
      }
    })
  }

  const toggleMain = async (c: Character) => {
    if (!isAdmin || c.player_id !== playerID) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${c.id}`, { is_main: !c.is_main })
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Aktualisieren fehlgeschlagen')
      }
    })
  }

  const editCharacter = (c: Character | null) => {
    if (!isAdmin || !isActive() || isPending() || (c && c.player_id !== playerID)) return
    ++characterEditVersion.current
    setTransferCharacter(null)
    setEditingCharacter(c)
    if (c) setCharacterDraft({ name: c.name, class: c.class, main_spec: c.main_spec, second_spec: c.second_spec, is_main: c.is_main, active: c.active })
  }

  const saveCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !editingCharacter || !characterDraft.name.trim() || renderedCharacterEditVersion !== characterEditVersion.current) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<Character>(`/api/v1/characters/${editingCharacter.id}`, { ...characterDraft, name: characterDraft.name.trim() })
        if (!isActive()) return
        ++characterEditVersion.current
        setEditingCharacter(null)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter speichern fehlgeschlagen')
      }
    })
  }

  const changeCharacter = (patch: Partial<typeof characterDraft>) => {
    if (!isActive() || isPending() || renderedCharacterEditVersion !== characterEditVersion.current) return
    setCharacterDraft((previous) => ({ ...previous, ...patch }))
  }

  const startCharacterTransfer = (c: Character) => {
    if (!isAdmin || !isActive() || isPending() || c.player_id !== playerID) return
    setEditingCharacter(null)
    setTransferStatus('')
    setTransferPlayerID('')
    setTransferCharacter(c)
  }

  const transferCharacterOwner = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !transferCharacter || !transferPlayerID || transferCharacter.player_id !== playerID) return
    const target = mergePlayers.find((candidate) => candidate.id === Number(transferPlayerID))
    if (!target || !window.confirm(`${transferCharacter.name} wirklich zu ${target.name} verschieben? Raid-Aufstellungen werden zugeordnet; Loot und EP bleiben unverändert.`)) return
    await submit(async () => {
      setError('')
      setTransferStatus('')
      try {
        await api.post<Character>(`/api/v1/characters/${transferCharacter.id}/transfer`, { target_player_id: target.id })
        if (!isActive()) return
        setTransferCharacter(null)
        setTransferPlayerID('')
        setTransferStatus(`${transferCharacter.name} gehört jetzt zu ${target.name}.`)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charakter verschieben fehlgeschlagen')
      }
    })
  }

  const createLock = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnMember) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ItemLock>(`/api/v1/players/${playerID}/locks`, {
          raid_id: Number(lockRaid),
          item_id: lockItemID,
          item_name: lockItem,
          boss: lockBoss,
        })
        if (!isActive()) return
        setLockRaid('')
        setLockItem('')
        setLockItemID('')
        setLockBoss('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Lock anlegen fehlgeschlagen')
      }
    })
  }

  const cancelLock = async (lock: ItemLock) => {
    if (!isOwnMember || lock.player_id !== playerID) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ItemLock>(`/api/v1/players/${playerID}/locks/${lock.id}/cancel`)
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Lock stornieren fehlgeschlagen')
      }
    })
  }

  const searchClaimCharacters = async () => {
    if (!isOwnProfile || claimQuery.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setClaimStatus('')
      setClaimSelected(null)
      setClaimSearching(true)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(claimQuery.trim())}`)
        if (isActive()) setClaimResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können beansprucht werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen')
      } finally {
        if (isActive()) setClaimSearching(false)
      }
    })
  }

  const requestClaim = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !claimSelected) return
    await submit(async () => {
      setError('')
      setClaimStatus('')
      try {
        await api.post('/api/v1/characters/claim-requests', { profile_id: claimSelected.profile_id })
        if (isActive()) setClaimStatus(`Anfrage für ${claimSelected.name} erstellt. Ein Admin sendet dir den Code ingame an diesen Charakter.`)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfrage fehlgeschlagen')
      }
    })
  }

  const redeemClaim = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !claimCode.trim()) return
    await submit(async () => {
      setError('')
      setClaimStatus('')
      try {
        await api.post('/api/v1/characters/claim-with-code', { code: claimCode })
        if (!isActive()) return
        setClaimCode('')
        setClaimStatus('Charakter wurde beansprucht.')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht eingelöst werden')
      }
    })
  }

  const setAccount = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.put<Player>(`/api/v1/players/${playerID}/account`, {
          username: accUser,
          password: accPass,
          role: accRole,
        })
        if (!isActive()) return
        setAccPass('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Konto speichern fehlgeschlagen')
      }
    })
  }

  const mergeAccount = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !mergeSourceID || !mergeReason.trim()) return
    if (!window.confirm('Quellspielerkonto wirklich zusammenführen und danach endgültig löschen?')) return
    await submit(async () => {
      setError('')
      try {
        await api.post(`/api/v1/players/${playerID}/merge`, {
          source_player_id: Number(mergeSourceID),
          reason: mergeReason.trim(),
        })
        if (!isActive()) return
        setMergeSourceID('')
        setMergeReason('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Konten zusammenführen fehlgeschlagen')
      }
    })
  }

  const mergeCharacter = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin || !mergeCharacterTarget || !mergeCharacterSource || mergeCharacterTarget === mergeCharacterSource || !mergeCharacterReason.trim()) return
    if (!window.confirm('Doppelten Charakter wirklich zusammenführen und danach endgültig löschen? Historische Einträge werden auf den Zielcharakter korrigiert.')) return
    await submit(async () => {
      setError('')
      try {
        await api.post(`/api/v1/characters/${mergeCharacterTarget}/merge`, {
          source_character_id: Number(mergeCharacterSource),
          reason: mergeCharacterReason.trim(),
        })
        if (!isActive()) return
        setMergeCharacterSource('')
        setMergeCharacterTarget('')
        setMergeCharacterReason('')
        await load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktere zusammenführen fehlgeschlagen')
      }
    })
  }

  const startTOTPSetup = async () => {
    if (!isOwnProfile || !mfaPassword.trim()) return
    await submit(async () => {
      setError('')
      setRecoveryCodes([])
      try {
        const setup = await api.post<{ secret: string; uri: string }>('/api/v1/auth/mfa/totp/setup', { current_password: mfaPassword })
        if (!isActive()) return
        setTotpSecret(setup.secret)
        setTotpURI(setup.uri)
        setMfaPassword('')
        setMfaPasswordPrompt('')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'MFA-Einrichtung konnte nicht gestartet werden')
      }
    })
  }

  const enableTOTP = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !totpCode.trim()) return
    await submit(async () => {
      setError('')
      try {
        const enabled = await api.post<{ recovery_codes: string[] }>('/api/v1/auth/mfa/totp/enable', { code: totpCode })
        if (!isActive()) return
        setRecoveryCodes(enabled.recovery_codes)
        setTotpCode('')
        setTotpSecret('')
        setTotpURI('')
        await loadMFAStatus()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'MFA-Code konnte nicht bestätigt werden')
      }
    })
  }

  const registerSecurityKey = async () => {
    if (!isOwnProfile || !mfaPassword.trim()) return
    await submit(async () => {
      setError('')
      setSecurityKeyStatus('')
      try {
        const begin = await api.post<WebAuthnBeginResponse<Parameters<typeof createWebAuthnCredential>[0]>>('/api/v1/auth/mfa/webauthn/register/begin', { current_password: mfaPassword })
        const response = await createWebAuthnCredential(begin.options)
        await api.post('/api/v1/auth/mfa/webauthn/register/finish', { session: begin.session, response })
        if (!isActive()) return
        setMfaPassword('')
        setMfaPasswordPrompt('')
        setSecurityKeyStatus('Sicherheitsschlüssel wurde registriert.')
        await loadMFAStatus()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Sicherheitsschlüssel konnte nicht registriert werden')
      }
    })
  }

  const disableTOTP = async () => {
    if (!isOwnProfile || !mfaPassword.trim()) return
    await submit(async () => {
      setError('')
      try {
        await api.post('/api/v1/auth/mfa/totp/disable', { current_password: mfaPassword })
        if (!isActive()) return
        setMfaPassword('')
        setMfaPasswordPrompt('')
        setRecoveryCodes([])
        await loadMFAStatus()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Authenticator konnte nicht entfernt werden')
      }
    })
  }

  const deleteSecurityKey = async (id: number) => {
    if (!isOwnProfile || !mfaPassword.trim()) return
    await submit(async () => {
      setError('')
      try {
        await api.post(`/api/v1/auth/mfa/webauthn/credentials/${id}/delete`, { current_password: mfaPassword })
        if (!isActive()) return
        setMfaPassword('')
        setMfaPasswordPrompt('')
        await loadMFAStatus()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Sicherheitsschlüssel konnte nicht entfernt werden')
      }
    })
  }

  const copyRecoveryCodes = async () => {
    const text = recoveryCodes.join('\n')
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      setCopyStatus('Wiederherstellungscodes kopiert.')
    } catch {
      setCopyStatus('Kopieren nicht möglich. Bitte manuell markieren.')
    }
  }

  const changeOwnPassword = async (event: FormEvent) => {
    event.preventDefault()
    if (!isOwnProfile || !currentPassword.trim() || !newPassword.trim()) return
    await submit(async () => {
      setError('')
      setPasswordStatus('')
      try {
        await api.post('/api/v1/auth/password', { current_password: currentPassword, new_password: newPassword })
        if (!isActive()) return
        setNewPassword('')
        setPasswordStatus('Passwort wurde geändert.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Passwort konnte nicht geändert werden')
      }
    })
  }

  if (!player) {
    return (
      <div>
        <p>
          <Link to="/players">← Spieler</Link>
        </p>
        {error ? <p className="error">{error}</p> : <p className="muted">Lade…</p>}
      </div>
    )
  }

  return (
    <div>
      <p>
        <Link to="/players">← Spieler</Link>
      </p>
      <header className={`card profile-header ${characterClassName(preferredCharacter?.class ?? '')}`}>
        <div className="profile-sigil" aria-hidden="true">{preferredCharacter?.name.slice(0, 1) ?? player.name.slice(0, 1)}</div>
        <div><p className="eyebrow">Spielerprofil</p><h1>{player.name} {player.role && <span className="badge">{player.role}</span>}</h1>
          {preferredCharacter ? <p><strong className="character-name">{preferredCharacter.name}</strong> · {preferredCharacter.class}{preferredCharacter.main_spec && ` · ${preferredCharacter.main_spec}`} {preferredCharacter.is_main && <span className="badge">Main</span>}</p> : <p className="muted">Noch keine eigenen Charaktere.</p>}
          {player.guild_rank_id != null && <p className="muted">Gildenrang: {ranks.find((r) => r.id === player.guild_rank_id)?.name ?? (rankError || (rankLoading ? 'Wird geladen…' : 'Nicht verfügbar'))}</p>}
        </div>
      </header>
      {error && <p className="error">{error}</p>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}

      <nav className="workspace-tabs" aria-label="Profilbereiche">
        <button type="button" aria-pressed={view === 'points'} aria-controls="profile-points" onClick={() => setView('points')}>EP</button>
        <button type="button" aria-pressed={view === 'loot'} aria-controls="profile-loot" onClick={() => setView('loot')}>Loot ({loot.length})</button>
        <button type="button" aria-pressed={view === 'characters'} aria-controls="profile-characters" onClick={() => setView('characters')}>Charaktere ({ownedCharacters.length})</button>
        <button type="button" aria-pressed={view === 'locks'} aria-controls="profile-locks" onClick={() => setView('locks')}>Item-Locks</button>
        {(isAdmin || isOwnProfile) && <button type="button" aria-pressed={view === 'account'} aria-controls="profile-account" onClick={() => setView('account')}>Konto</button>}
      </nav>
      <div id="profile-points" hidden={view !== 'points'}>
        <EnhancePointsCard points={points} showHistory />
      </div>

      <section id="profile-loot" hidden={view !== 'loot'} className="card">
        <div className="section-heading">
          <div><h2>Loot</h2><p className="muted">Tokens sind normale Loot-Einträge und können hier gefiltert werden.</p></div>
          <label>Filter<select value={lootFilter} onChange={(event) => setLootFilter(event.target.value as 'all' | 'tokens')}>
            <option value="all">Alles</option>
            <option value="tokens">Nur Tokens</option>
          </select></label>
        </div>
        <div className="table-scroll"><table className="table">
          <thead><tr><th>Item</th><th>Boss</th><th>Charakter</th><th>Methode</th><th>Raid</th><th>Datum</th></tr></thead>
          <tbody>
            {visibleLoot.map((row) => {
              const method = row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
              return <tr key={row.id}>
                <td>{row.item_name}{row.token_award && <span className="badge">Token</span>}</td>
                <td>{row.boss || '—'}</td>
                <td>{row.awarded_to_character_id ? <Link className="character-name" to={`/characters/${row.awarded_to_character_id}`}>{row.awarded_to_character_name || 'Charakterprofil'}</Link> : <span className="character-name">{row.awarded_to_character_name || '—'}</span>}</td>
                <td>{LOOT_METHOD_LABELS[method]}</td>
                <td><Link to={`/raids/${row.raid_id}`}>Raid #{row.raid_id}</Link></td>
                <td>{new Date(row.awarded_at).toLocaleDateString('de-DE')}</td>
              </tr>
            })}
            {visibleLoot.length === 0 && <tr><td colSpan={6} className="muted">{lootFilter === 'tokens' ? 'Keine Tokens gefunden.' : 'Noch kein Loot.'}</td></tr>}
          </tbody>
        </table></div>
      </section>

      <section id="profile-characters" hidden={view !== 'characters'} className="card">
        <h2>Charaktere</h2>
        {transferStatus && <p className="muted" role="status">{transferStatus}</p>}

        {isAdmin && (
          <details className="editor-disclosure"><summary>Charakter hinzufügen</summary>
          {optionsError && <p className="error">{optionsError} <button type="button" onClick={reloadOptions}>Erneut laden</button></p>}
          <form className="inline-form" onSubmit={createCharacter}>
            <label>Rising-Gods-Charakter<input value={charQuery} onChange={(e) => setCharQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={pending || charQuery.trim().length < 3} onClick={() => void searchNewCharacters()}>{charSearching ? 'Suche…' : 'Suchen'}</button>
            <label className="check">
              <input type="checkbox" checked={charIsMain} onChange={(e) => setCharIsMain(e.target.checked)} /> Main
            </label>
            <button type="submit" disabled={pending || !charSelected}>Anlegen</button>
            {charSelected && <p className="muted">Ausgewählt: {charSelected.name} · {charSelected.class} · {charSelected.main_spec}</p>}
          </form>
          {charSearching && <p className="search-status" role="status"><span className="search-spinner" aria-hidden="true" />Rising-Gods-Suche läuft… Das kann einige Sekunden dauern. Schneller geht es mit einem Rising-Gods-Profil-Link.</p>}
          {charResults.length > 0 && <div className="plain-list">{charResults.map((row) => <button key={row.profile_id} type="button" className={charSelected?.profile_id === row.profile_id ? 'wishlist-toggle active' : 'wishlist-toggle'} onClick={() => setCharSelected(row)}>{row.name} · {row.class} · {row.main_spec || 'Spec unbekannt'} · Stufe {row.level || '—'}</button>)}</div>}
          </details>
        )}
        {isOwnProfile && (
          <details className="editor-disclosure"><summary>Charakter beanspruchen</summary>
          <p className="muted">Suche deinen Rising-Gods-Charakter. Ein Admin sendet dir den Code ingame an diesen Charakter.</p>
          <div className="inline-form">
            <label>Charakter suchen<input value={claimQuery} onChange={(e) => setClaimQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={pending || claimQuery.trim().length < 3} onClick={() => void searchClaimCharacters()}>{claimSearching ? 'Suche…' : 'Suchen'}</button>
          </div>
          {claimSearching && <p className="search-status" role="status"><span className="search-spinner" aria-hidden="true" />Rising-Gods-Suche läuft… Das kann einige Sekunden dauern. Schneller geht es mit einem Rising-Gods-Profil-Link.</p>}
          {claimResults.length > 0 && <div className="plain-list">{claimResults.map((row) => <button key={row.profile_id} type="button" className={claimSelected?.profile_id === row.profile_id ? 'wishlist-toggle active' : 'wishlist-toggle'} onClick={() => setClaimSelected(row)}>{row.name} · {row.class} · Stufe {row.level || '—'}</button>)}</div>}
          <form className="inline-form" onSubmit={requestClaim}>
            <button type="submit" disabled={pending || !claimSelected}>Anfrage erstellen</button>
            {claimSelected && <p className="muted">Ausgewählt: {claimSelected.name} · {claimSelected.class}</p>}
          </form>
          <form className="inline-form" onSubmit={redeemClaim}>
            <label>Code<input value={claimCode} onChange={(e) => setClaimCode(e.target.value)} /></label>
            <button type="submit" disabled={pending || !claimCode.trim()}>Code einlösen</button>
          </form>
          {claimStatus && <p className="muted" role="status">{claimStatus}</p>}
          </details>
        )}
        {isAdmin && ownedCharacters.filter((c) => c.active).length > 1 && <form className="inline-form" onSubmit={mergeCharacter}>
          <h3>Doppelte Charaktere zusammenführen</h3>
          <p className="muted">Nur für fehlerhaft doppelt angelegte Charaktere desselben Spielers. Raid- und Loot-Zuordnungen werden korrigiert; bei Konflikten wird nichts verändert.</p>
          <label>Zielcharakter behalten<select required disabled={pending} value={mergeCharacterTarget} onChange={(e) => setMergeCharacterTarget(e.target.value)}>
            <option value="">Auswählen</option>
            {ownedCharacters.filter((c) => c.active).map((c) => <option key={c.id} value={c.id}>{c.name} (#{c.id})</option>)}
          </select></label>
          <label>Doppelten Charakter zusammenführen<select required disabled={pending} value={mergeCharacterSource} onChange={(e) => setMergeCharacterSource(e.target.value)}>
            <option value="">Auswählen</option>
            {ownedCharacters.filter((c) => c.active && String(c.id) !== mergeCharacterTarget).map((c) => <option key={c.id} value={c.id}>{c.name} (#{c.id})</option>)}
          </select></label>
          <label>Grund<input required disabled={pending} value={mergeCharacterReason} onChange={(e) => setMergeCharacterReason(e.target.value)} placeholder="Falsch importierter Charaktername" /></label>
          <button type="submit" disabled={pending || !mergeCharacterTarget || !mergeCharacterSource || mergeCharacterTarget === mergeCharacterSource || !mergeCharacterReason.trim()}>Charaktere zusammenführen</button>
        </form>}
        <div className="table-scroll"><table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Klasse</th>
              <th>Main-Spec</th>
              <th>GearScore</th>
              <th>Rising Gods</th>
              <th>Main</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {ownedCharacters.map((c) => (
              <tr key={c.id} className={characterClassName(c.class)}>
                {editingCharacter?.id === c.id ? <td colSpan={7}><form className="inline-form in-place-editor" onSubmit={saveCharacter}>
                  <label>Name<input autoFocus aria-label="Charakter bearbeiten: Name" disabled={pending} required value={characterDraft.name} onChange={(e) => changeCharacter({ name: e.target.value })} /></label>
                  <label>Klasse<select disabled={pending} value={characterDraft.class} onChange={(e) => changeCharacter({ class: e.target.value, main_spec: '', second_spec: '' })}>
                    {!CLASSES.includes(characterDraft.class) && <option value={characterDraft.class}>{characterDraft.class}</option>}
                    {CLASSES.map((cl) => <option key={cl} value={cl}>{cl}</option>)}
                  </select></label>
                  <SpecSelect label="Main-Spec" disabled={pending} value={characterDraft.main_spec} specs={classSpecs[canonicalClass(characterDraft.class)] ?? []} onChange={(value) => changeCharacter({ main_spec: value })} />
                  <SpecSelect label="Second-Spec" disabled={pending} value={characterDraft.second_spec} specs={classSpecs[canonicalClass(characterDraft.class)] ?? []} onChange={(value) => changeCharacter({ second_spec: value })} />
                  <label className="check"><input type="checkbox" disabled={pending} checked={characterDraft.is_main} onChange={(e) => changeCharacter({ is_main: e.target.checked })} />Main</label>
                  <label className="check"><input type="checkbox" disabled={pending} checked={characterDraft.active} onChange={(e) => changeCharacter({ active: e.target.checked })} />Aktiv</label>
                  <button type="submit" disabled={pending || !characterDraft.name.trim()}>Charakter speichern</button>
                  <button type="button" disabled={pending} onClick={() => editCharacter(null)}>Bearbeitung abbrechen</button>
                  {error && <p className="error" role="alert">{error}</p>}
                </form></td> : transferCharacter?.id === c.id ? <td colSpan={7}><form className="inline-form in-place-editor" onSubmit={transferCharacterOwner}>
                  <p className="muted"><strong>{c.name}</strong> gehört danach zum ausgewählten Spieler. Raid-Aufstellungen werden zugeordnet; Loot und EP bleiben unverändert.</p>
                  <label>Neuer Spieler<select autoFocus required disabled={pending} value={transferPlayerID} onChange={(e) => setTransferPlayerID(e.target.value)}>
                    <option value="">Auswählen</option>
                    {mergePlayers.map((candidate) => <option key={candidate.id} value={candidate.id}>{candidate.name}</option>)}
                  </select></label>
                  <button type="submit" disabled={pending || !transferPlayerID}>Charakter verschieben</button>
                  <button type="button" disabled={pending} onClick={() => setTransferCharacter(null)}>Abbrechen</button>
                  {error && <p className="error" role="alert">{error}</p>}
                </form></td> : <>
                <td><Link className="character-name" to={`/characters/${c.id}`}>{c.name}</Link></td>
                <td>{c.class}</td>
                <td>{c.main_spec || '—'}</td>
                <td>{c.gear_score ?? '—'}</td>
                <td>{c.external_profile_url ? <a href={c.external_profile_url} target="_blank" rel="noreferrer">verknüpft</a> : <span className="muted">—</span>}</td>
                <td>{c.is_main ? '★' : ''}</td>
                <td className="actions">
                  {isAdmin && (
                    <>
                      <button disabled={pending} onClick={() => toggleMain(c)}>Main umschalten</button>
                      <button disabled={pending} onClick={() => editCharacter(c)}>Bearbeiten</button>
                      <button disabled={pending} onClick={() => startCharacterTransfer(c)}>Verschieben</button>
                    </>
                  )}
                </td>
                </>}
              </tr>
            ))}
            {ownedCharacters.length === 0 && (
              <tr>
                <td colSpan={7} className="muted">
                  Keine Charaktere.
                </td>
              </tr>
            )}
          </tbody>
        </table></div>
      </section>

      <section id="profile-locks" hidden={view !== 'locks'} className="card">
        <h2>Item-Locks</h2>
        <div className="table-scroll"><table className="table">
          <thead>
            <tr>
              <th>Item</th>
              <th>Boss</th>
              <th>Raid</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {locks.map((l) => (
              <tr key={l.id}>
                <td>{l.item_name || l.item_id}</td>
                <td>{l.boss || '—'}</td>
                <td><Link to={`/raids/${l.raid_id}`}>Raid #{l.raid_id}</Link></td>
                <td>{l.status}</td>
                <td>
                  {isOwnMember && l.status === 'ACTIVE' && <button disabled={pending} onClick={() => cancelLock(l)}>Stornieren</button>}
                </td>
              </tr>
            ))}
            {locks.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Keine Locks.
                </td>
              </tr>
            )}
          </tbody>
        </table></div>
        {isOwnMember && (
          <form className="inline-form" onSubmit={createLock}>
            <input aria-label="Raid-ID" placeholder="Raid-ID" value={lockRaid} onChange={(e) => setLockRaid(e.target.value)} required />
            <input aria-label="Item" placeholder="Item" value={lockItem} onChange={(e) => setLockItem(e.target.value)} required />
            <input aria-label="Item-ID" placeholder="Item-ID" value={lockItemID} onChange={(e) => setLockItemID(e.target.value)} />
            <input aria-label="Boss" placeholder="Boss" value={lockBoss} onChange={(e) => setLockBoss(e.target.value)} />
            <button type="submit" disabled={pending}>Lock anlegen</button>
          </form>
        )}
      </section>

      {(isAdmin || isOwnProfile) && (
        <section id="profile-account" hidden={view !== 'account'} className="card">
          <h2>Konto</h2>
          {isAdmin && <section className="subcard">
            <h3>Kontodaten</h3>
           <form className="inline-form" onSubmit={setAccount}>
              <input aria-label="Benutzername" placeholder="Benutzername" value={accUser} onChange={(e) => setAccUser(e.target.value)} required />
              {!isOwnProfile && <input
                type="password"
                aria-label="Passwort (leer = unverändert)"
                placeholder="Passwort (leer = unverändert)"
                value={accPass}
                onChange={(e) => setAccPass(e.target.value)}
              />}
              <select aria-label="Kontorolle" value={accRole} onChange={(e) => setAccRole(e.target.value as Role)}>
                {ROLES.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
              <button type="submit" disabled={pending}>Speichern</button>
            </form>
            {isOwnProfile && <p className="muted">Dein eigenes Passwort änderst du im separaten Bereich darunter.</p>}
           </section>}
           {isAdmin && <section className="subcard">
             <h3>Konto zusammenführen</h3>
             <p className="muted">Der Quellaccount wird deaktiviert. Seine Charaktere, Locks und sein EP-Guthaben werden diesem Konto zugeordnet; Quell-Sitzungen und MFA werden widerrufen.</p>
             <form className="inline-form" onSubmit={mergeAccount}>
               <label>Quellspieler<select aria-label="Quellspieler" value={mergeSourceID} onChange={(event) => setMergeSourceID(event.target.value)} required>
                 <option value="">Bitte wählen</option>
                 {mergePlayers.map((row) => <option key={row.id} value={row.id}>{row.name} (#{row.id})</option>)}
               </select></label>
               <label>Grund<input aria-label="Grund für Kontozusammenführung" value={mergeReason} onChange={(event) => setMergeReason(event.target.value)} required /></label>
               <button type="submit" disabled={pending || !mergeSourceID || !mergeReason.trim()}>Zusammenführen</button>
             </form>
           </section>}
           {isOwnProfile && <section className="subcard">
            <h3>Passwort ändern</h3>
            <form className="inline-form" onSubmit={changeOwnPassword}>
              <input type="password" aria-label="Aktuelles Passwort" placeholder="Aktuelles Passwort" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} autoComplete="current-password" />
              <input type="password" aria-label="Neues Passwort" placeholder="Neues Passwort (mind. 12 Zeichen)" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoComplete="new-password" />
              <button type="submit" disabled={pending || !currentPassword.trim() || !newPassword.trim()}>Passwort ändern</button>
            </form>
            {passwordStatus && <p className="muted" role="status">{passwordStatus}</p>}
          </section>}
          {isOwnProfile && <section className="subcard">
            <h3>Multi-Faktor-Authentifizierung</h3>
            <p className="muted">Optional: Richte einen Authenticator-Code oder YubiKey ein. Du wirst beim Start einer Änderung nach deinem aktuellen Passwort gefragt.</p>
            <p className="muted">Authenticator-App: {mfaStatus.totp_enabled ? 'aktiv' : 'nicht aktiv'}</p>
            {!mfaStatus.totp_enabled && !totpSecret && <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('totp_setup') }}>Authenticator einrichten</button>}
            {mfaStatus.totp_enabled && <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('totp_disable') }}>Authenticator entfernen</button>}
            {mfaPasswordPrompt === 'totp_setup' && <form className="inline-form" onSubmit={(event) => { event.preventDefault(); void startTOTPSetup() }}>
              <input type="password" aria-label="Aktuelles Passwort für MFA" placeholder="Aktuelles Passwort bestätigen" value={mfaPassword} onChange={(e) => setMfaPassword(e.target.value)} autoComplete="current-password" autoFocus />
              <button type="submit" disabled={pending || !mfaPassword.trim()}>Fortfahren</button>
              <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('') }}>Abbrechen</button>
            </form>}
            {mfaPasswordPrompt === 'totp_disable' && <form className="inline-form" onSubmit={(event) => { event.preventDefault(); void disableTOTP() }}>
              <input type="password" aria-label="Aktuelles Passwort zum Entfernen" placeholder="Aktuelles Passwort bestätigen" value={mfaPassword} onChange={(e) => setMfaPassword(e.target.value)} autoComplete="current-password" autoFocus />
              <button type="submit" disabled={pending || !mfaPassword.trim()}>Entfernen bestätigen</button>
              <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('') }}>Abbrechen</button>
            </form>}
            {totpSecret && <form className="inline-form" onSubmit={enableTOTP}>
              {qrDataURL && <img className="totp-qr" src={qrDataURL} alt="QR-Code für Authenticator-App" />}
              <p className="muted">Secret: <code>{totpSecret}</code></p>
              <p className="muted">URI für Authenticator-App: <code>{totpURI}</code></p>
              <input aria-label="Authenticator-Code" placeholder="6-stelliger Code" value={totpCode} onChange={(e) => setTotpCode(e.target.value)} />
              <button type="submit" disabled={pending || !totpCode.trim()}>MFA aktivieren</button>
            </form>}
            {recoveryCodes.length > 0 && <div>
              <p className="muted">Wiederherstellungscodes werden nur jetzt angezeigt.</p>
              <button type="button" disabled={pending} onClick={() => void copyRecoveryCodes()}>Codes kopieren</button>
              {copyStatus && <p className="muted" role="status">{copyStatus}</p>}
              <ul className="plain-list">{recoveryCodes.map((code) => <li key={code}><code>{code}</code></li>)}</ul>
            </div>}
            <h3>YubiKey / Sicherheitsschlüssel</h3>
            <p className="muted">Registriere einen FIDO2/WebAuthn-Sicherheitsschlüssel als zweiten Faktor.</p>
            {mfaStatus.security_keys.length > 0 && <ul className="plain-list">
              {mfaStatus.security_keys.map((key) => <li key={key.id}>{key.name || 'Sicherheitsschlüssel'} · registriert am {new Date(key.created_at).toLocaleDateString('de-DE')} {key.last_used_at ? `· zuletzt genutzt ${new Date(key.last_used_at).toLocaleDateString('de-DE')}` : ''} <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt(`security_key_delete:${key.id}`) }}>Entfernen</button>
                {mfaPasswordPrompt === `security_key_delete:${key.id}` && <form className="inline-form" onSubmit={(event) => { event.preventDefault(); void deleteSecurityKey(key.id) }}>
                  <input type="password" aria-label="Aktuelles Passwort zum Entfernen des Sicherheitsschlüssels" placeholder="Aktuelles Passwort bestätigen" value={mfaPassword} onChange={(e) => setMfaPassword(e.target.value)} autoComplete="current-password" autoFocus />
                  <button type="submit" disabled={pending || !mfaPassword.trim()}>Entfernen bestätigen</button>
                  <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('') }}>Abbrechen</button>
                </form>}
              </li>)}
            </ul>}
            {browserSupportsWebAuthn()
              ? <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('security_key') }}>Sicherheitsschlüssel registrieren</button>
              : <p className="muted">Dieser Browser unterstützt WebAuthn nicht.</p>}
            {mfaPasswordPrompt === 'security_key' && <form className="inline-form" onSubmit={(event) => { event.preventDefault(); void registerSecurityKey() }}>
              <input type="password" aria-label="Aktuelles Passwort für Sicherheitsschlüssel" placeholder="Aktuelles Passwort bestätigen" value={mfaPassword} onChange={(e) => setMfaPassword(e.target.value)} autoComplete="current-password" autoFocus />
              <button type="submit" disabled={pending || !mfaPassword.trim()}>Fortfahren</button>
              <button type="button" disabled={pending} onClick={() => { setMfaPassword(''); setMfaPasswordPrompt('') }}>Abbrechen</button>
            </form>}
            {securityKeyStatus && <p className="muted" role="status">{securityKeyStatus}</p>}
          </section>}
        </section>
      )}
    </div>
  )
}
