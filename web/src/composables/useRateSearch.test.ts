import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import type { LocationQuery } from 'vue-router'
import { useRateSearch } from './useRateSearch'

function result(code: string) {
  return {
    query: { code, npi: [], ein: [], match: 'all', sort: 'rate_asc' },
    code: null,
    summary: { rates: 1, providers: 1, direct_rates: 1, linked_rates: 0, min_cents: 1, median_cents: 1, max_cents: 1 },
    results: [],
    page: { page: 1, page_size: 25, total_providers: 1, total_pages: 1 },
    sources: [],
  }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

afterEach(() => vi.unstubAllGlobals())

describe('useRateSearch', () => {
  it('aborts the stale request so an old response cannot overwrite a newer one', async () => {
    const pending: { url: string; signal: AbortSignal; resolve: (r: Response) => void }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init: RequestInit) => new Promise<Response>((resolve) => pending.push({ url, signal: init.signal!, resolve }))),
    )

    const query = ref<LocationQuery>({ code: '99213' })
    const scope = effectScope()
    const s = scope.run(() => useRateSearch(() => query.value))!

    expect(s.status.value).toBe('loading')
    query.value = { code: '45380' }
    await nextTick()

    expect(pending).toHaveLength(2)
    expect(pending[0]!.signal.aborted).toBe(true)

    // The newer response arrives first, then the stale one.
    pending[1]!.resolve(new Response(JSON.stringify(result('45380'))))
    await flush()
    pending[0]!.resolve(new Response(JSON.stringify(result('99213'))))
    await flush()

    expect(s.status.value).toBe('success')
    expect(s.data.value?.query.code).toBe('45380')
    scope.stop()
  })

  it('exposes the server error envelope, including field errors', async () => {
    const body = { error: { code: 'invalid_query', message: 'bad', fields: [{ field: 'npi', code: 'invalid_npi', message: 'nope' }] } }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status: 400 })))

    const scope = effectScope()
    const s = scope.run(() => useRateSearch(() => ({ code: '99213', npi: '1' })))!
    await flush()

    expect(s.status.value).toBe('error')
    expect(s.error.value?.status).toBe(400)
    expect(s.error.value?.fields[0]?.field).toBe('npi')
    scope.stop()
  })

  it('stays idle with no code', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const scope = effectScope()
    const s = scope.run(() => useRateSearch(() => ({})))!
    expect(s.status.value).toBe('idle')
    expect(fetchMock).not.toHaveBeenCalled()
    scope.stop()
  })
})
