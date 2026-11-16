<script setup lang="ts">
import { computed } from 'vue'
import type { RateSearchResult } from '@/api/types'
import { count, plural } from '@/lib/format'

// Every empty result explains WHY it is empty and offers the next step,
// using the server's hint rather than a generic "no results".
const props = defineProps<{ result: RateSearchResult }>()
const emit = defineEmits<{ search: [code: string]; clearProviders: []; resetFilters: [] }>()

const providerLabel = computed(() => {
  const q = props.result.query
  const parts = [...q.npi.map((n) => `NPI ${n}`), ...q.ein.map((e) => `EIN ${e}`)]
  return parts.join(' and ')
})
const totalForCode = computed(() => (props.result.code ? props.result.code.direct_count + props.result.code.linked_count : 0))
const sourceNames = computed(() => props.result.sources.map((s) => s.short_name).join(', '))
</script>

<template>
  <section class="card empty" role="status">
    <template v-if="result.hint === 'code_not_found'">
      <h2>No rates for <span class="mono">{{ result.query.code }}</span></h2>
      <p>
        This code does not appear in the loaded rate file{{ result.sources.length > 1 ? 's' : '' }}
        (<span class="mono">{{ sourceNames }}</span>). Check the code, or try a related one.
      </p>
      <div v-if="result.suggestions?.length" class="suggest">
        <span class="muted">Codes starting with {{ result.query.code.slice(0, 3) }}:</span>
        <button
          v-for="s in result.suggestions"
          :key="s.code"
          type="button"
          class="btn btn-sm"
          :title="s.description"
          @click="emit('search', s.code)"
        >
          <span class="mono">{{ s.code }}</span>
        </button>
      </div>
    </template>

    <template v-else-if="result.hint === 'provider_not_found'">
      <h2>No provider matches {{ providerLabel }}</h2>
      <p>
        <template v-if="result.query.npi.length && result.query.ein.length">
          No single provider group in the network has both this NPI and this tax ID.
        </template>
        <template v-else>This identifier is not in the loaded in-network file, so the provider may be out of network.</template>
      </p>
      <button type="button" class="btn" @click="emit('clearProviders')">Search all providers for {{ result.query.code }}</button>
    </template>

    <template v-else-if="result.hint === 'no_rates_for_provider'">
      <h2>{{ providerLabel }} has no rate for <span class="mono">{{ result.query.code }}</span></h2>
      <p>
        The provider is in network, but no negotiated rate for this code is listed for them.
        <template v-if="totalForCode">{{ result.query.code }} has {{ plural(totalForCode, 'rate') }} with other providers.</template>
      </p>
      <button type="button" class="btn" @click="emit('clearProviders')">
        See all {{ count(totalForCode) }} rates for {{ result.query.code }}
      </button>
    </template>

    <template v-else>
      <h2>No rates match these filters</h2>
      <p><span class="mono">{{ result.query.code }}</span> has rates, but none with the selected rate type, billing class or source.</p>
      <button type="button" class="btn" @click="emit('resetFilters')">Reset filters</button>
    </template>
  </section>
</template>

<style scoped>
.empty {
  padding: var(--space-6) var(--space-5);
  text-align: center;
}
h2 {
  font-size: 18px;
}
p {
  color: var(--text-2);
  max-width: 60ch;
  margin: var(--space-2) auto var(--space-4);
}
.suggest {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  align-items: center;
  gap: var(--space-2);
}
</style>
