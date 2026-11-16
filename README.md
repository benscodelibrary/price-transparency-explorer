# Price Transparency Explorer

Search negotiated in-network rates from a CMS **Transparency in Coverage** payer file by billing code, optionally narrowed to a provider by NPI or EIN.

The app reads the Fidelis (Centene) table-of-contents index, picks the in-network rate file it references, stream-parses that file into a normalized in-memory store, and serves a small JSON API and a Vue UI on one port.

- **Backend:** Go standard library only, with no third-party dependencies.
- **Frontend:** Vue 3 + TypeScript + Vite, with no UI or state library.
- **Speed:** a search takes 0.05–0.34 ms server-side (benchmarked on the real 180k-price file).

> The original CMS implementation guide that came with this repository is kept at [`CMS_GUIDE.md`](CMS_GUIDE.md). Its schemas and examples in [`schemas/`](schemas) and [`examples/`](examples) are used by the parser tests.

---

## Quick start

**Requirements:** Go 1.24 or newer, and Node 20.19+ or 22.12+.

```bash
# 1. Build the frontend once
cd web
npm ci
npm run build

# 2. Run the server (it serves the API and the built UI)
cd ../server
go run ./cmd/explorer

# 3. Open http://localhost:8080
```

On first start the server downloads the index and the 89 MB `fidelis-ex` in-network file into `server/data/cache/`, then indexes it. The UI shows a progress banner meanwhile, and searches are ready in about 4 seconds on a fast connection. Later starts revalidate the cache with ETag (HTTP 304) and skip the download.

### Development (hot reload)

```bash
cd server && go run ./cmd/explorer      # terminal 1: API on :8080
cd web && npm run dev                   # terminal 2: UI on :5173, proxies /api to :8080
```

### Tests

```bash
cd server && go test ./...              # parser, normalization, search, API, fetch/cache
cd web && npm test                      # validation, URL state, stale-request handling
cd web && npm run typecheck

# Benchmark against the real file (absolute path to the cached download)
cd server && PRICE_BENCH_FILE=$PWD/data/cache/2026-08-31_centene-management-company-llc_fidelis-ex_in-network.json \
  go test ./internal/search -run '^$' -bench BenchmarkSearchReal
```

### Configuration

Every setting has a flag and an environment variable. Defaults are relative to `server/`.

| Flag | Env | Default | Purpose |
| --- | --- | --- | --- |
| `-sources` | `SOURCES` | `fidelis-ex` | In-network files to load, by short name, or `all`. `fidelis-es` is the second file. |
| `-index-url` | `INDEX_URL` | the Centene index URL | Table-of-contents index; this is the source of truth for file URLs |
| `-index-file` | `INDEX_FILE` | `data/index/2026-08-31_fidelis_index.json` | Bundled copy, used if the URL is unreachable |
| `-addr` | `ADDR` | `:8080` | Listen address |
| `-cache-dir` | `CACHE_DIR` | `data/cache` | Download cache |
| `-web-dir` | `WEB_DIR` | `../web/dist` | Built UI to serve (skipped if missing) |
| `-parallel` | `PARALLEL` | `2` | Files ingested concurrently |
| `-cors-origins` | `CORS_ORIGINS` | `http://localhost:5173` | Origins allowed for local development |
| `-log-level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

`go run ./cmd/explorer -sources all` loads both Fidelis files: about 540k prices, 4.7 s, and a 148 MB process.

> **Windows note:** Smart App Control, when it is on, blocks newly compiled, unsigned executables, and that includes every `go build` and `go test` binary. If you hit "An Application Control policy has blocked this file", run the backend in WSL2 (`go` works unchanged there) instead of turning the protection off. The frontend tooling is unaffected.

---

## How it works

```
 centene.com                         Go binary (one process)
┌──────────────┐   ┌───────────────────────────── ingest, once at startup ─────────────────────────────┐
│ index JSON   │──▶│ toc: resolve ──▶ fetch: cache, ──▶ ingest: stream-parse ──▶ merge + build indexes │
│ in-network   │   │ index, pick      ETag, retry,      (1 goroutine per file)                          │
│ files        │   │ files            sha256                                         │ atomic swap     │
└──────────────┘   └─────────────────────────────────────────────────────────────────▼─────────────────┘
                                                                         ┌─────────────────────────┐
                    ┌──────────────┐  JSON   ┌───────────┐   lock-free   │ immutable Store         │
                    │ Vue SPA      │◀──────▶│ api       │──▶ search ───▶│ tables + ByCode/ByNPI/  │
                    │ (URL = state)│         │ net/http  │    (pure)     │ ByEIN indexes           │
                    └──────────────┘         └───────────┘               └─────────────────────────┘
