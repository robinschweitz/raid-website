import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { api } from './api'
import type { Player } from './types'

interface AuthState {
  player: Player | null
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [player, setPlayer] = useState<Player | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api
      .get<Player>('/api/v1/auth/me')
      .then(setPlayer)
      .catch(() => setPlayer(null))
      .finally(() => setLoading(false))
  }, [])

  const login = async (username: string, password: string) => {
    const p = await api.post<Player>('/api/v1/auth/login', { username, password })
    setPlayer(p)
  }

  const logout = async () => {
    await api.post<void>('/api/v1/auth/logout')
    setPlayer(null)
  }

  return <AuthContext.Provider value={{ player, loading, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return ctx
}
