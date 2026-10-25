import { useCallback, useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { isIOSDevice, isStandaloneDisplay } from '../installHint'
import { browserHasPushAPI, disablePush, enablePush, pushActiveOnThisDevice, pushAvailability, type PushAvailability, type PushConfig } from '../pushNotifications'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from './ErrorMessage'

// Opt-in push notifications on the own account tab (FR-32, ADR 102). Nothing is
// asked of the browser until the player presses the button.
export function PushNotificationSettings() {
  const [config, setConfig] = useState<PushConfig | null>(null)
  const [availability, setAvailability] = useState<PushAvailability | 'loading'>('loading')
  const [isSubscribed, setIsSubscribed] = useState(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const { pending, submit } = useSubmission()

  const refresh = useCallback(async () => {
    try {
      const loaded = await api.get<PushConfig>('/api/v1/push/config')
      setConfig(loaded)
      const hasPushAPI = browserHasPushAPI()
      setAvailability(pushAvailability({
        hasPushAPI,
        isIOS: isIOSDevice(),
        isStandalone: isStandaloneDisplay(),
        permission: hasPushAPI ? Notification.permission : 'unsupported',
        serverEnabled: loaded.enabled,
      }))
      setIsSubscribed(hasPushAPI && loaded.enabled && (await pushActiveOnThisDevice()))
    } catch {
      setAvailability('unsupported')
    }
  }, [])

  useEffect(() => { void refresh() }, [refresh])

  const enable = () => submit(async () => {
    if (!config) return
    setError('')
    setStatus('')
    try {
      await enablePush(config)
      setStatus('Benachrichtigungen sind auf diesem Gerät aktiv.')
    } catch (err) {
      if (err instanceof ApiError) setError(err.message)
      else if (err instanceof Error && err.message === 'denied') setError('Der Browser hat Benachrichtigungen blockiert. Du kannst das in den Website-Einstellungen des Browsers wieder erlauben.')
      else if (err instanceof Error && err.message === 'dismissed') setError('Benachrichtigungen wurden nicht erlaubt.')
      else setError('Benachrichtigungen konnten nicht eingerichtet werden.')
    }
    await refresh()
  })

  const disable = () => submit(async () => {
    setError('')
    setStatus('')
    try {
      await disablePush()
      setStatus('Benachrichtigungen sind auf diesem Gerät aus.')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Benachrichtigungen konnten nicht abgeschaltet werden.')
    }
    await refresh()
  })

  if (availability === 'loading' || availability === 'server-off') return null
  return <section className="subcard" aria-labelledby="push-settings-title">
    <h3 id="push-settings-title">Benachrichtigungen</h3>
    <p className="muted">Freiwillig: Dieses Gerät meldet sich, wenn eine neue Umfrage oder ein neuer Raid angelegt wird und wenn du in eine Raid-Aufstellung kommst. Die Meldung nennt keine Inhalte; Details siehst du erst auf der Website.</p>
    {availability === 'ios-install' && <p className="muted">Auf iPhone und iPad gibt es Benachrichtigungen nur, wenn du RAID zum Home-Bildschirm hinzugefügt hast (ab iOS 16.4) und es von dort öffnest.</p>}
    {availability === 'unsupported' && <p className="muted">Dieser Browser unterstützt keine Benachrichtigungen.</p>}
    {availability === 'denied' && <p className="muted">Der Browser blockiert Benachrichtigungen für diese Seite. Erlaube sie in den Website-Einstellungen des Browsers, um sie hier einzuschalten.</p>}
    {availability === 'available' && <>
      <p className="muted">Auf diesem Gerät: {isSubscribed ? 'aktiv' : 'aus'}</p>
      {isSubscribed
        ? <button type="button" disabled={pending} onClick={() => void disable()}>{pending ? 'Wird abgeschaltet …' : 'Benachrichtigungen abschalten'}</button>
        : <button type="button" disabled={pending} onClick={() => void enable()}>{pending ? 'Wird eingerichtet …' : 'Benachrichtigungen aktivieren'}</button>}
    </>}
    {status && <p className="muted" role="status">{status}</p>}
    {error && <ErrorMessage>{error}</ErrorMessage>}
  </section>
}
