export function GuildCrest({ className = '' }: { className?: string }) {
  return <svg className={`guild-crest ${className}`} viewBox="0 0 120 140" fill="none" aria-hidden="true">
    <path d="M60 4 103 26 97 91 60 132 23 91 17 26Z" fill="#091725" stroke="#91bdcf" strokeWidth="2" />
    <path d="m60 13 34 18-6 55-28 33-28-33-6-55Z" fill="#102b40" stroke="#365e77" />
    <path d="m60 18 7 28 19-12-9 25 20 8-25 8 7 24-19-15-19 15 7-24-25-8 20-8-9-25 19 12Z" fill="#163b50" stroke="#79cce5" />
    <path d="M46 40h28l-4 37-10 20-10-20Z" fill="#b4d6e5" />
    <path d="m60 30 5 15-5 40-5-40Z" fill="#ebf8ff" />
    <path d="M36 108 60 121 84 108M12 48l-8-9 4 34 16 21M108 48l8-9-4 34-16 21" stroke="#608ca7" strokeWidth="2" />
  </svg>
}
