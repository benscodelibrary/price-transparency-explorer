<script setup lang="ts">
import type { CodeRef } from '@/api/types'
import { count } from '@/lib/format'

defineProps<{ code: CodeRef }>()
</script>

<template>
  <div class="code-header">
    <div class="title">
      <h2>
        <span class="mono">{{ code.code }}</span>
        <span v-if="code.type" class="badge">{{ code.type }}</span>
      </h2>
      <p class="desc">
        {{ code.description || 'No description in the source file. This code appears only on facility claims under a revenue code.' }}
      </p>
    </div>
    <p class="counts">
      <span class="badge badge-direct" title="Rates billed directly under this code">
        {{ count(code.direct_count) }} billed under this code
      </span>
      <span
        class="badge badge-facility"
        title="Facility rates filed under a revenue code, with this code in the rate's additional information"
      >
        {{ count(code.linked_count) }} facility (revenue code)
      </span>
    </p>
  </div>
</template>

<style scoped>
.code-header {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: var(--space-3);
  align-items: flex-start;
}
h2 {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: 22px;
}
.desc {
  margin: var(--space-1) 0 0;
  color: var(--text-2);
  max-width: 70ch;
}
.counts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
  margin: 0;
}
</style>
