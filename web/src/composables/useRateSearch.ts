import { computed, onScopeDispose, ref, shallowRef, watch } from 'vue'
import type { LocationQuery } from 'vue-router'
import { ApiError, api, isAbort } from '@/api/client'
import type { RateSearchResult } from '@/api/types'
import { apiParams } from '@/lib/searchParams'

export type SearchStatus = 'idle' | 'loading' | 'success' | 'error'

/**
 * Runs a rate search whenever the route query changes.
 *
 * - Every new search aborts the one in flight, so a slow old response can
 *   never overwrite a newer one.
 * - The previous result stays visible while the next page or filter loads
 *   (`isRefreshing`), which avoids layout flashes when paging.
 */
export function useRateSearch(query: () => LocationQuery) {
  const status = ref<SearchStatus>('idle')
  const data = shallowRef<RateSearchResult | null>(null)
  const error = shallowRef<ApiError | null>(null)
  let controller: AbortController | null = null

  const params = computed(() => apiParams(query()))
  const key = computed(() => (params.value ? JSON.stringify(params.value) : ''))

  async function run(p: Record<string, string>) {
    controller?.abort()
    const c = new AbortController()
    controller = c
    status.value = 'loading'
    error.value = null
    try {
      const res = await api.searchRates(p, c.signal)
      if (c.signal.aborted) return
      data.value = res
      status.value = 'success'
    } catch (err) {
      if (isAbort(err)) return
      error.value = err instanceof ApiError ? err : new ApiError(0, 'unknown', String(err))
      status.value = 'error'
    }
  }

  function reset() {
    controller?.abort()
    status.value = 'idle'
    data.value = null
    error.value = null
  }

  function retry() {
    if (params.value) void run(params.value)
  }

  watch(
    key,
    () => {
      if (params.value) void run(params.value)
      else reset()
    },
    { immediate: true },
  )

  onScopeDispose(() => controller?.abort())

  const isRefreshing = computed(() => status.value === 'loading' && data.value !== null)

  return { status, data, error, isRefreshing, retry }
}
