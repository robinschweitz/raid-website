import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { api, ApiError } from './api'
import type { Player } from './types'
import { getWebAuthnCredential, type WebAuthnBeginResponse } from './webauthn'

export interface MFAFactors {
  totp_enabled: boolean
  recovery_codes: boolean
  webauthn: boolean
}

interface AuthState {
  player: Player | null
  loading: boolean
  error: string
  refresh: () => Promise<void>
  login: (username: string, password: string) => Promise<{ mfaRequired: boolean; challenge?: string; factors?: MFAFactors }>
  verifyMFA: (challenge: string, code: string) => Promise<void>
  verifySecurityKey: (challenge: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [player, setPlayer] = useState<Player | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const version = useRef(0)

  const refresh = useCallback(async () => {
    const requestVersion = ++version.current
    setLoading(true)
    setError('')
    try {
      const current = await api.get<Player>('/api/v1/auth/me')
      if (requestVersion === version.current) setPlayer(current)
    } catch (err) {
      if (requestVersion !== version.current) return
      if (err instanceof ApiError && err.status === 401) {
        setPlayer(null)
      } else {
        setError('Sitzung konnte nicht geladen werden. Bitte erneut versuchen.')
      }
    } finally {
      if (requestVersion === version.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    const requests = version
    void refresh()
    return () => { ++requests.current }
  }, [refresh])

  const login = async (username: string, password: string) => {
    const requestVersion = ++version.current
    const result = await api.post<Player | { mfa_required: true; challenge: string; factors: MFAFactors }>('/api/v1/auth/login', { username, password })
    if ('mfa_required' in result) {
      if (requestVersion === version.current) setLoading(false)
      return { mfaRequired: true, challenge: result.challenge, factors: result.factors }
    }
    if (requestVersion === version.current) {
      setPlayer(result)
      setError('')
      setLoading(false)
    }
    return { mfaRequired: false }
  }

  const verifyMFA = async (challenge: string, code: string) => {
    const requestVersion = ++version.current
    const p = await api.post<Player>('/api/v1/auth/mfa/verify', { challenge, code })
    if (requestVersion === version.current) {
      setPlayer(p)
      setError('')
      setLoading(false)
    }
  }

  const verifySecurityKey = async (challenge: string) => {
    const requestVersion = ++version.current
    const begin = await api.post<WebAuthnBeginResponse<Parameters<typeof getWebAuthnCredential>[0]>>('/api/v1/auth/mfa/webauthn/login/begin', { challenge })
    const response = await getWebAuthnCredential(begin.options)
    const p = await api.post<Player>('/api/v1/auth/mfa/webauthn/login/finish', { session: begin.session, response })
    if (requestVersion === version.current) {
      setPlayer(p)
      setError('')
      setLoading(false)
    }
  }

  const logout = async () => {
    const requestVersion = ++version.current
    try {
      await api.post<void>('/api/v1/auth/logout')
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 401)) throw err
    }
    if (requestVersion === version.current) {
      setPlayer(null)
      setError('')
      setLoading(false)
    }
  }

  return <AuthContext.Provider value={{ player, loading, error, refresh, login, verifyMFA, verifySecurityKey, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
