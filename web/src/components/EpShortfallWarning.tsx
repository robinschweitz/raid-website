import { ErrorMessage } from './ErrorMessage'

export interface EpShortfall {
  player_id: number
  player_name: string
  balance: number
  points_taken: number
  points_not_taken: number
}

interface Props {
  shortfalls: EpShortfall[]
  pending: boolean
  onConfirm: () => void
  onCancel: () => void
}

// Warns before a correction would take more EP than a player has; the server
// limits the deduction at zero and the rest is never taken later (FR-16.6, FR-16.8).
export function EpShortfallWarning({ shortfalls, pending, onConfirm, onCancel }: Props) {
  return (
    <div>
      <ErrorMessage>
        <strong>EP reichen nicht für die Korrektur.</strong>{' '}
        {shortfalls.map((row) => (
          <span key={row.player_id}>{row.player_name}: Es werden nur {row.points_taken} EP abgezogen, {row.points_not_taken} EP werden nicht abgezogen. </span>
        ))}
        Der Kontostand bleibt bei 0 und die fehlenden Punkte werden auch später nicht abgezogen. Die Zahl wird im Audit-Log festgehalten.
      </ErrorMessage>
      <button type="button" disabled={pending} onClick={onConfirm}>{pending ? 'Speichere…' : 'Trotzdem speichern'}</button>{' '}
      <button type="button" disabled={pending} onClick={onCancel}>Abbrechen</button>
    </div>
  )
}
