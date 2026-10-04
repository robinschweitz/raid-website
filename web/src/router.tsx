import { useEffect, useState, type MouseEvent, type ReactNode } from 'react'

function readPath(): string {
  const hash = window.location.hash.replace(/^#/, '')
  return hash || '/'
}

export function navigate(to: string) {
  if (readPath() === to) {
    return
  }
  window.location.hash = to
}

export function useRoute(): string {
  const [path, setPath] = useState(readPath())
  useEffect(() => {
    const onChange = () => setPath(readPath())
    window.addEventListener('hashchange', onChange)
    return () => window.removeEventListener('hashchange', onChange)
  }, [])
  return path
}

interface LinkProps {
  to: string
  children: ReactNode
  className?: string
}

export function Link({ to, children, className }: LinkProps) {
  const onClick = (event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault()
    navigate(to)
  }
  return (
    <a href={`#${to}`} className={className} onClick={onClick}>
      {children}
    </a>
  )
}
