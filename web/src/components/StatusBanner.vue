<script setup lang="ts">
import { computed } from 'vue'
import { useStatus } from '@/composables/useStatus'
import { bytes } from '@/lib/format'

const { status, state } = useStatus()

const files = computed(() => status.value?.ingest.files ?? [])
const problemFiles = computed(() => files.value.filter((f) => f.phase === 'failed' || f.phase === 'partial'))

function percent(done: number, total: number) {
  return total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0
}

const phaseLabel: Record<string, string> = {
  queued: 'Queued',
  downloading: 'Downloading',
  parsing: 'Indexing',
  done: 'Ready',
  partial: 'Partly loaded',
  failed: 'Failed',
}
</script>

<template>
  <div v-if="state === 'loading'" class="banner banner-info" role="status">
    <div class="container">
      <strong>Loading rate data…</strong>
      <span class="muted"> Search will be available in a few seconds.</span>
      <ul v-if="files.length" class="files">
        <li v-for="f in files" :key="f.url">
          <span class="mono">{{ f.short_name }}</span>
          <span class="phase">{{ phaseLabel[f.phase] ?? f.phase }}</span>
          <template v-if="f.phase === 'downloading' || f.phase === 'parsing'">
            <span class="bar" aria-hidden="true">
              <span :style="{ width: `${percent(f.bytes_done, f.bytes_total)}%` }" />
            </span>
            <span class="muted">{{ bytes(f.bytes_done) }} / {{ bytes(f.bytes_total) }}</span>
          </template>
        </li>
      </ul>
    </div>
  </div>

  <div v-else-if="state === 'degraded'" class="banner banner-warn" role="status">
    <div class="container">
      <strong>Some rate files did not load completely.</strong>
      Search works on the data that loaded.
      <ul class="files">
        <li v-for="f in problemFiles" :key="f.url">
          <span class="mono">{{ f.short_name }}</span>
          <span class="phase">{{ phaseLabel[f.phase] }}</span>
          <span class="muted">{{ f.error }}</span>
        </li>
      </ul>
    </div>
  </div>

  <div v-else-if="state === 'failed'" class="banner banner-danger" role="alert">
    <div class="container">
      <strong>Rate data failed to load.</strong>
      {{ status?.ingest.error }}
    </div>
  </div>

  <div v-else-if="state === 'unreachable'" class="banner banner-danger" role="alert">
    <div class="container">
      <strong>Cannot reach the server.</strong>
      Retrying automatically…
    </div>
  </div>
</template>

<style scoped>
.banner {
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--border);
  font-size: 14px;
}
.banner-info {
  background: var(--primary-soft);
}
.banner-warn {
  background: var(--warn-soft);
}
.banner-danger {
  background: var(--danger-soft);
  color: var(--danger);
}
.files {
  list-style: none;
  margin: var(--space-2) 0 0;
  padding: 0;
  display: grid;
  gap: var(--space-1);
}
.files li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}
.phase {
  font-weight: 600;
  min-width: 90px;
}
.bar {
  position: relative;
  width: 160px;
  height: 6px;
  border-radius: 3px;
  background: var(--surface);
  overflow: hidden;
}
.bar span {
  position: absolute;
  inset: 0 auto 0 0;
  background: var(--primary);
  transition: width 0.4s ease;
}
</style>
