import type { LocationQuery } from 'vue-router'
import type { BillingClass, MatchMode, SortMode } from '@/api/types'
import { formatEIN, normalizeCode, validateEINs, validateNPIs } from './validate'

// The URL query string is the single source of truth for a search. Its keys
// are exactly the API's parameter names, so a page URL maps 1:1 to an API call
// and every search can be shared, bookmarked and reached with Back.

export interface SearchForm {
  code: string
  npi: string
  ein: string
  match: MatchMode
  billingClass: BillingClass | ''
  source: string
  sort: SortMode
}

export const DEFAULT_PAGE_SIZE = 25

export function emptyForm(): SearchForm {
  return { code: '', npi: '', ein: '', match: 'all', billingClass: '', source: '', sort: 'rate_asc' }
}

function first(v: LocationQuery[string] | undefined): string {
  const s = Array.isArray(v) ? v[0] : v
  return typeof s === 'string' ? s : ''
}

const matchModes: MatchMode[] = ['all', 'direct', 'linked']
const sortModes: SortMode[] = ['rate_asc', 'rate_desc', 'provider']
const classes: BillingClass[] = ['professional', 'institutional', 'both']

export function formFromQuery(q: LocationQuery): SearchForm {
  const f = emptyForm()
  f.code = first(q.code)
  f.npi = first(q.npi).split(',').join(', ')
  f.ein = first(q.ein)
    .split(',')
    .filter(Boolean)
    .map(formatEIN)
    .join(', ')
  const match = first(q.match) as MatchMode
  if (matchModes.includes(match)) f.match = match
  const cls = first(q.billing_class) as BillingClass
  if (classes.includes(cls)) f.billingClass = cls
  f.source = first(q.source)
  const sort = first(q.sort) as SortMode
  if (sortModes.includes(sort)) f.sort = sort
  return f
}

/** Builds a clean URL query from a (validated) form; defaults are omitted. */
export function queryFromForm(f: SearchForm, page = 1): Record<string, string> {
  const q: Record<string, string> = { code: normalizeCode(f.code) }
  const npis = validateNPIs(f.npi).values
  const eins = validateEINs(f.ein).values
  if (npis.length) q.npi = npis.join(',')
  if (eins.length) q.ein = eins.join(',')
  if (f.match !== 'all') q.match = f.match
  if (f.billingClass) q.billing_class = f.billingClass
  if (f.source) q.source = f.source
  if (f.sort !== 'rate_asc') q.sort = f.sort
  if (page > 1) q.page = String(page)
  return q
}

/** API parameters for a route query: the same keys plus the page size. */
export function apiParams(q: LocationQuery): Record<string, string> | null {
  const code = first(q.code)
  if (!code) return null
  const params: Record<string, string> = { page_size: String(DEFAULT_PAGE_SIZE) }
  for (const key of ['code', 'npi', 'ein', 'match', 'billing_class', 'source', 'sort', 'page']) {
    const v = first(q[key])
    if (v) params[key] = v
  }
  return params
}

export function pageFromQuery(q: LocationQuery): number {
  const n = Number(first(q.page))
  return Number.isInteger(n) && n > 0 ? n : 1
}
