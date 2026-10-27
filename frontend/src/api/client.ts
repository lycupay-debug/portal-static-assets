export class ApiRequestError extends Error {
  readonly status: number
  readonly payload: unknown

  constructor(message: string, status: number, payload: unknown) {
    super(message)
    this.name = 'ApiRequestError'
    this.status = status
    this.payload = payload
  }
}

/** Shared same-origin JSON transport for API components. */
export async function apiRequest<T>(path: string, init: RequestInit = {}, fallbackMessage?: string): Promise<T> {
  const response = await fetch(`/api${path}`, {
    ...init,
    credentials: init.credentials ?? 'same-origin',
    headers: {
      Accept: 'application/json',
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...init.headers,
    },
  })

  const contentType = response.headers.get('content-type') || ''
  const payload = contentType.includes('application/json')
    ? await response.json().catch(() => null)
    : null

  if (!response.ok) {
    const message = payload && typeof payload === 'object' && 'error' in payload && typeof payload.error === 'string'
      ? payload.error
      : (fallbackMessage || `请求失败（${response.status}）`)
    throw new ApiRequestError(message, response.status, payload)
  }

  return payload as T
}