```

1. **Resolve.** [`toc`](server/internal/toc) reads the index (remote first, bundled copy as fallback). It lists each `reporting_structure` with its plans and in-network file URL, then selects files by short name: `fidelis-ex` (Ambetter exchange plans) or `fidelis-es` (Essential Plan).
2. **Fetch.** [`fetch`](server/internal/fetch) streams each file to disk, hashing it as it writes. It revalidates with ETag and Last-Modified, retries with backoff, and falls back to the cached copy if the network fails.
3. **Parse.** [`ingest`](server/internal/ingest) walks the file with `encoding/json.Decoder` tokens and decodes one provider reference or one `in_network` item at a time. Memory stays flat regardless of file size. Each file gets its own goroutine and builder.
4. **Normalize and index.** Rows go into normalized tables (below). [`store`](server/internal/store) then builds the lookup indexes and is published with `atomic.Pointer`, so readers never take a lock and never see a half-built store.
5. **Serve.** [`api`](server/internal/api) validates the request and calls [`search`](server/internal/search), which is plain Go with no HTTP, so it is unit-testable and benchmarkable. It returns results grouped by provider, with stats and source traceability.

The HTTP server starts immediately. Until the data is ready, `/api/v1/rates` answers `503` with `Retry-After`, and `/api/v1/status` reports per-file progress, which the UI shows as a banner.

### Data model

The nested CMS file is flattened into normalized tables. Every key is an integer equal to the row's position in its slice, so a join is an array access.

| Table | One row per | Key fields |
| --- | --- | --- |
| `source_file` | ingested file | URL, `last_updated_on`, SHA-256, byte size, status, counts |
| `reporting_plan` | index plan entry | surrogate key (HIOS `plan_id` is **not** unique) |
| `provider_group` | (file, `provider_group_id`) | ids are only unique within a file, so the key is global |
| `provider_entity` | TIN block in a group | EIN as 9 digits, NPIs as `uint64`, `tin_only` flag |
| `billing_item` | `in_network` element | normalized code (RC padded to 4 digits), raw code, description |
| `negotiated_rate` | price × provider group | `int64` cents, enums, interned modifier and place-of-service sets; 40 bytes per row |

| Index | Shape | Why |
| --- | --- | --- |
| `ByCode` | code → `{Direct, Linked}` sorted rate keys | The code is required, so it is always the first and most selective lookup |
| `ByNPI`, `ByEIN` | identifier → sorted provider-group keys | Maps to groups, not rates: one group can hold about 1,200 NPIs, and indexing every NPI × price would explode |
| `Codes` | sorted slice | Prefix typeahead by binary search |

**Search** = look up the code's postings, build the allowed group set from NPI ∩ EIN, then make one pass over candidates checking each group by binary search. Stats are computed over all matches, and only the returned page is hydrated with names and NPIs.

### API

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/rates?code=45380&npi=…&ein=…` | Search. `code` is required. `npi` and `ein` accept repeated or comma-separated values (NPI AND EIN must match the same provider group). Optional: `match=all\|direct\|linked`, `billing_class`, `source`, `sort=rate_asc\|rate_desc\|provider`, `page`, `page_size` (≤100). |
| `GET /api/v1/codes?prefix=4538` | Typeahead with descriptions and rate counts |
| `GET /api/v1/sources` | Index metadata, loaded files (date, checksum, counts), and plans grouped by HIOS id |
| `GET /api/v1/status` | Ingest state (`loading`, `ready`, `degraded` or `failed`) with per-file progress |

Responses use integer cents. An empty result carries a `hint` that says why: `code_not_found`, `provider_not_found`, `no_rates_for_provider` or `no_rates_match_filters`. Errors share one envelope with field-level details:

```json
{ "error": { "code": "invalid_query", "message": "…",
             "fields": [{ "field": "npi", "code": "invalid_npi", "message": "1234567890 fails the NPI check digit." }] } }
```

Rate responses include `ETag` (the store is immutable, so repeat searches return `304`) and `Server-Timing`, and are gzip-compressed.

### Frontend

The URL query string is the single source of truth: its keys are the API's parameter names. Searches can therefore be shared and bookmarked, and survive refresh and Back.

