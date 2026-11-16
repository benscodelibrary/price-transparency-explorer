<script setup lang="ts">
import { computed } from 'vue'
import type { ApiError } from '@/api/client'
import type { RateSearchResult } from '@/api/types'
import type { SearchStatus } from '@/composables/useRateSearch'
import { plural } from '@/lib/format'
import CodeHeader from './CodeHeader.vue'
import EmptyState from './EmptyState.vue'
import Pagination from './Pagination.vue'
import ProviderCard from './ProviderCard.vue'
import SummaryStrip from './SummaryStrip.vue'

const props = defineProps<{
  status: SearchStatus
  data: RateSearchResult | null
  error: ApiError | null
  isRefreshing: boolean
}>()
const emit = defineEmits<{ retry: []; page: [n: number]; search: [code: string]; clearProviders: []; resetFilters: [] }>()

const examples = [
  { code: '99213', label: 'Office visit, established patient' },
  { code: '45380', label: 'Colonoscopy with biopsy' },
  { code: '0450', label: 'Emergency room (revenue code)' },
  { code: '80053', label: 'Comprehensive metabolic panel' },
]

const hasResults = computed(() => !!props.data && props.data.results.length > 0)
const announcement = computed(() => {
  if (props.status === 'loading') return 'Searching…'
  if (props.status === 'error') return props.error?.message ?? 'Search failed.'
  const d = props.data
  if (!d) return ''
  if (!d.summary.rates) return `No rates found for ${d.query.code}.`
  return `Found ${plural(d.summary.rates, 'rate')} from ${plural(d.summary.providers, 'provider group')} for ${d.query.code}.`
})
// Field validation errors (400) are shown on the form itself.
const showError = computed(() => props.status === 'error' && props.error && props.error.status !== 400)
</script>

<template>
  <div class="results">
    <p class="sr-only" aria-live="polite">{{ announcement }}</p>

    <!-- Nothing searched yet -->
    <section v-if="status === 'idle'" class="card intro">
      <h2>Look up a negotiated rate</h2>
      <p>
        Enter a billing code to see what this plan pays in-network providers. Add an NPI or tax ID to narrow it to one
        provider. Results include facility rates filed under a revenue code when they reference your code.
      </p>
      <div class="examples">
        <button v-for="e in examples" :key="e.code" type="button" class="example" @click="emit('search', e.code)">
          <span class="mono">{{ e.code }}</span>
          <span class="muted">{{ e.label }}</span>
        </button>
      </div>
    </section>

    <!-- Server or network failure -->
    <section v-else-if="showError" class="card error-state" role="alert">
      <template v-if="error!.isNotReady">
        <h2>Rate data is still loading</h2>
        <p>Your search will run automatically as soon as it is ready.</p>
      </template>
      <template v-else>
        <h2>Search failed</h2>
        <p>{{ error!.message }}</p>
        <button type="button" class="btn" @click="emit('retry')">Try again</button>
      </template>
    </section>

    <!-- First load: skeleton -->
    <div v-else-if="status === 'loading' && !data" class="skeleton" aria-hidden="true">
      <div class="sk sk-title" />
      <div class="sk sk-strip" />
      <div v-for="i in 3" :key="i" class="sk sk-card" />
    </div>

    <!-- Results (stay visible, dimmed, while the next page or filter loads) -->
    <div v-else-if="data" class="stack" :class="{ refreshing: isRefreshing }" :aria-busy="isRefreshing">
      <CodeHeader v-if="data.code" :code="data.code" />

      <EmptyState
        v-if="data.hint"
        :result="data"
        @search="emit('search', $event)"
        @clear-providers="emit('clearProviders')"
        @reset-filters="emit('resetFilters')"
      />

      <template v-else>
        <SummaryStrip :summary="data.summary" :sources="data.sources" />
        <Pagination :page="data.page" @go="emit('page', $event)" />
        <ProviderCard
          v-for="p in data.results"
          :key="p.group_key"
          :provider="p"
          :code="data.query.code"
          :eins="data.query.ein"
        />
        <p v-if="!hasResults" class="card past-end">
          This page is past the last result.
          <button type="button" class="btn btn-link" @click="emit('page', 1)">Back to page 1</button>
        </p>
        <Pagination :page="data.page" @go="emit('page', $event)" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.results {
  margin-top: var(--space-5);
}
.stack {
  display: grid;
  gap: var(--space-4);
  transition: opacity 0.15s ease;
}
.refreshing {
  opacity: 0.55;
  pointer-events: none;
}
.intro,
.error-state {
  padding: var(--space-5);
}
.intro h2,
.error-state h2 {
  font-size: 18px;
}
.intro p,
.error-state p {
  color: var(--text-2);
  max-width: 70ch;
}
.error-state {
  border-color: var(--danger);
}
.examples {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--space-3);
}
.example {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: var(--space-3);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-2);
  color: var(--text);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.example:hover {
  border-color: var(--primary);
  background: var(--primary-soft);
}
.example .mono {
  font-weight: 700;
  font-size: 16px;
}
.example .muted {
  font-size: 13px;
}
.past-end {
  padding: var(--space-4);
  text-align: center;
}
.skeleton {
  display: grid;
  gap: var(--space-4);
}
.sk {
  border-radius: var(--radius-lg);
  background: linear-gradient(90deg, var(--surface-2) 25%, var(--border) 50%, var(--surface-2) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.2s infinite;
}
.sk-title {
  height: 56px;
  width: 60%;
}
.sk-strip {
  height: 120px;
}
.sk-card {
  height: 160px;
}
@keyframes shimmer {
  to {
    background-position: -200% 0;
  }
}
</style>
