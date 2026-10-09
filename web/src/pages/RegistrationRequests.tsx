import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { Link } from '../router'
import type { GeneratedRegistrationCode, RegistrationCode, RegistrationRequest } from '../types'
import { useSubmission } from '../useSubmission'

export function RegistrationRequestsPage() {
  const { pending, submit, isActive } = useSubmission()
  const [requests, setRequests] = useState<RegistrationRequest[]>([])
  const [codes, setCodes] = useState<Record<number, RegistrationCode[]>>({})
  const [error, setError] = useState('')
  const [generated, setGenerated] = useState<Record<number, string>>({})

  const load = () => {
    setError('')
    api.get<RegistrationRequest[]>('/api/v1/registration-requests')
      .then(async (rows) => {
        const loaded = await Promise.all(rows.map(async (request) => [request.id, await api.get<RegistrationCode[]>(`/api/v1/registration-requests/${request.id}/codes`).catch(() => [])] as const))
        if (isActive()) { setRequests(rows); setCodes(Object.fromEntries(loaded)) }
      })
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfragen konnten nicht geladen werden.') })
  }

  useEffect(load, [isActive])

  const createCode = async (request: RegistrationRequest) => {
    await submit(async () => {
      setError('')
      try {
        const result = await api.post<GeneratedRegistrationCode>(`/api/v1/registration-requests/${request.id}/code`)
        if (!isActive()) return
        setGenerated((values) => ({ ...values, [request.id]: result.code }))
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
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anfrage konnte nicht abgelehnt werden.')
      }
    })
  }

  const revoke = async (code: RegistrationCode) => {
    await submit(async () => {
      setError('')
      try {
        await api.post<RegistrationCode>(`/api/v1/registration-codes/${code.id}/revoke`)
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht widerrufen werden.')
      }
    })
  }

  return <div>
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Registrierungsanfragen</h1></div></div>
    <p className="muted">Code erstellen und per Ingame-Post oder Whisper an den angezeigten Charakter senden. Codes werden nur einmal angezeigt.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <div className="grid">
      {requests.map((request) => <article className="card" key={request.id}>
        <h2>{request.character_name}</h2>
        <p>{request.class} · {request.main_spec}{request.second_spec ? ` / ${request.second_spec}` : ''} · Stufe {request.level || '—'}</p>
        <p className="muted">Status: {request.status} · Zweck: {request.purpose === 'CLAIM_CHARACTER' ? 'Charakter beanspruchen' : 'Neues Konto'}</p>
        <p><a href={request.external_profile_url} target="_blank" rel="noreferrer">Rising-Gods-Profil öffnen</a></p>
        {generated[request.id] && <p role="status"><strong>Code: {generated[request.id]}</strong></p>}
        {(codes[request.id] ?? []).length > 0 && <div className="subcard">
          <h3>Codes</h3>
          {(codes[request.id] ?? []).map((code) => <p key={code.id}>#{code.id} · gültig bis {new Date(code.expires_at).toLocaleDateString('de-DE')} · {code.used_at ? 'verwendet' : code.revoked_at ? 'widerrufen' : 'offen'} {!code.used_at && !code.revoked_at && <button disabled={pending} onClick={() => void revoke(code)}>Widerrufen</button>}</p>)}
        </div>}
        <div className="actions">
          <button disabled={pending || request.status === 'REJECTED' || request.status === 'CONSUMED'} onClick={() => void createCode(request)}>Code erstellen</button>
          <button disabled={pending || request.status === 'REJECTED' || request.status === 'CONSUMED'} onClick={() => void reject(request)}>Ablehnen</button>
        </div>
      </article>)}
    </div>
    {requests.length === 0 && <p className="muted">Keine Registrierungsanfragen.</p>}
  </div>
}