- **`SearchView`** owns the state. A submit updates the URL, and the URL change runs the search.
- **Composables** handle the async work:
  - `useRateSearch` cancels a stale request with `AbortController` and keeps old results visible, dimmed, while a page or filter loads.
  - `useCodeSuggest` is a debounced, cached typeahead.
  - `useStatus` is a single shared poller that runs only while data is loading.
- **Edge cases handled:**
  - Validation mirrors the server's rules, including the NPI check digit, EINs with or without the dash, and 3-digit revenue codes. Errors appear next to the field, and the server's field errors land on the same inputs.
  - Every empty result explains why and offers the next step (suggested codes, "search all providers", "reset filters").
  - Facility rates have their own badge. Providers matching the NPI or EIN are badged and listed first. Groups with dozens of tax IDs are collapsed.
  - A "Data source" footer lists the index, each file with its date, checksum and status, and the plans covered.
- **Layout and accessibility:** responsive, checked at 390 px (rate tables become stacked cards); keyboard-accessible combobox; screen-reader announcements; dark mode.

---

## What the data actually looks like

These findings come from profiling the real files, and they drove most of the design.

| Finding | Impact | Handling |
| --- | --- | --- |
| **Revenue-code (RC) prices carry the CPT/HCPCS code in `additional_information`** (154k of 180k prices in `fidelis-ex`) | Searching `billing_code` alone misses most facility rates: 45380 has 11 direct rates but 42 facility rates | Each price is indexed under its item code *and* its linked code, and labelled "Facility · RC 0490". Only values shaped like a code are indexed, because CMS defines the field as free text. |
| **The same facility rate is repeated under many items.** RC 0490 appears as 162 items, each described as a different procedure, and every provider's rate is repeated under all of them. | 45380 showed 6,804 facility rows that are really 42 distinct rates; counts were inflated about 160× and medians were skewed | Index-time deduplication on (code, provider group, price, class, setting, place of service, modifiers, expiration, revenue code) skipped 324,923 duplicate postings. The misleading item description is not shown on facility rates. |
| RC codes are 3 digits (`"260"`); the standard form is 4 (`"0260"`) | A user typing `0260` finds nothing | Both data and input are normalized to 4 digits |
| One provider group has up to about 1,200 NPIs and up to 39 tax IDs | Per-NPI rows would multiply memory; the UI cards become very long | Groups are stored once; the cards collapse long lists |
| `provider_group_id` is unique only within a file | Collisions when loading both files | Keyed globally by (file, id) |
| 5 groups have `npi: [0]`, and each file has one 9-digit NPI | A bogus "0" NPI; an invalid identifier | `0` becomes a "TIN-level contract" flag; the invalid NPI is skipped and counted on `/status` |
| `plan_id` repeats across cost-sharing variants (94 entries, 24 ids); one name has a typo ("Ambetter fromFidelis") | Not usable as a key | Surrogate keys; names grouped by HIOS id and kept verbatim |
| Every `expiration_date` is `9999-12-31`, every `billing_code_type_version` is `"2022"`, every `network_name` is empty | Noise | Stored for traceability; shown as "No end date"; no filters built for them |
| The rate file's `last_updated_on` (2026-09-02) is later than the index's (2026-08-31) | Which date is "the data date"? | Both are shown, per file |

---

## Design decisions and tradeoffs

| Decision | Why | Tradeoff / when to revisit |
| --- | --- | --- |
| **In-memory store, no database** | Allowed by the brief; about 540k rows fit in about 150 MB; sub-millisecond reads; one command to run | Re-parse on cold start (0.5–3 s here); single instance. Next step: SQLite or Postgres using the same tables. |
| **Streaming parse** (`json.Decoder` tokens) | Flat memory on files of 100+ MB | More code than `json.Unmarshal`. A faster decoder is an option if parsing becomes the bottleneck. |
| **Normalized tables + integer keys** | No duplication per NPI; O(1) joins; maps directly to SQL | Hydrating a page costs a few extra lookups |
| **Filter by provider group, not by rate** | Small indexes; one NPI hit covers every price for that group | One membership check per candidate |
| **Immutable snapshot + `atomic.Pointer`** | Lock-free, consistent reads; a reload can build a new store and swap it in | Two copies in memory during a reload |
| **Integer cents, `uint8` enums, interned sets** | Exact money; a 40-byte rate row | Formatting happens at the edges |
| **Index the linked RC codes, and deduplicate** | Correct coverage *and* correct counts; both are needed | Relies on a free-text field; every match type is labelled in the UI |
| **Load `fidelis-ex` only by default** | Faster demo start; the index still drives selection | `-sources all` loads both |
| **URL as UI state; one stateful view** | Shareable, testable, survives refresh | Query parameter names are a public contract |
| **No Redis, Kafka or queue** | The data is already in process memory, with one producer and one consumer, and it changes monthly | Add a job queue for multi-payer scheduled ingestion, and a shared cache only with several API replicas |
| **Standard library only (Go)** | Nothing to audit or update; easy to read | A little more code (router patterns, gzip middleware) |

