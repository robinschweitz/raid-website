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
  const [generated, setGenerated] = useState<Record<number, { code: string; codeId: number; characterName: string }>>({})
  const [copied, setCopied] = useState(false)

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
        setGenerated((values) => ({ ...values, [request.id]: { code: result.code, codeId: result.code_record.id, characterName: request.character_name } }))
        setCopied(false)
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
        if (isActive()) {
          setGenerated((values) => Object.fromEntries(Object.entries(values).filter(([, value]) => value.codeId !== code.id)))
          setCopied(false)
          load()
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht widerrufen werden.')
      }
    })
  }

  const markSent = async (code: RegistrationCode) => {
    await submit(async () => {
      setError('')
      try {
        await api.post<RegistrationCode>(`/api/v1/registration-codes/${code.id}/sent`)
        if (isActive()) {
          setGenerated((values) => Object.fromEntries(Object.entries(values).filter(([, value]) => value.codeId !== code.id)))
          setCopied(false)
          load()
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Code konnte nicht als gesendet markiert werden.')
      }
    })
  }

  const markExportedSent = async () => {
    const codeIDs = Object.entries(generated)
      .filter(([requestID, generatedCode]) => {
        const code = (codes[Number(requestID)] ?? []).find((row) => row.id === generatedCode.codeId)
        return !code || (!code.sent_at && !code.used_at && !code.revoked_at)
      })
      .map(([, generatedCode]) => generatedCode.codeId)
    if (codeIDs.length === 0) return
    await submit(async () => {
      setError('')
      try {
        await Promise.all(codeIDs.map((codeID) => api.post<RegistrationCode>(`/api/v1/registration-codes/${codeID}/sent`)))
        if (isActive()) {
          setGenerated((values) => Object.fromEntries(Object.entries(values).filter(([, value]) => !codeIDs.includes(value.codeId))))
          setCopied(false)
          load()
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Codes konnten nicht als gesendet markiert werden.')
      }
    })
  }

  const addonCodes = Object.entries(generated)
    .filter(([requestID, generatedCode]) => {
      const code = (codes[Number(requestID)] ?? []).find((row) => row.id === generatedCode.codeId)
      return !code || (!code.sent_at && !code.used_at && !code.revoked_at)
    })
    .map(([, generatedCode]) => `${generatedCode.characterName}:${generatedCode.code}`)
    .join('\n')

  const copyAddonCodes = async () => {
    if (!addonCodes) return
    await navigator.clipboard.writeText(addonCodes)
    setCopied(true)
  }

  return <div>
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Registrierungsanfragen</h1></div></div>
    <p className="muted">Code erstellen und per Ingame-Post oder Whisper an den angezeigten Charakter senden. Codes werden nur einmal angezeigt.</p>
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
    <div className="grid">
      {requests.map((request) => <article className="card" key={request.id}>
        <h2>{request.character_name}</h2>
        <p>{request.class} · {request.main_spec}{request.second_spec ? ` / ${request.second_spec}` : ''} · Stufe {request.level || '—'}</p>
        <p className="muted">Status: {request.status} · Zweck: {request.purpose === 'CLAIM_CHARACTER' ? 'Charakter beanspruchen' : 'Neues Konto'}</p>
        <p><a href={request.external_profile_url} target="_blank" rel="noreferrer">Rising-Gods-Profil öffnen</a></p>
        {generated[request.id] && <p role="status"><strong>Code: {generated[request.id].code}</strong></p>}
        {(codes[request.id] ?? []).length > 0 && <div className="subcard">
          <h3>Codes</h3>
          {(codes[request.id] ?? []).map((code) => <p key={code.id}>#{code.id} · gültig bis {new Date(code.expires_at).toLocaleDateString('de-DE')} · {code.used_at ? 'verwendet' : code.revoked_at ? 'widerrufen' : code.sent_at ? 'gesendet' : 'offen'} {!code.used_at && !code.revoked_at && !code.sent_at && <button disabled={pending} onClick={() => void markSent(code)}>Als gesendet markieren</button>} {!code.used_at && !code.revoked_at && <button disabled={pending} onClick={() => void revoke(code)}>Widerrufen</button>}</p>)}
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
