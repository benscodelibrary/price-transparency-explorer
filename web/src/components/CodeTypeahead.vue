<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { CodeRef } from '@/api/types'
import { useCodeSuggest } from '@/composables/useCodeSuggest'
import { count } from '@/lib/format'

// Accessible combobox (WAI-ARIA 1.2 pattern): arrow keys move through
// suggestions, Enter picks one, Escape closes, and the active option is
// announced via aria-activedescendant while focus stays in the input.

const model = defineModel<string>({ required: true })
const props = defineProps<{ id: string; invalid?: boolean; describedBy?: string; disabled?: boolean }>()
const emit = defineEmits<{ select: [code: string] }>()

const { suggestions, loading } = useCodeSuggest(model)
const open = ref(false)
const active = ref(-1)
const listId = `${props.id}-listbox`

const visible = computed(() => open.value && suggestions.value.length > 0)
const activeId = computed(() => (visible.value && active.value >= 0 ? `${props.id}-opt-${active.value}` : undefined))

watch(suggestions, () => {
  active.value = -1
})

function choose(c: CodeRef) {
  model.value = c.code
  open.value = false
  emit('select', c.code)
}

function onKeydown(e: KeyboardEvent) {
  const n = suggestions.value.length
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      if (!open.value) open.value = true
      else if (n) active.value = (active.value + 1) % n
      break
    case 'ArrowUp':
      e.preventDefault()
      if (n) active.value = active.value <= 0 ? n - 1 : active.value - 1
      break
    case 'Enter': {
      const pick = visible.value && active.value >= 0 ? suggestions.value[active.value] : undefined
      if (pick) {
        e.preventDefault() // pick the code; a second Enter submits the form
        choose(pick)
      } else {
        open.value = false
      }
      break
    }
    case 'Escape':
      open.value = false
      break
  }
}
</script>

<template>
  <div class="combo">
    <input
      :id="id"
      v-model="model"
      class="input mono-input"
      type="text"
      role="combobox"
      inputmode="text"
      autocomplete="off"
      autocapitalize="characters"
      spellcheck="false"
      maxlength="12"
      placeholder="e.g. 99213, 45380, 0450"
      :disabled="disabled"
      :aria-invalid="invalid ? 'true' : undefined"
      :aria-describedby="describedBy"
      aria-autocomplete="list"
      :aria-expanded="visible"
      :aria-controls="listId"
      :aria-activedescendant="activeId"
      @input="open = true"
      @focus="open = true"
      @blur="open = false"
      @keydown="onKeydown"
    />
    <span v-if="loading" class="spinner" aria-hidden="true" />

    <ul v-show="visible" :id="listId" class="listbox card" role="listbox" aria-label="Matching billing codes">
      <li
        v-for="(c, i) in suggestions"
        :id="`${id}-opt-${i}`"
        :key="c.code"
        role="option"
        :aria-selected="i === active"
        :class="{ active: i === active }"
        @mousedown.prevent="choose(c)"
        @mousemove="active = i"
      >
        <span class="opt-head">
          <strong class="mono">{{ c.code }}</strong>
          <span v-if="c.type" class="badge">{{ c.type }}</span>
        </span>
        <span class="opt-desc">
          {{ c.description || (c.direct_count === 0 ? 'Facility rates only (billed under a revenue code)' : '') }}
        </span>
        <span class="opt-counts muted">
          <template v-if="c.direct_count">{{ count(c.direct_count) }} direct</template>
          <template v-if="c.direct_count && c.linked_count"> · </template>
          <template v-if="c.linked_count">{{ count(c.linked_count) }} facility</template>
        </span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.combo {
  position: relative;
}
.mono-input {
  font-family: var(--mono);
  letter-spacing: 0.02em;
}
.spinner {
  position: absolute;
  right: 12px;
  top: 12px;
  width: 16px;
  height: 16px;
  border: 2px solid var(--border-strong);
  border-top-color: var(--primary);
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
.listbox {
  position: absolute;
  z-index: 20;
  top: calc(100% + 4px);
  left: 0;
  width: max(100%, 380px);
  max-width: calc(100vw - 32px);
  max-height: 340px;
  overflow-y: auto;
  margin: 0;
  padding: var(--space-1);
  list-style: none;
}
.listbox li {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 2px var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  cursor: pointer;
}
.listbox li.active {
  background: var(--primary-soft);
}
.opt-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  grid-row: span 2;
}
.opt-desc {
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.opt-counts {
  font-size: 12px;
}
</style>
