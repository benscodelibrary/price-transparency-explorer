import { onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import { api, isAbort } from '@/api/client'
import type { CodeRef } from '@/api/types'

// Shared across component instances: a prefix answered once is never asked again.
const cache = new Map<string, CodeRef[]>()
const DEBOUNCE_MS = 150
const MIN_PREFIX = 2

/** Debounced, cached, cancellable code typeahead. */
export function useCodeSuggest(input: Ref<string>) {
  const suggestions = shallowRef<CodeRef[]>([])
  const loading = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | null = null

  watch(input, (raw) => {
    clearTimeout(timer)
    controller?.abort()
    loading.value = false
    const prefix = raw.replace(/\s+/g, '').toUpperCase()
    if (prefix.length < MIN_PREFIX || !/^[A-Z0-9]+$/.test(prefix)) {
      suggestions.value = []
      return
    }
    const hit = cache.get(prefix)
    if (hit) {
      suggestions.value = hit
      return
    }
    timer = setTimeout(async () => {
      const c = new AbortController()
      controller = c
      loading.value = true
      try {
        const codes = await api.suggestCodes(prefix, c.signal)
        cache.set(prefix, codes)
        if (!c.signal.aborted) suggestions.value = codes
      } catch (err) {
        // Typeahead is a nicety: on any failure just show no suggestions.
        if (!isAbort(err)) suggestions.value = []
      } finally {
        if (controller === c) loading.value = false
      }
    }, DEBOUNCE_MS)
  })

  onScopeDispose(() => {
    clearTimeout(timer)
    controller?.abort()
  })

  return { suggestions, loading }
}
