import type { Poll, Raid } from '../types'

interface GuildNotificationsProps {
  polls: Poll[]
  raids: Raid[]
}

export function GuildNotifications({ polls, raids }: GuildNotificationsProps) {
  const notifications: { icon: string; text: string; time: string }[] = []

  polls.filter((p) => p.status === 'OPEN').forEach((poll) => {
    notifications.push({
      icon: '\u{1F4F0}',
      text: `Neue Umfrage: ${poll.title}`,
      time: new Date(poll.created_at).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' }),
    })
  })

  raids
    .filter((r) => !r.archived_at && (r.status === 'PLANNED' || r.status === 'OPEN'))
    .sort((a, b) => Date.parse(a.scheduled_start ?? '') - Date.parse(b.scheduled_start ?? ''))
    .slice(0, 3)
    .forEach((raid) => {
      notifications.push({
        icon: '\u{2694}',
        text: `${raid.name} - ${raid.status === 'OPEN' ? 'Anmeldung offen' : 'Geplant'}`,
        time: raid.scheduled_start
          ? new Date(raid.scheduled_start).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' })
          : '',
      })
    })

  raids
    .filter((r) => r.archived_at)
    .sort((a, b) => Date.parse(b.archived_at ?? '') - Date.parse(a.archived_at ?? ''))
    .slice(0, 2)
    .forEach((raid) => {
      notifications.push({
        icon: '\u{2714}',
        text: `${raid.name} archiviert`,
        time: raid.archived_at
          ? new Date(raid.archived_at).toLocaleDateString('de-DE', { day: '2-digit', month: 'short' })
          : '',
      })
    })

  return <div>
    {notifications.length === 0 && <p className="muted">Keine Benachrichtigungen.</p>}
    {notifications.map((n, i) => <div className="notification-item" key={i}>
      <div className="notification-icon">{n.icon}</div>
      <div className="notification-body">
        <p>{n.text}</p>
        <time>{n.time}</time>
      </div>
    </div>)}
  </div>
}
