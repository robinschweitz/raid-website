import { Fragment, useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type GeneratedRegistrationCode, type RegistrationCode, type RegistrationRequest } from '../types'
import { useSubmission } from '../useSubmission'

const pageSize = 20

const statusLabels: Record<string, string> = { PENDING: 'Offen', APPROVED: 'Code erstellt', REJECTED: 'Abgelehnt', CONSUMED: 'Eingelöst' }

// Which player a redeemed code was used for (ADR 084).
function linkedPlayerCell(request: RegistrationRequest) {
  const linked = request.linked_player
  if (!linked) return <span className="muted">—</span>
  return <span className="player-account">
    <Link to={`/players/${linked.player_id}`}>{linked.player_name}</Link>
    <small>{linked.username ? `@${linked.username} · ` : ''}{new Date(linked.used_at).toLocaleDateString('de-DE')}</small>
  </span>
}

export function RegistrationRequestsPage() {
  const { pending, submit, isActive } = useSubmission()
  const { player: me } = useAuth()
  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false
  const [requests, setRequests] = useState<RegistrationRequest[]>([])
  const [codes, setCodes] = useState<Record<number, RegistrationCode[]>>({})
  const [error, setError] = useState('')
  const [generated, setGenerated] = useState<Record<number, { code: string; codeId: number; characterName: string }>>({})
  const [copied, setCopied] = useState(false)
  const [view, setView] = useState<'active' | 'history'>('active')
  const [page, setPage] = useState(0)
  const [openRequestID, setOpenRequestID] = useState<number | null>(null)

  const load = useCallback(() => {
    setError('')
    const statuses = view === 'active' ? 'PENDING,APPROVED' : 'REJECTED,CONSUMED'
    api.get<RegistrationRequest[]>(`/api/v1/registration-requests?status=${statuses}`)
      .then((rows) => { if (isActive()) setRequests(rows) })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfragen konnten nicht geladen werden.') })
  }, [isActive, view])

  useEffect(() => { if (isAdmin) load() }, [load, isAdmin])

  const changeView = (next: 'active' | 'history') => {
    setView(next)
    setPage(0)
    setOpenRequestID(null)
    setCodes({})
  }

  const createCode = async (request: RegistrationRequest) => {
    await submit(async () => {
      setError('')
      try {
        const result = await api.post<GeneratedRegistrationCode>(`/api/v1/registration-requests/${request.id}/code`)
        if (!isActive()) return
        setGenerated((values) => ({ ...values, [request.id]: { code: result.code, codeId: result.code_record.id, characterName: request.character_name } }))
        setCodes((values) => ({ ...values, [request.id]: [result.code_record, ...(values[request.id] ?? [])] }))
        setCopied(false)
        setOpenRequestID(request.id)
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht erstellt werden.')
      }
    })
  }

  const reject = async (request: RegistrationRequest) => {
    await submit(async () => {
      setError('')
      try {
        await api.post<RegistrationRequest>(`/api/v1/registration-requests/${request.id}/reject`)
        if (isActive()) {
          setOpenRequestID(null)
          load()
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfrage konnte nicht abgelehnt werden.')
      }
    })
  }

  const updateCode = (updated: RegistrationCode) => {
    setCodes((values) => ({ ...values, [updated.request_id]: (values[updated.request_id] ?? []).map((code) => code.id === updated.id ? updated : code) }))
    setGenerated((values) => Object.fromEntries(Object.entries(values).filter(([, value]) => value.codeId !== updated.id)))
    setCopied(false)
  }

  const revoke = async (code: RegistrationCode) => {
    await submit(async () => {
      setError('')
      try {
        const updated = await api.post<RegistrationCode>(`/api/v1/registration-codes/${code.id}/revoke`)
        if (isActive()) updateCode(updated)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht widerrufen werden.')
      }
    })
  }

  const markSent = async (code: RegistrationCode) => {
    await submit(async () => {
      setError('')
      try {
        const updated = await api.post<RegistrationCode>(`/api/v1/registration-codes/${code.id}/sent`)
        if (isActive()) updateCode(updated)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht als gesendet markiert werden.')
      }
    })
  }

  const markExportedSent = async () => {
    const codeIDs = Object.values(generated).map((code) => code.codeId)
    if (codeIDs.length === 0) return
    await submit(async () => {
      setError('')
      try {
        const updated = await Promise.all(codeIDs.map((codeID) => api.post<RegistrationCode>(`/api/v1/registration-codes/${codeID}/sent`)))
        if (isActive()) updated.forEach(updateCode)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Codes konnten nicht als gesendet markiert werden.')
      }
    })
  }

  const toggleDetails = (request: RegistrationRequest) => {
    if (openRequestID === request.id) {
      setOpenRequestID(null)
      return
    }
    setOpenRequestID(request.id)
    if (codes[request.id]) return
    api.get<RegistrationCode[]>(`/api/v1/registration-requests/${request.id}/codes`)
      .then((rows) => { if (isActive()) setCodes((values) => ({ ...values, [request.id]: rows })) })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Codes konnten nicht geladen werden.') })
  }

  const addonCodes = Object.values(generated).map((code) => `${code.characterName}:${code.code}`).join('\n')
  const copyAddonCodes = async () => {
    if (!addonCodes) return
    await navigator.clipboard.writeText(addonCodes)
    setCopied(true)
  }

  const lastPage = Math.max(0, Math.ceil(requests.length / pageSize) - 1)
  const currentPage = Math.min(page, lastPage)
  const visibleRequests = requests.slice(currentPage * pageSize, (currentPage + 1) * pageSize)

  if (me && !isAdmin) return <div className="registration-requests">
    <p><Link to="/">← Übersicht</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Registrierungsanfragen</h1></div></div>
    <p className="muted" role="status">Dieser Bereich ist nur für Admins sichtbar.</p>
  </div>
  return <div className="registration-requests">
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Registrierungsanfragen</h1></div></div>
    <p className="muted">Codes werden nur einmal angezeigt. Die aktive Ansicht enthält nur offene Anfragen und aktive Codes.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {addonCodes && <section className="card">
      <h2>Addon-Export</h2>
      <p className="muted">Neue offene, noch nicht gesendete Codes dieser Sitzung im Format <code>Name:Code</code>.</p>
      <textarea className="addon-code-export" readOnly value={addonCodes} rows={Math.max(3, addonCodes.split('\n').length)} />
      <div className="actions">
        <button type="button" onClick={() => void copyAddonCodes()}>Für Addon kopieren</button>
        <button type="button" disabled={pending} onClick={() => void markExportedSent()}>Exportierte als gesendet markieren</button>
        {copied && <span className="muted" role="status">Kopiert.</span>}
      </div>
    </section>}
    <div className="workspace-tabs registration-tabs" role="group" aria-label="Anfragen filtern">
      <button type="button" aria-pressed={view === 'active'} onClick={() => changeView('active')}>Aktiv</button>
      <button type="button" aria-pressed={view === 'history'} onClick={() => changeView('history')}>Verlauf</button>
    </div>
    <p className="muted" role="status">{requests.length} {view === 'active' ? 'aktive' : 'historische'} Anfragen</p>
    {visibleRequests.length > 0 && <div className="registration-table-wrap"><table className="table registration-table">
      <thead><tr><th>Charakter</th><th>Anfrage</th><th>Status</th><th>Verknüpfter Spieler</th><th aria-label="Details" /><th>Aktionen</th></tr></thead>
      <tbody>{visibleRequests.map((request) => <Fragment key={request.id}>
        <tr>
          <td><strong>{request.character_name}</strong><small>{request.class} · {request.main_spec}{request.second_spec ? ` / ${request.second_spec}` : ''} · Stufe {request.level || '—'}</small></td>
          <td><small>{request.purpose === 'CLAIM_CHARACTER' ? 'Charakter beanspruchen' : 'Neues Konto'}<br />{new Date(request.created_at).toLocaleDateString('de-DE')}</small></td>
          <td><span className={`registration-status ${request.status.toLowerCase()}`}>{statusLabels[request.status] ?? request.status}</span></td>
          <td>{linkedPlayerCell(request)}</td>
          <td><button type="button" className="registration-details" aria-expanded={openRequestID === request.id} onClick={() => toggleDetails(request)}>{openRequestID === request.id ? 'Schließen' : 'Details'}</button></td>
          <td className="actions">
            <button disabled={pending || request.status === 'REJECTED' || request.status === 'CONSUMED'} onClick={() => void createCode(request)}>Code erstellen</button>
            <button disabled={pending || request.status === 'REJECTED' || request.status === 'CONSUMED'} onClick={() => void reject(request)}>Ablehnen</button>
          </td>
        </tr>
        {openRequestID === request.id && <tr className="registration-details-row"><td colSpan={6}>
          <div className="registration-details-content">
            <p><a href={request.external_profile_url} target="_blank" rel="noreferrer">Rising-Gods-Profil öffnen</a></p>
            {generated[request.id] && <p role="status"><strong>Neuer Code: {generated[request.id].code}</strong></p>}
            {codes[request.id] ? <div className="registration-codes"><strong>Codes</strong>{codes[request.id].length === 0 ? <p className="muted">Noch keine Codes erstellt.</p> : codes[request.id].map((code) => <div key={code.id}><span>#{code.id} · gültig bis {new Date(code.expires_at).toLocaleDateString('de-DE')} · {code.used_at ? `verwendet${code.used_by_username ? ` von @${code.used_by_username}` : ''}` : code.revoked_at ? 'widerrufen' : code.sent_at ? 'gesendet' : 'offen'}</span>{!code.used_at && !code.revoked_at && !code.sent_at && <button disabled={pending} onClick={() => void markSent(code)}>Als gesendet markieren</button>}{!code.used_at && !code.revoked_at && <button disabled={pending} onClick={() => void revoke(code)}>Widerrufen</button>}</div>)}</div> : <p className="muted">Codes werden geladen…</p>}
          </div>
        </td></tr>}
      </Fragment>)}</tbody>
    </table></div>}
    {requests.length === 0 && <p className="muted">{view === 'active' ? 'Keine aktiven Registrierungsanfragen.' : 'Keine historischen Registrierungsanfragen.'}</p>}
    {requests.length > pageSize && <div className="catalog-pagination"><button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>Zurück</button><span>Seite {currentPage + 1} / {lastPage + 1}</span><button type="button" disabled={currentPage === lastPage} onClick={() => setPage(currentPage + 1)}>Weiter</button></div>}
  </div>
}
