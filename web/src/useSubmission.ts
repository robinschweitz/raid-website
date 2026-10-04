import { useCallback, useLayoutEffect, useRef, useState } from 'react'

export function useSubmission() {
  const active = useRef(true)
  const busy = useRef(false)
  const [pending, setPending] = useState(false)

  useLayoutEffect(() => {
    active.current = true
    return () => { active.current = false }
  }, [])

  const isActive = useCallback(() => active.current, [])
  const isPending = useCallback(() => busy.current, [])
  const submit = useCallback(async (work: () => Promise<void>) => {
    // State alone cannot block a second event before React renders.
    if (busy.current || !active.current) return
    busy.current = true
    setPending(true)
    try {
      await work()
    } finally {
      busy.current = false
      if (active.current) setPending(false)
    }
  }, [])

  return { pending, submit, isActive, isPending }
}
