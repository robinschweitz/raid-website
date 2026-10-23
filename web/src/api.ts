// The authorization guards answer in English; the UI always shows German.
function germanGuardMessage(code: string, message?: string): string | undefined {
  if (code === 'forbidden' && message === 'insufficient permissions') return 'Dafür fehlt dir die Berechtigung.'
  if (code === 'unauthenticated' && message === 'login required') return 'Bitte melde dich an.'
  if (code === 'login_required' && message === 'login required') return 'Bitte melde dich an.'
  if (code === 'mfa_setup_required') return 'Als Admin musst du zuerst einen zweiten Faktor (Authenticator-App oder Sicherheitsschlüssel) einrichten.'
  return message
}

export class ApiError extends Error {
  status: number
  code: string
  data?: unknown

  constructor(status: number, code: string, message: string, data?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.data = data
  }
}

let csrfToken = ''

async function ensureCSRFToken(): Promise<string> {
  if (csrfToken) return csrfToken
  const token = await request<{ token: string }>('GET', '/api/v1/auth/csrf')
  csrfToken = token.token
  return csrfToken
}

async function request<T>(method: string, path: string, body?: unknown, envelope = false): Promise<T> {
  const headers: Record<string, string> = body !== undefined ? { 'Content-Type': 'application/json' } : {}
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers['X-CSRF-Token'] = await ensureCSRFToken()
  }
  const res = await fetch(path, {
    method,
    credentials: 'include',
    headers: Object.keys(headers).length > 0 ? headers : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (res.status === 204) {
    return undefined as T
  }
  const text = await res.text()
  let json: unknown = undefined
  if (text) {
    try {
      json = JSON.parse(text)
    } catch {
      json = undefined
    }
  }
  if (!res.ok) {
    const env = json as { error?: { code?: string; message?: string }; data?: unknown } | undefined
    const code = env?.error?.code ?? 'error'
    throw new ApiError(res.status, code, germanGuardMessage(code, env?.error?.message) ?? res.statusText, env?.data)
  }
  if (!envelope && json && typeof json === 'object' && 'data' in json) {
    return (json as { data: T }).data
  }
  return json as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  postEnvelope: <T>(path: string, body?: unknown) => request<T>('POST', path, body, true),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}
