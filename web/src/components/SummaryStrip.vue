<script setup lang="ts">
import { computed } from 'vue'
import type { SourceRef, Summary } from '@/api/types'
import { date, money, plural } from '@/lib/format'

const props = defineProps<{ summary: Summary; sources: SourceRef[] }>()

// Position of the median on the min-max bar, as a percentage.
const medianPos = computed(() => {
  const { min_cents: lo, max_cents: hi, median_cents: m } = props.summary
  return hi > lo ? ((m - lo) / (hi - lo)) * 100 : 50
})
</script>

<template>
  <section class="summary card" aria-label="Rate summary">
    <dl class="stats">
      <div>
        <dt>Rates</dt>
        <dd>{{ plural(summary.rates, 'rate') }}</dd>
        <dd class="sub">from {{ plural(summary.providers, 'provider group') }}</dd>
      </div>
      <div>
        <dt>Lowest</dt>
        <dd>{{ money(summary.min_cents) }}</dd>
      </div>
      <div>
        <dt>Median</dt>
        <dd class="median">{{ money(summary.median_cents) }}</dd>
      </div>
      <div>
        <dt>Highest</dt>
        <dd>{{ money(summary.max_cents) }}</dd>
      </div>
    </dl>

    <div class="range" aria-hidden="true">
      <div class="track">
        <span class="tick" :style="{ left: `${medianPos}%` }" />
      </div>
      <div class="range-labels muted">
        <span>{{ money(summary.min_cents) }}</span>
        <span>{{ money(summary.max_cents) }}</span>
      </div>
    </div>

    <p class="mix muted">
      <span v-if="summary.direct_rates">{{ plural(summary.direct_rates, 'rate') }} billed under this code</span>
      <span v-if="summary.direct_rates && summary.linked_rates"> · </span>
      <span v-if="summary.linked_rates">{{ plural(summary.linked_rates, 'facility rate') }} under revenue codes</span>
    </p>

    <p v-for="s in sources" :key="s.short_name" class="source muted">
      Source:
      <a :href="s.url" target="_blank" rel="noopener" class="mono">{{ s.short_name }}</a>
      <template v-if="s.last_updated_on"> · updated {{ date(s.last_updated_on) }}</template>
      · covers {{ plural(s.plans, 'plan') }}
    </p>
  </section>
</template>

<style scoped>
.summary {
  padding: var(--space-4) var(--space-5);
}
.stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--space-4);
  margin: 0;
}
dt {
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-3);
}
dd {
  margin: 2px 0 0;
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
dd.sub {
  font-size: 13px;
  font-weight: 400;
  color: var(--text-3);
}
.median {
  color: var(--primary);
}
.range {
  margin-top: var(--space-4);
}
.track {
  position: relative;
  height: 8px;
  border-radius: 4px;
  background: linear-gradient(90deg, var(--ok-soft), var(--primary-soft), var(--warn-soft));
  border: 1px solid var(--border);
}
.tick {
  position: absolute;
  top: -5px;
  width: 3px;
  height: 16px;
  margin-left: -1.5px;
  border-radius: 2px;
  background: var(--primary);
}
.range-labels {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  margin-top: 2px;
}
.mix,
.source {
  margin: var(--space-2) 0 0;
  font-size: 13px;
}
@media (max-width: 640px) {
  .summary {
    padding: var(--space-4);
  }
  .stats {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
