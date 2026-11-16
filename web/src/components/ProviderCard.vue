<script setup lang="ts">
import { computed, ref } from 'vue'
import type { ProviderResult } from '@/api/types'
import { count, money, plural } from '@/lib/format'
import RateTable from './RateTable.vue'

const props = defineProps<{ provider: ProviderResult; code: string; eins?: string[] }>()

// Entities that matched the user's NPI/EIN filter are listed first so they
// are never hidden behind "show more".
const isMatch = (e: ProviderResult['entities'][number]) => !!e.matched_npis?.length || !!props.eins?.includes(e.tin)
const entities = computed(() => [...props.provider.entities].sort((a, b) => Number(isMatch(b)) - Number(isMatch(a))))

const COLLAPSED_RATES = 5
const COLLAPSED_ENTITIES = 3
const expanded = ref(false)
const showAllEntities = ref(false)

// Some groups (IPAs, networks) hold dozens of tax IDs; the title names the
// first and says how many others share the same contracted rate.
const title = computed(() => entities.value.find((e) => e.business_name)?.business_name ?? 'Unnamed provider group')
const others = computed(() => entities.value.length - 1)
const visibleEntities = computed(() =>
  showAllEntities.value ? entities.value : entities.value.slice(0, COLLAPSED_ENTITIES),
)
const hiddenEntities = computed(() => entities.value.length - COLLAPSED_ENTITIES)
const visibleRates = computed(() =>
  expanded.value ? props.provider.rates : props.provider.rates.slice(0, COLLAPSED_RATES),
)
const hidden = computed(() => props.provider.rates.length - COLLAPSED_RATES)
const priceLabel = computed(() => {
  const { min_cents: lo, max_cents: hi } = props.provider
  return lo === hi ? money(lo) : `${money(lo)} – ${money(hi)}`
})
</script>

<template>
  <article class="card provider" :aria-labelledby="`p-${provider.group_key}`">
    <header class="head">
      <div class="who">
        <h3 :id="`p-${provider.group_key}`">
          {{ title }}
          <span v-if="others > 0" class="others">+ {{ plural(others, 'other organization') }}</span>
        </h3>
        <ul class="entities">
          <li v-for="(e, i) in visibleEntities" :key="i">
            <span v-if="provider.entities.length > 1 && e.business_name" class="ename">{{ e.business_name }}</span>
            <span class="muted">{{ e.tin_type === 'ein' ? 'EIN' : 'TIN (NPI)' }}</span>
            <span class="mono">{{ e.tin }}</span>
            <span v-if="e.tin_only" class="badge badge-warn" title="The source lists no individual NPIs: the contract is at the tax ID level">
              TIN-level contract
            </span>
            <details v-else-if="e.npi_count" class="npis">
              <summary>{{ plural(e.npi_count, 'NPI') }}</summary>
              <span class="mono">{{ e.npi_sample.join(', ') }}</span>
              <span v-if="e.npi_count > e.npi_sample.length" class="muted">
                and {{ count(e.npi_count - e.npi_sample.length) }} more
              </span>
            </details>
            <span v-for="n in e.matched_npis" :key="n" class="badge badge-ok">Matches NPI {{ n }}</span>
            <span v-if="eins?.includes(e.tin)" class="badge badge-ok">Matches EIN</span>
          </li>
        </ul>
        <button
          v-if="hiddenEntities > 0"
          type="button"
          class="btn btn-link more-entities"
          :aria-expanded="showAllEntities"
          @click="showAllEntities = !showAllEntities"
        >
          {{ showAllEntities ? 'Show fewer organizations' : `Show ${hiddenEntities} more organizations` }}
        </button>
      </div>
      <div class="price">
        <span class="amount">{{ priceLabel }}</span>
        <span class="muted">{{ plural(provider.rate_count, 'rate') }}</span>
      </div>
    </header>

    <RateTable :rates="visibleRates" :code="code" />

    <footer v-if="hidden > 0 || provider.rates_truncated" class="foot">
      <button v-if="hidden > 0" type="button" class="btn btn-link" :aria-expanded="expanded" @click="expanded = !expanded">
        {{ expanded ? 'Show fewer rates' : `Show all ${provider.rates.length} rates` }}
      </button>
      <span v-if="provider.rates_truncated && expanded" class="muted">
        Showing the lowest {{ provider.rates.length }} of {{ count(provider.rate_count) }} rates.
      </span>
    </footer>

    <p class="trace muted">
      Source <span class="mono">{{ provider.source }}</span>
      <template v-if="provider.source_group_id"> · provider group #{{ provider.source_group_id }}</template>
      <template v-if="provider.network_names?.length"> · {{ provider.network_names.join(', ') }}</template>
    </p>
  </article>
</template>

<style scoped>
.provider {
  overflow: hidden;
}
.head {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5) var(--space-3);
  border-bottom: 1px solid var(--border);
}
h3 {
  font-size: 16px;
}
.entities {
  list-style: none;
  margin: var(--space-1) 0 0;
  padding: 0;
  font-size: 13px;
}
.entities li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px var(--space-2);
}
.ename {
  font-weight: 600;
}
.others {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-3);
}
.more-entities {
  margin-top: var(--space-1);
  font-size: 13px;
}
.npis summary {
  cursor: pointer;
  color: var(--primary);
}
.npis[open] {
  flex-basis: 100%;
}
.price {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  text-align: right;
  font-size: 13px;
  flex-shrink: 0;
}
.amount {
  font-size: 18px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.foot {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  align-items: center;
  padding: var(--space-2) var(--space-5);
  border-top: 1px solid var(--border);
  font-size: 14px;
}
.trace {
  margin: 0;
  padding: var(--space-2) var(--space-5);
  font-size: 12px;
  background: var(--surface-2);
  border-top: 1px solid var(--border);
}
@media (max-width: 640px) {
  .head {
    flex-direction: column;
    padding: var(--space-4);
  }
  .price {
    align-items: flex-start;
    text-align: left;
  }
  .foot,
  .trace {
    padding-left: var(--space-4);
    padding-right: var(--space-4);
  }
}
</style>
