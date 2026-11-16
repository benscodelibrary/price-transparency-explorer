<script setup lang="ts">
import { computed } from 'vue'
import type { PageInfo } from '@/api/types'
import { count } from '@/lib/format'

const props = defineProps<{ page: PageInfo }>()
const emit = defineEmits<{ go: [page: number] }>()

// A compact window of page numbers: 1 … 4 5 [6] 7 8 … 20
const pages = computed<(number | '…')[]>(() => {
  const { page: cur, total_pages: n } = props.page
  const set = new Set([1, n, cur - 2, cur - 1, cur, cur + 1, cur + 2].filter((p) => p >= 1 && p <= n))
  const sorted = [...set].sort((a, b) => a - b)
  const out: (number | '…')[] = []
  sorted.forEach((p, i) => {
    if (i > 0 && p - sorted[i - 1]! > 1) out.push('…')
    out.push(p)
  })
  return out
})

const range = computed(() => {
  const { page, page_size, total_providers } = props.page
  const start = (page - 1) * page_size + 1
  return `${count(start)}–${count(Math.min(page * page_size, total_providers))} of ${count(total_providers)} providers`
})
</script>

<template>
  <nav v-if="page.total_pages > 1" class="pager" aria-label="Result pages">
    <span class="muted">{{ range }}</span>
    <div class="buttons">
      <button type="button" class="btn btn-sm" :disabled="page.page <= 1" @click="emit('go', page.page - 1)">
        <span aria-hidden="true">‹</span> Previous
      </button>
      <template v-for="(p, i) in pages" :key="i">
        <span v-if="p === '…'" class="gap" aria-hidden="true">…</span>
        <button
          v-else
          type="button"
          class="btn btn-sm num"
          :class="{ current: p === page.page }"
          :aria-current="p === page.page ? 'page' : undefined"
          :aria-label="`Page ${p}`"
          @click="p !== page.page && emit('go', p)"
        >
          {{ p }}
        </button>
      </template>
      <button type="button" class="btn btn-sm" :disabled="page.page >= page.total_pages" @click="emit('go', page.page + 1)">
        Next <span aria-hidden="true">›</span>
      </button>
    </div>
  </nav>
</template>

<style scoped>
.pager {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: var(--space-3);
  font-size: 14px;
}
.buttons {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
  align-items: center;
}
.num {
  min-width: 36px;
}
.current {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}
.current:hover {
  background: var(--primary) !important;
}
.gap {
  padding: 0 4px;
  color: var(--text-3);
}
@media (max-width: 520px) {
  .num:not(.current) {
    display: none;
  }
}
</style>
