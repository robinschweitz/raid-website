import type { ReactNode } from 'react'

interface ErrorMessageProps {
  children: ReactNode
}

// Every error in the UI renders through this component so screen readers announce it (DQ-5).
export function ErrorMessage({ children }: ErrorMessageProps) {
  return <p className="error" role="alert">{children}</p>
}
