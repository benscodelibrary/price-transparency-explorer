<script setup lang="ts">
import { computed } from 'vue'
import { useStatus } from '@/composables/useStatus'
import { bytes, count, date, plural, shortHash } from '@/lib/format'

// Source traceability: what index was used, which files were loaded, their
// dates and checksums, and which plans they cover.
const { sources } = useStatus()

const loadedStructures = computed(() => (sources.value?.structures ?? []).filter((s) => s.loaded_files.length > 0))
</script>

<template>
  <details v-if="sources" class="card source-info">
    <summary>
      <span>
        <strong>Data source</strong>
        <span class="muted">
          · {{ sources.index.reporting_entity_name }} · index dated {{ date(sources.index.last_updated_on) }}
        </span>
      </span>
    </summary>

    <div class="body">
      <p class="muted index-src">
        Index: <a :href="sources.index.source" target="_blank" rel="noopener">{{ sources.index.source }}</a>
      </p>

      <table class="files">
        <thead>
          <tr>
            <th scope="col">File</th>
            <th scope="col">Updated</th>
            <th scope="col">Size</th>
            <th scope="col">Prices</th>
            <th scope="col">SHA-256</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in sources.files" :key="f.url">
            <td>
              <a :href="f.url" target="_blank" rel="noopener" class="mono">{{ f.short_name }}</a>
            </td>
            <td>{{ f.last_updated_on ? date(f.last_updated_on) : '—' }}</td>
            <td>{{ bytes(f.bytes) }}</td>
            <td>{{ count(f.counts.rates) }}</td>
            <td class="mono" :title="f.sha256">{{ shortHash(f.sha256) }}</td>
            <td>
              <span class="badge" :class="f.status === 'loaded' ? 'badge-ok' : 'badge-warn'">{{ f.status }}</span>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-for="s in loadedStructures" :key="s.key" class="plans">
        <h3>
          Plans covered by {{ s.loaded_files.join(', ') }}
          <span class="muted">({{ plural(s.plans.length, 'plan ID') }})</span>
        </h3>
        <ul>
          <li v-for="p in s.plans" :key="p.plan_id">
            <span class="mono">{{ p.plan_id }}</span>
            <span class="muted"> {{ p.plan_id_type.toUpperCase() }} · {{ p.plan_market_type }}</span>
            — {{ p.plan_names.join('; ') }}
          </li>
        </ul>
      </div>
    </div>
  </details>
</template>

<style scoped>
.source-info {
  font-size: 14px;
}
summary {
  cursor: pointer;
  padding: var(--space-3) var(--space-4);
  list-style-position: inside;
}
.body {
  padding: 0 var(--space-4) var(--space-4);
}
.index-src {
  word-break: break-all;
}
.files {
  width: 100%;
  border-collapse: collapse;
  margin: var(--space-3) 0;
}
.files th,
.files td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
.files th {
  font-size: 12px;
  color: var(--text-3);
  font-weight: 600;
}
.plans h3 {
  font-size: 14px;
  margin: var(--space-4) 0 var(--space-2);
}
.plans ul {
  margin: 0;
  padding-left: var(--space-4);
  color: var(--text-2);
}
@media (max-width: 640px) {
  .files th:nth-child(5),
  .files td:nth-child(5),
  .files th:nth-child(3),
  .files td:nth-child(3) {
    display: none;
  }
}
</style>
