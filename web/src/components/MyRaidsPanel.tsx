import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { Link } from '../router'
import { useSubmission } from '../useSubmission'
import { myRaidStatus, quickSignupChoice, upcomingRaids } from '../myRaids'
import type { AbsenceStatus, Character, Raid, SignupOverview } from '../types'
import { ErrorMessage } from './ErrorMessage'

interface Props {
  playerID: number
  raids: Raid[]
  characters: Character[]
}

const badgeClass: Record<string, string> = { absent: 'badge bad', tentative: 'badge warn', accepted: 'badge ok', signed: 'badge info', open: 'badge warn', closed: 'badge' }

// MyRaidsPanel answers "am I in?" for the next raids right on the overview,
// with one-click signup and "can't come" (ADR 081).
export function MyRaidsPanel({ playerID, raids, characters }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [now] = useState(() => Date.now())
  const upcoming = upcomingRaids(raids, now, 4)
  const [overviews, setOverviews] = useState<Record<number, SignupOverview>>({})
  const [declining, setDeclining] = useState<{ raidID: number; status: AbsenceStatus } | null>(null)
  const [reason, setReason] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const raidKey = upcoming.map((raid) => raid.id).join(',')

  const load = useCallback(async () => {
    const ids = raidKey ? raidKey.split(',').map(Number) : []
    const rows = await Promise.all(ids.map(async (id) => [id, await api.get<SignupOverview>(`/api/v1/raids/${id}/signups`)] as const))
    if (isActive()) setOverviews(Object.fromEntries(rows))
  }, [raidKey, isActive])

  useEffect(() => {
    void load().catch((err: unknown) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Anmeldungen konnten nicht geladen werden.') })
  }, [load, isActive])

  const run = (work: () => Promise<unknown>, success: string) => submit(async () => {
    setError('')
    setMessage('')
    try {
      await work()
      if (!isActive()) return
      setDeclining(null)
      setReason('')
      setMessage(success)
      await load()
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Speichern fehlgeschlagen. Bitte die Seite neu laden.')
    }
  })

  if (upcoming.length === 0) return null

  return <section className="card my-raids" aria-label="Meine nächsten Raids">
    <div className="section-heading"><div><h2>Meine nächsten Raids</h2><p className="muted">Hier meldest du dich an, sagst ab oder meldest, dass du noch unsicher bist. Die Raidleitung sieht es sofort.</p></div></div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {message && <p className="success-message" role="status"><span>{message}</span><button type="button" aria-label="Meldung schließen" onClick={() => setMessage('')}>Schließen</button></p>}
    <ul className="my-raid-list">{upcoming.map((raid) => {
      const overview = overviews[raid.id]
      const status = overview ? myRaidStatus(overview, playerID) : null
      const quick = overview ? quickSignupChoice(characters, overview.class_specs) : null
      const when = new Date(raid.scheduled_start!).toLocaleString('de-DE', { weekday: 'short', day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
      const signupLink = `/raids/${raid.id}/anmeldungen`
      const remove = (success: string) => run(() => api.del(`/api/v1/raids/${raid.id}/absences/${playerID}`), success)
      const ask = (absenceStatus: AbsenceStatus) => { setDeclining({ raidID: raid.id, status: absenceStatus }); setReason(''); setMessage('') }
      const cannot = <button type="button" disabled={pending} onClick={() => ask('DECLINED')}>Kann nicht</button>
      const unsure = <button type="button" disabled={pending} onClick={() => ask('TENTATIVE')}>Unsicher</button>
      return <li className={`my-raid my-raid-${status?.state ?? 'loading'}`} key={raid.id} aria-label={raid.name}>
        <div className="my-raid-head">
          <Link to={signupLink}><strong>{raid.name}</strong></Link>
          <time className="muted" dateTime={raid.scheduled_start!}>{when} Uhr</time>
        </div>
        {!status ? <p className="muted">Wird geladen…</p> : <>
          <div className="my-raid-status">
            <span className={badgeClass[status.state]}>{status.label}</span>
            {status.detail && <span className="my-raid-detail">{status.state === 'absent' ? 'Angemeldet war ' : 'mit '}{status.detail}</span>}
            {status.reason && <span className="my-raid-detail muted">„{status.reason}“</span>}
          </div>
          {declining?.raidID === raid.id ? <div className="my-raid-decline">
            <label>{declining.status === 'TENTATIVE' ? 'Was ist unsicher? (optional)' : 'Grund (optional)'}<input autoFocus value={reason} maxLength={200} disabled={pending} placeholder={declining.status === 'TENTATIVE' ? 'z. B. komme evtl. später' : 'z. B. krank oder Urlaub'} onChange={(event) => setReason(event.target.value)} /></label>
            <button type="button" className="primary-action my-raid-main" disabled={pending} onClick={() => void run(() => api.put(`/api/v1/raids/${raid.id}/absences/${playerID}`, { reason, status: declining.status }), declining.status === 'TENTATIVE' ? `Für ${raid.name} als unsicher gemeldet.` : `Absage für ${raid.name} gesendet.`)}>{declining.status === 'TENTATIVE' ? 'Als unsicher melden' : 'Absage senden'}</button>
            <div className="my-raid-secondary"><button type="button" disabled={pending} onClick={() => { setDeclining(null); setReason('') }}>Abbrechen</button></div>
          </div> : <>
            <div className="my-raid-primary">
              {status.state === 'open' && quick && <>
                <button type="button" className="primary-action my-raid-main" disabled={pending}
                  onClick={() => void run(() => api.put(`/api/v1/raids/${raid.id}/signups`, { choices: [{ character_id: quick.character.id, spec: quick.spec }] }), `Für ${raid.name} mit ${quick.character.name} angemeldet.`)}>
                  Mit {quick.character.name} ({quick.spec}) anmelden</button>
                <Link className="my-raid-link" to={signupLink}>Anderer Charakter oder Spec</Link>
              </>}
              {status.state === 'open' && !quick && <Link className="button-link my-raid-main" to={signupLink}>Anmelden</Link>}
              {(status.state === 'signed' || status.state === 'accepted' || status.state === 'closed') && <Link className="button-link my-raid-main" to={signupLink}>{status.state === 'closed' ? 'Raid ansehen' : 'Anmeldung ansehen'}</Link>}
              {status.state === 'absent' && <button type="button" className="my-raid-main" disabled={pending} onClick={() => void remove(`Du bist bei ${raid.name} wieder dabei.`)}>Doch dabei</button>}
              {status.state === 'tentative' && <button type="button" className="my-raid-main" disabled={pending} onClick={() => void remove(`Du bist bei ${raid.name} sicher dabei.`)}>Sicher dabei</button>}
            </div>
            <div className="my-raid-secondary">
              {status.state !== 'tentative' && unsure}
              {status.state !== 'absent' && cannot}
            </div>
          </>}
        </>}
      </li>
    })}</ul>
    <p className="muted"><Link to="/raids">Alle Raids ansehen →</Link></p>
  </section>
}
