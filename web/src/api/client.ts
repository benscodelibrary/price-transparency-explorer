import type { CodeRef, ErrorBody, FieldError, RateSearchResult, SourcesResponse, StatusResponse } from './types'

/** ApiError carries the server's error envelope so the UI can react to it. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly fields: FieldError[] = [],
    readonly retryAfter?: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }

  get isNotReady() {
    return this.status === 503
  }
}

const BASE = '/api/v1'

async function request<T>(path: string, params: Record<string, string> = {}, signal?: AbortSignal): Promise<T> {
  const qs = new URLSearchParams(params).toString()
  let res: Response
  try {
    res = await fetch(`${BASE}${path}${qs ? `?${qs}` : ''}`, { signal, headers: { Accept: 'application/json' } })
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') throw err
    throw new ApiError(0, 'network', 'Could not reach the server. Check your connection and try again.')
  }

  if (res.ok) return (await res.json()) as T

  let body: ErrorBody | undefined
  try {
    body = (await res.json()) as ErrorBody
  } catch {
    // Non-JSON error (proxy page, crash); fall through to a generic message.
  }
  const retry = Number(res.headers.get('Retry-After')) || undefined
  throw new ApiError(
    res.status,
    body?.error.code ?? 'http_error',
    body?.error.message ?? `The server returned an error (${res.status}).`,
    body?.error.fields ?? [],
    retry,
  )
}

export const api = {
  searchRates: (params: Record<string, string>, signal?: AbortSignal) =>
    request<RateSearchResult>('/rates', params, signal),

  suggestCodes: (prefix: string, signal?: AbortSignal) =>
    request<{ prefix: string; codes: CodeRef[] }>('/codes', { prefix, limit: '8' }, signal).then((r) => r.codes),

  status: (signal?: AbortSignal) => request<StatusResponse>('/status', {}, signal),

  sources: (signal?: AbortSignal) => request<SourcesResponse>('/sources', {}, signal),
}

export function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}
