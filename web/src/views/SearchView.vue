<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ResultsPanel from '@/components/ResultsPanel.vue'
import SearchForm from '@/components/SearchForm.vue'
import { useRateSearch } from '@/composables/useRateSearch'
import { useStatus } from '@/composables/useStatus'
import { formFromQuery, queryFromForm, type SearchForm as Form } from '@/lib/searchParams'

// SearchView owns the state. A submit only updates the URL; the URL change
// runs the search. Back/forward, refresh and shared links all take the same path.

const route = useRoute()
const router = useRouter()
const { canSearch, state, loadedFiles } = useStatus()
const { status, data, error, isRefreshing, retry } = useRateSearch(() => route.query)

const form = ref<Form>(formFromQuery(route.query))
watch(
  () => route.query,
  (q) => (form.value = formFromQuery(q)),
)

// A search that hit "still loading" (503) re-runs once data is ready.
watch(canSearch, (ok) => {
  if (ok && error.value?.isNotReady) retry()
})

watch(data, (d) => {
  document.title = d ? `${d.query.code} rates · Price Transparency Explorer` : 'Price Transparency Explorer'
})

const serverErrors = computed(() => (error.value?.status === 400 ? error.value.fields : []))
const disabledReason = computed(() => {
  switch (state.value) {
    case 'loading':
      return 'Rate data is still loading.'
    case 'failed':
      return 'Rate data failed to load.'
    case 'unreachable':
      return 'Cannot reach the server.'
    default:
      return ''
  }
})

function sameQuery(next: Record<string, string>) {
  const cur = route.query
  const keys = new Set([...Object.keys(next), ...Object.keys(cur)])
  return [...keys].every((k) => (next[k] ?? '') === (cur[k] ?? ''))
}

function submit(page = 1) {
  const next = queryFromForm(form.value, page)
  // Re-submitting an identical search re-runs it (e.g. after a network error).
  if (sameQuery(next)) retry()
  else void router.push({ query: next })
}

function goPage(n: number) {
  submit(n)
  void nextTick(() => document.getElementById('results-top')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

function searchCode(code: string) {
  form.value.code = code
  submit()
}

function clearProviders() {
  form.value.npi = ''
  form.value.ein = ''
  if (route.query.code) submit()
}

function resetFilters() {
  Object.assign(form.value, { match: 'all', billingClass: '', source: '', sort: 'rate_asc' })
  if (route.query.code) submit()
}
</script>

<template>
  <h1 class="sr-only">Search negotiated rates</h1>
  <SearchForm
    v-model="form"
    :disabled="!canSearch"
    :disabled-reason="disabledReason"
    :server-errors="serverErrors"
    :sources="loadedFiles"
    @submit="submit()"
    @clear-providers="clearProviders"
  />
  <div id="results-top" />
  <ResultsPanel
    :status="status"
    :data="data"
    :error="error"
    :is-refreshing="isRefreshing"
    @retry="retry"
    @page="goPage"
    @search="searchCode"
    @clear-providers="clearProviders"
    @reset-filters="resetFilters"
  />
</template>
