import { computed, ref, shallowRef } from 'vue'
import { api } from '@/api/client'
import type { SourcesResponse, StatusResponse } from '@/api/types'

// One shared poller for the whole app: the banner and the search view read
// the same state, and the server is polled only while ingest is running.

const status = shallowRef<StatusResponse | null>(null)
const sources = shallowRef<SourcesResponse | null>(null)
const unreachable = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let started = false

const LOADING_POLL_MS = 2000
const UNREACHABLE_POLL_MS = 5000

async function poll() {
  try {
    status.value = await api.status()
    unreachable.value = false
  } catch {
    unreachable.value = true
  }
  const state = status.value?.ingest.state
  if (unreachable.value) {
    timer = setTimeout(poll, UNREACHABLE_POLL_MS)
  } else if (state === 'loading') {
    timer = setTimeout(poll, LOADING_POLL_MS)
  } else if ((state === 'ready' || state === 'degraded') && !sources.value) {
    void loadSources()
  }
}

async function loadSources() {
  try {
    sources.value = await api.sources()
  } catch {
    // Source details are informational; search works without them.
  }
}

export function useStatus() {
  if (!started) {
    started = true
    void poll()
  }
  const state = computed(() => {
    if (unreachable.value) return 'unreachable' as const
    return status.value?.ingest.state ?? ('loading' as const)
  })
  const canSearch = computed(() => state.value === 'ready' || state.value === 'degraded')
  const loadedFiles = computed(() => (sources.value?.files ?? []).filter((f) => f.status === 'loaded' || f.status === 'partial'))

  return { status, sources, state, canSearch, loadedFiles }
}

/** Test hook: stop polling and forget state. */
export function _resetStatus() {
  clearTimeout(timer)
  started = false
  status.value = null
  sources.value = null
  unreachable.value = false
}
