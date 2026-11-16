<script setup lang="ts">
import { computed, nextTick, reactive, ref, watch } from 'vue'
import type { FieldError, SourceFile } from '@/api/types'
import type { SearchForm } from '@/lib/searchParams'
import { validateCode, validateEINs, validateNPIs } from '@/lib/validate'
import CodeTypeahead from './CodeTypeahead.vue'

const form = defineModel<SearchForm>({ required: true })
const props = defineProps<{ disabled?: boolean; disabledReason?: string; serverErrors?: FieldError[]; sources: SourceFile[] }>()
const emit = defineEmits<{ submit: []; clearProviders: [] }>()

type Field = 'code' | 'npi' | 'ein'
const errors = reactive<Record<Field, string | null>>({ code: null, npi: null, ein: null })
const otherError = ref<string | null>(null)
const showMore = ref(false)

// Errors returned by the server land on the same fields as client errors.
watch(
  () => props.serverErrors,
  (list) => {
    otherError.value = null
    for (const e of list ?? []) {
      if (e.field === 'code' || e.field === 'npi' || e.field === 'ein') errors[e.field] = e.message
      else otherError.value = e.message
    }
  },
)

// Clear a field's error as soon as the user edits it.
watch(() => form.value.code, () => (errors.code = null))
watch(() => form.value.npi, () => (errors.npi = null))
watch(() => form.value.ein, () => (errors.ein = null))

const activeFilters = computed(() => {
  const f = form.value
  return [f.match !== 'all', !!f.billingClass, !!f.source, f.sort !== 'rate_asc'].filter(Boolean).length
})
const hasProviderFilter = computed(() => !!form.value.npi.trim() || !!form.value.ein.trim())

function validate(): boolean {
  errors.code = validateCode(form.value.code)
  errors.npi = validateNPIs(form.value.npi).error
  errors.ein = validateEINs(form.value.ein).error
  const firstBad = (['code', 'npi', 'ein'] as Field[]).find((f) => errors[f])
  if (firstBad) {
    void nextTick(() => document.getElementById(`${firstBad}-input`)?.focus())
    return false
  }
  return true
}

function onSubmit() {
  if (props.disabled) return
  if (validate()) emit('submit')
}

function resetFilters() {
  Object.assign(form.value, { match: 'all', billingClass: '', source: '', sort: 'rate_asc' })
}
</script>