### Assumptions

- Plans are linked to rates through the index's `reporting_structure`; the in-network file itself does not name plans.
- `additional_information` on an RC price that is shaped like a CPT/HCPCS code is the procedure the facility rate applies to. This is inferred from the data and consistent across both files, but it is not defined by CMS.
- Only in-network files are ingested; allowed-amount files are listed under `/sources` but not loaded.
- Bundle and capitation arrangements are ingested as ordinary rates and labelled; their nested `bundled_codes` and `covered_services` are not expanded (neither file uses them).
- Remote `provider_references` (with a `location` URL) are counted and skipped; neither file uses them.
- Code search is exact (plus a prefix typeahead). There is no full-text search on descriptions or provider names.

### Known limitations

- Only one payer index is configured; the design supports more, but none were tested.
- Duplicate rate rows are skipped by the index but still kept in memory, for traceability. Compacting them would save memory.
- No provider name, address or specialty lookup (NPPES), so individual NPIs are shown as numbers.
- Codes that appear only as linked codes have no description in the source file, so the UI labels them "Facility rates only".

## What I would do with more time

1. **Persistence:** a separate ingest job writing to Postgres (same schema), so the API restarts instantly and scales horizontally.
2. **Snapshot cache:** serialize the built store, keyed by file SHA-256, to skip re-parsing on restart.
3. **Scheduled refresh:** CMS files change monthly. Recheck the index and ETags, rebuild in the background, and swap atomically.
4. **Packaging:** a Dockerfile and `docker compose up`, plus CI (go test, vitest, typecheck, lint).
5. **Richer search:** provider names and locations via NPPES, plan-level filtering, and comparing a code across files and payers.
6. **Full schema coverage:** bundled and capitation codes, remote provider references, and allowed-amount files.
7. **UX:** CSV export, a price-distribution chart per code, and saved searches.
8. **Observability:** Prometheus metrics (ingest duration, query latency histogram) and an OpenAPI spec.

## Measured performance

Measured on an 8-core laptop (Ryzen 7 260) against the real `fidelis-ex` file:

| | |
| --- | --- |
| Ingest `fidelis-ex`, cold (download + parse) | 3.6 s |
| Ingest `fidelis-ex`, from cache | 0.5–2.6 s (Windows / WSL file system) |
| Ingest both files | 4.7 s; 539,801 prices; 148 MB process |
| Search `45380` (53 rates, 52 providers) | 0.05 ms |
| Search `99213` (201 rates, 192 providers) | 0.34 ms |
| Search `99213` + EIN filter | 0.004 ms |
| Frontend bundle | 49 KB gzipped |

## Repository layout

```
server/                 Go backend (module price-explorer, stdlib only)
  cmd/explorer/         main: config, wiring, graceful shutdown
  internal/toc/         index parsing and file selection
  internal/fetch/       cached, retrying downloads
  internal/ingest/      streaming parser, normalization, builder, progress tracker
  internal/store/       tables, indexes, interner, code and identifier rules
  internal/search/      query validation and search (no HTTP)
  internal/api/         handlers, middleware, error envelope
  internal/testfixture/ shared fixture covering every data quirk
  data/index/           bundled copy of the Fidelis index
web/                    Vue 3 + TypeScript frontend
  src/api/              typed client mirroring the Go contract
  src/composables/      useRateSearch, useCodeSuggest, useStatus
  src/components/       SearchForm, CodeTypeahead, ProviderCard, RateTable, …
  src/lib/              validation, formatting, URL ↔ form mapping
CMS_GUIDE.md, schemas/, examples/   original CMS implementation guide
```

## Use of AI

This project was built with the help of Claude Code (Anthropic), used for data profiling, planning, implementation and testing. Design decisions, data findings and every number in this README were checked against the real files and the running application.
