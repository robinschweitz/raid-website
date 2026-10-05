export function GuildCrest({ className = '' }: { className?: string }) {
  return <img
    className={`guild-crest ${className}`}
    src="/guild-emblem.png"
    alt="Bratwurst des Schicksals"
    aria-hidden="true"
  />
}