<template>
  <form class="card search-form" novalidate role="search" aria-label="Search negotiated rates" @submit.prevent="onSubmit">
    <div class="row">
      <div class="field code-field">
        <label for="code-input">Billing code <span aria-hidden="true" class="req">*</span></label>
        <CodeTypeahead
          id="code-input"
          v-model="form.code"
          :invalid="!!errors.code"
          :described-by="errors.code ? 'code-error' : 'code-hint'"
          @select="onSubmit"
        />
        <span v-if="errors.code" id="code-error" class="error" role="alert">{{ errors.code }}</span>
        <span v-else id="code-hint" class="hint">CPT, HCPCS or revenue code (RC)</span>
      </div>

      <div class="field">
        <label for="npi-input">Provider NPI <span class="muted">(optional)</span></label>
        <input
          id="npi-input"
          v-model="form.npi"
          class="input mono-input"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          placeholder="10 digits"
          :aria-invalid="errors.npi ? 'true' : undefined"
          :aria-describedby="errors.npi ? 'npi-error' : 'npi-hint'"
        />
        <span v-if="errors.npi" id="npi-error" class="error" role="alert">{{ errors.npi }}</span>
        <span v-else id="npi-hint" class="hint">Separate several with commas</span>
      </div>

      <div class="field">
        <label for="ein-input">Tax ID (EIN) <span class="muted">(optional)</span></label>
        <input
          id="ein-input"
          v-model="form.ein"
          class="input mono-input"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          placeholder="12-3456789"
          :aria-invalid="errors.ein ? 'true' : undefined"
          :aria-describedby="errors.ein ? 'ein-error' : 'ein-hint'"
        />
        <span v-if="errors.ein" id="ein-error" class="error" role="alert">{{ errors.ein }}</span>
        <span v-else id="ein-hint" class="hint">With or without the dash</span>
      </div>

      <div class="actions">
        <button class="btn btn-primary" type="submit" :disabled="disabled" :title="disabled ? disabledReason : undefined">
          Search
        </button>
      </div>
    </div>

    <div class="more-bar">
      <button
        type="button"
        class="btn-link btn"
        :aria-expanded="showMore"
        aria-controls="more-filters"
        @click="showMore = !showMore"
      >
        {{ showMore ? 'Hide filters' : 'More filters' }}
        <span v-if="activeFilters" class="badge badge-direct">{{ activeFilters }} active</span>
      </button>
      <button v-if="hasProviderFilter" type="button" class="btn-link btn" @click="emit('clearProviders')">
        Clear provider filter
      </button>
      <span v-if="disabled && disabledReason" class="muted">{{ disabledReason }}</span>
    </div>

    <div v-show="showMore" id="more-filters" class="row filters">
      <div class="field">
        <label for="match-select">Rate type</label>
        <select id="match-select" v-model="form.match" class="select">
          <option value="all">All rates</option>
          <option value="direct">Billed under this code</option>
          <option value="linked">Facility rates (under a revenue code)</option>
        </select>
      </div>
      <div class="field">
        <label for="class-select">Billing class</label>
        <select id="class-select" v-model="form.billingClass" class="select">
          <option value="">Any</option>
          <option value="professional">Professional</option>
          <option value="institutional">Institutional</option>
          <option value="both">Both</option>
        </select>
      </div>
      <div v-if="sources.length > 1" class="field">
        <label for="source-select">Source file</label>
        <select id="source-select" v-model="form.source" class="select">
          <option value="">All loaded files</option>
          <option v-for="s in sources" :key="s.short_name" :value="s.short_name">{{ s.short_name }}</option>
        </select>
      </div>
      <div class="field">
        <label for="sort-select">Sort providers by</label>
        <select id="sort-select" v-model="form.sort" class="select">
          <option value="rate_asc">Lowest rate first</option>
          <option value="rate_desc">Highest rate first</option>
          <option value="provider">Provider name</option>
        </select>
      </div>
      <div class="actions">
        <button type="button" class="btn btn-sm" :disabled="!activeFilters" @click="resetFilters">Reset filters</button>
      </div>
    </div>

    <p v-if="otherError" class="error other" role="alert">{{ otherError }}</p>
  </form>
</template>

<style scoped>
.search-form {
  padding: var(--space-4) var(--space-5);
}
.row {
  display: grid;
  grid-template-columns: minmax(200px, 1.3fr) 1fr 1fr auto;
  gap: var(--space-4);
  align-items: start;
}
.filters {
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  align-items: end;
  margin-top: var(--space-3);
  padding-top: var(--space-4);
  border-top: 1px solid var(--border);
}
.actions {
  padding-top: 23px;
}
.filters .actions {
  padding-top: 0;
}
.req {
  color: var(--danger);
}
.mono-input {
  font-family: var(--mono);
}
.more-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-4);
  margin-top: var(--space-3);
  font-size: 14px;
}
.other {
  margin: var(--space-3) 0 0;
  color: var(--danger);
}
@media (max-width: 800px) {
  .row {
    grid-template-columns: 1fr 1fr;
  }
  .code-field {
    grid-column: 1 / -1;
  }
  .actions {
    grid-column: 1 / -1;
    padding-top: 0;
  }
  .actions .btn {
    width: 100%;
  }
}
@media (max-width: 520px) {
  .search-form {
    padding: var(--space-4);
  }
  .row {
    grid-template-columns: 1fr;
  }
}
</style>
