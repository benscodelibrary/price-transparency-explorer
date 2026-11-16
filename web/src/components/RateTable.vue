<script setup lang="ts">
import type { Rate } from '@/api/types'
import { capitalize, date, posName, rateValue } from '@/lib/format'

defineProps<{ rates: Rate[]; code: string }>()
</script>

<template>
  <table class="rates">
    <caption class="sr-only">Negotiated rates for {{ code }}</caption>
    <thead>
      <tr>
        <th scope="col" class="num">Rate</th>
        <th scope="col">Billed as</th>
        <th scope="col">Class · setting</th>
        <th scope="col">Place of service</th>
        <th scope="col">Modifiers</th>
        <th scope="col">Contract ends</th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="(r, i) in rates" :key="i">
        <td data-label="Rate" class="num rate">
          {{ rateValue(r) }}
          <span v-if="r.negotiated_type !== 'negotiated'" class="ntype muted">{{ r.negotiated_type }}</span>
        </td>
        <td data-label="Billed as">
          <span v-if="r.match === 'direct'" class="badge badge-direct">Code {{ code }}</span>
          <span
            v-else
            class="badge badge-facility"
            :title="`Facility rate filed under revenue code ${r.via?.code} that references ${code}`"
          >
            Facility · {{ r.via?.code_type }} {{ r.via?.code }}
          </span>
        </td>
        <td data-label="Class · setting">{{ capitalize(r.billing_class) }} · {{ r.setting }}</td>
        <td data-label="Place of service">
          <span v-if="!r.service_codes.length" class="muted">—</span>
          <span v-for="sc in r.service_codes" :key="sc" class="pos">
            <span class="chip">{{ sc }}</span>
            <span v-if="posName(sc)" class="muted">{{ posName(sc) }}</span>
          </span>
        </td>
        <td data-label="Modifiers">
          <span v-if="!r.modifiers.length" class="muted">—</span>
          <span v-for="m in r.modifiers" :key="m" class="chip">{{ m }}</span>
        </td>
        <td data-label="Contract ends">
          <span v-if="r.open_ended" class="muted">No end date</span>
          <span v-else-if="r.expires">{{ date(r.expires) }}</span>
          <span v-else class="muted">Not stated</span>
        </td>
      </tr>
    </tbody>
  </table>
</template>

<style scoped>
.rates {
  width: 100%;
  border-collapse: collapse;
  font-size: 14px;
}
th {
  text-align: left;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border);
  white-space: nowrap;
}
td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border);
  vertical-align: top;
}
tbody tr:last-child td {
  border-bottom: 0;
}
.num {
  text-align: right;
}
.rate {
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.ntype {
  display: block;
  font-size: 12px;
  font-weight: 400;
}
.pos {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  margin: 0 8px 2px 0;
  font-size: 13px;
}
td .chip + .chip {
  margin-left: 4px;
}

/* Under 640px each row becomes a stacked mini-card with inline labels. */
@media (max-width: 640px) {
  thead {
    display: none;
  }
  tr {
    display: block;
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--border);
  }
  tbody tr:last-child {
    border-bottom: 0;
  }
  td {
    display: grid;
    grid-template-columns: 120px 1fr;
    gap: var(--space-2);
    padding: 3px var(--space-3);
    border: 0;
    text-align: left;
  }
  td::before {
    content: attr(data-label);
    font-size: 12px;
    font-weight: 600;
    color: var(--text-3);
  }
  .num {
    text-align: left;
  }
  td > .badge {
    justify-self: start;
  }
}
</style>
