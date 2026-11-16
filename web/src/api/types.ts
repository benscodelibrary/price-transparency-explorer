// These types mirror the Go API contract (server/internal/search and
// server/internal/api). Money is always integer cents.

export type MatchMode = 'all' | 'direct' | 'linked'
export type SortMode = 'rate_asc' | 'rate_desc' | 'provider'
export type BillingClass = 'professional' | 'institutional' | 'both'

export type Hint =
  | 'code_not_found'
  | 'provider_not_found'
  | 'no_rates_for_provider'
  | 'no_rates_match_filters'

export interface CodeRef {
  code: string
  type: string
  description?: string
  direct_count: number
  linked_count: number
}

export interface Summary {
  rates: number
  providers: number
  direct_rates: number
  linked_rates: number
  min_cents: number
  median_cents: number
  max_cents: number
}

export interface PageInfo {
  page: number
  page_size: number
  total_providers: number
  total_pages: number
}

export interface SourceRef {
  short_name: string
  url: string
  last_updated_on?: string
  plans: number
}

export interface Entity {
  business_name?: string
  tin_type: string
  tin: string
  npi_count: number
  npi_sample: string[]
  matched_npis?: string[]
  tin_only?: boolean
}

/** The revenue code a facility rate is filed under. No description on
 * purpose: payers repeat one rate under many items with unrelated labels. */
export interface Via {
  code_type: string
  code: string
}

export interface Rate {
  cents: number
  is_percentage?: boolean
  match: 'direct' | 'linked'
  via?: Via
  negotiated_type: string
  billing_class: string
  setting: string
  service_codes: string[]
  modifiers: string[]
  expires: string | null
  open_ended?: boolean
}

export interface ProviderResult {
  group_key: number
  source_group_id?: number
  source: string
  network_names?: string[]
  entities: Entity[]
  rate_count: number
  min_cents: number
  max_cents: number
  rates: Rate[]
  rates_truncated?: boolean
}

export interface RateSearchResult {
  query: {
    code: string
    npi: string[]
    ein: string[]
    match: MatchMode
    billing_class?: BillingClass
    source?: string
    sort: SortMode
  }
  code: CodeRef | null
  summary: Summary
  results: ProviderResult[]
  page: PageInfo
  sources: SourceRef[]
  hint?: Hint
  suggestions?: CodeRef[]
}

export interface FieldError {
  field: string
  code: string
  message: string
}

export interface ErrorBody {
  error: {
    code: string
    message: string
    fields?: FieldError[]
    details?: Record<string, unknown>
  }
}

export type IngestState = 'loading' | 'ready' | 'degraded' | 'failed'

export interface FileProgress {
  short_name: string
  url: string
  phase: 'queued' | 'downloading' | 'parsing' | 'done' | 'partial' | 'failed'
  bytes_done: number
  bytes_total: number
  error?: string
}

export interface StatusResponse {
  ingest: {
    state: IngestState
    error?: string
    index?: string
    started_at: string
    finished_at?: string
    duration_ms?: number
    files: FileProgress[]
  }
  store?: {
    built_at: string
    files: number
    provider_groups: number
    rates: number
    codes: number
  }
}

export interface PlanGroup {
  plan_id: string
  plan_id_type: string
  plan_market_type: string
  issuer_name?: string
  plan_names: string[]
}

export interface SourceFile {
  short_name: string
  url: string
  reporting_entity_name?: string
  last_updated_on?: string
  sha256?: string
  bytes: number
  fetched_at: string
  status: 'pending' | 'loaded' | 'partial' | 'failed'
  error?: string
  structure_key: number
  counts: { provider_groups: number; items: number; rates: number; linked_rates: number }
}

export interface SourcesResponse {
  index: {
    source: string
    reporting_entity_name: string
    reporting_entity_type: string
    last_updated_on: string
    version: string
  }
  structures: {
    key: number
    in_network_urls: string[]
    allowed_amount_url?: string
    loaded_files: string[]
    plans: PlanGroup[]
  }[]
  files: SourceFile[]
}
