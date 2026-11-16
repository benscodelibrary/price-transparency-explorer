# Price Transparency Explorer

Look up what an insurer has agreed to pay providers for a procedure. Enter a billing code (for example `45380`, a colonoscopy with biopsy), optionally narrow it to one provider by NPI or EIN, and see every negotiated rate, grouped by provider.

The data comes from Fidelis Care (Centene), published under the CMS **Transparency in Coverage** rule. The app reads Fidelis's index file, uses it to find the in-network rate file, loads that file into memory, and serves a search API and a web page from a single server.

- **Backend:** Go, using only the standard library.
- **Frontend:** Vue 3 with TypeScript and Vite.
- **Speed:** a search takes well under a millisecond on the server (0.05–0.34 ms on the real file).

> The original CMS guide that came with this repository is now in [`CMS_GUIDE.md`](CMS_GUIDE.md). Its [`schemas/`](schemas) and [`examples/`](examples) folders are used by the parser tests.

---

## Getting started

You need **Go 1.24+** and **Node 20.19+ or 22.12+**.

```bash
# 1. Build the web page
cd web
npm ci
npm run build

# 2. Start the server (it serves both the API and the page)
cd ../server
go run ./cmd/explorer

# 3. Open http://localhost:8080
```

The first start downloads the 89 MB rate file into `server/data/cache/` and indexes it. This takes a few seconds, and the page shows a progress banner while it happens. Later starts reuse the downloaded copy unless the file has changed.

**Things to try:** `99213` (office visit, about 200 providers), `45380` (colonoscopy, which includes hospital facility rates), `0450` (emergency room), or `45380` with EIN `46-3729058` to narrow it to one practice.

### Working on the code

Run the backend and frontend separately to get live reloading:

```bash
cd server && go run ./cmd/explorer      # terminal 1: API on port 8080
cd web && npm run dev                   # terminal 2: page on port 5173, forwards /api to 8080
```

### Running the tests

```bash
cd server && go test ./...              # parsing, data cleanup, search, API, downloads
cd web && npm test                      # input validation, URL handling, search requests
cd web && npm run typecheck
```

To benchmark search against the real file (after one normal start has downloaded it):

```bash
cd server && PRICE_BENCH_FILE=$PWD/data/cache/2026-08-31_centene-management-company-llc_fidelis-ex_in-network.json \
  go test ./internal/search -run '^$' -bench BenchmarkSearchReal
```

### Settings

Each setting can be passed as a flag or an environment variable. Paths are relative to `server/`.

| Flag | Env | Default | What it does |
| --- | --- | --- | --- |
| `-sources` | `SOURCES` | `fidelis-ex` | Which rate files to load. `fidelis-es` is the second file; `all` loads both. |
| `-index-url` | `INDEX_URL` | the Centene index URL | Where to find the index file |
| `-index-file` | `INDEX_FILE` | `data/index/2026-08-31_fidelis_index.json` | Local copy of the index, used if the URL can't be reached |
| `-addr` | `ADDR` | `:8080` | Address to listen on |
| `-cache-dir` | `CACHE_DIR` | `data/cache` | Where downloaded files are kept |
| `-web-dir` | `WEB_DIR` | `../web/dist` | The built web page to serve |
| `-parallel` | `PARALLEL` | `2` | How many files to load at the same time |
| `-cors-origins` | `CORS_ORIGINS` | `http://localhost:5173` | Origins allowed to call the API during development |
| `-log-level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

Loading both files (`-sources all`) gives about 540,000 prices, takes about 5 seconds, and uses about 150 MB of memory.

> **Windows note:** if Smart App Control is on, Windows blocks freshly compiled Go programs with "An Application Control policy has blocked this file". Running the backend in WSL avoids this without turning the protection off. The frontend is not affected.

---

## How it works

```
  Centene website                        Go server
 ┌───────────────┐      ┌─────────────────────────────────────────────┐
 │ index file    │ ───▶ │ 1. read the index, pick the rate file       │
 │ rate file(s)  │ ───▶ │ 2. download it (kept in a local cache)      │
 └───────────────┘      │ 3. read it piece by piece, clean it up      │
                        │ 4. store it in memory with lookup indexes   │
                        │                                             │
   Web page  ◀── JSON ─▶│ 5. answer searches from memory              │
                        └─────────────────────────────────────────────┘
```

1. **Find the file.** The index lists Fidelis's plans and the rate file that covers each group of plans. The server reads it (falling back to a bundled copy if the website is down) and picks the file to load: `fidelis-ex` (Ambetter exchange plans) by default, or `fidelis-es` (Essential Plan).
2. **Download.** The file is saved to a local cache with a checksum, so restarts don't download it again. Failed downloads are retried, and if the network is down the cached copy is used.
3. **Read and clean up.** The file is read one entry at a time instead of being loaded whole, so memory stays low even for very large files. Codes, identifiers and prices are normalized along the way (see [what the data looks like](#what-the-data-looks-like)).
4. **Store.** The cleaned data goes into in-memory tables, plus three lookup indexes: billing code → rates, NPI → providers, EIN → providers. The finished store is swapped in all at once, so a search never sees half-loaded data.
5. **Search.** The API looks up the code, narrows to the requested providers, works out summary stats over all matches, and returns one page of results grouped by provider. Each result says which file it came from.

The server starts accepting requests right away. Until the data is loaded, searches return "not ready yet" (HTTP 503) and the page shows loading progress.

### How the data is stored

The rate file is deeply nested. It's flattened into a handful of tables so each piece of information is stored once.

| Table | One row per | Notes |
| --- | --- | --- |
| Source file | loaded file | URL, date, checksum, size, row counts |
| Plan | plan in the index | Plan ids aren't unique in this index, so each plan gets its own key |
| Provider group | group in a file | Group ids are only unique within one file |
| Provider | tax ID within a group | EIN, list of NPIs, and a flag for "contracted at the tax-ID level" |
| Billing item | code in the file | Cleaned-up code, original code, description |
| Rate | price for a provider group | Price in whole cents, place of service, modifiers, expiry date |

Rows refer to each other by their position in a table rather than by copying data. This matters because one provider group can have around 1,200 NPIs: copying them onto every rate would multiply the memory used.

The NPI and EIN indexes point to **provider groups**, not individual rates, for the same reason. A search finds the code's rates first (the code is always required, so it's the narrowest starting point), then keeps the ones that belong to a matching provider group.

### API

| Endpoint | What it does |
| --- | --- |
| `GET /api/v1/rates?code=45380&npi=…&ein=…` | Search. `code` is required. `npi` and `ein` are optional and accept several values. If both are given, a provider must match both. Also accepts `match` (all, direct or facility rates), `billing_class`, `source`, `sort`, `page` and `page_size`. |
| `GET /api/v1/codes?prefix=4538` | Code suggestions for the search box |
| `GET /api/v1/sources` | Which files are loaded, with dates, checksums and the plans they cover |
| `GET /api/v1/status` | Loading progress |

Prices are returned in whole cents. When a search finds nothing, the response includes a `hint` saying why (code not found, provider not found, the provider has no rates for this code, or the filters excluded everything). Errors name the field that was wrong:

```json
{ "error": { "code": "invalid_query", "message": "…",
             "fields": [{ "field": "npi", "code": "invalid_npi", "message": "1234567890 fails the NPI check digit." }] } }
```

### Web page

The search lives in the page's URL, so a search can be bookmarked or shared, and it survives a refresh or the Back button.

- **Search box** with code suggestions as you type, plus optional NPI and EIN fields. Input is checked before sending: NPIs must pass their check digit, EINs work with or without the dash, and 3-digit revenue codes are accepted.
- **Results** start with a summary (number of rates and providers, lowest, median and highest price), followed by a card per provider with its rates. Providers that match your NPI or EIN are highlighted and listed first. Facility rates are labelled so they aren't confused with the professional fee.
- **Empty results explain themselves** and suggest a next step, such as similar codes or "search all providers".
- **Works on phones** (rate tables turn into stacked cards on narrow screens), can be used with a keyboard and screen reader, and supports dark mode.
- A **data source** section at the bottom shows exactly which file and date the results came from.

---

## What the data looks like

I profiled the real files before designing anything, and these findings shaped most of the decisions.

| What I found | Why it matters | What the app does |
| --- | --- | --- |
| **Hospital (facility) rates are filed under revenue codes, with the procedure code tucked into a free-text note** (`additional_information`). This applies to 154,000 of 180,000 prices. | Searching only the main billing code misses most facility rates. For `45380`, that's 42 of 53 rates. | Each price is findable by both its revenue code and the procedure code in the note. These rates are labelled "Facility · RC 0490" in the results. Only notes that look like a real code are used. |
| **The same facility rate is repeated many times.** Revenue code 0490 appears as 162 separate entries, each describing a different procedure, and each provider's rate is copied under all of them. | `45380` showed 6,804 facility rows that were really 42 distinct rates. Counts were inflated about 160×, and the median was distorted. | Exact duplicates are dropped while loading (324,923 of them). The misleading per-entry description isn't shown. |
| Revenue codes are written with 3 digits (`260`), but the standard form is 4 (`0260`). | Someone typing `0260` would find nothing. | Both the data and the search input are padded to 4 digits. |
| One provider group has up to about 1,200 NPIs and up to 39 tax IDs. | Storing NPIs per rate would waste a lot of memory, and provider cards could get very long. | Groups are stored once, and long lists are collapsed on the page. |
| Provider group ids are only unique within one file. | Loading both files would mix up providers. | Groups are keyed by file and id together. |
| 5 groups list their NPI as `0`, and each file has one invalid 9-digit NPI. | `0` isn't a real NPI. | `0` is treated as "contracted at the tax-ID level". Invalid NPIs are skipped and counted on the status page. |
| Plan ids repeat across plan variants, and one plan name has a typo ("Ambetter fromFidelis"). | Plan id can't be used as a unique key. | Plans get their own keys, and names are shown exactly as published. |
| Every expiry date is `9999-12-31`, and a few other fields are always the same or empty. | They don't help narrow a search. | Kept for completeness and shown as "No end date", but there are no filters for them. |
| The rate file is dated 2026-09-02, later than the index (2026-08-31). | It's unclear which is "the" data date. | Both dates are shown. |

---

## Design decisions and tradeoffs

| Decision | Why | The cost, and when I'd change it |
| --- | --- | --- |
| **Keep everything in memory, no database** | The brief allows it, the data fits in about 150 MB, searches are very fast, and the app runs with one command | Data is reloaded on every start (a few seconds), and only one server can run. Next step: Postgres or SQLite using the same tables. |
| **Read the file piece by piece** | Memory stays low no matter how large the file is | More code than loading the whole file in one call |
| **Separate tables linked by position** | Nothing is duplicated per NPI, lookups are instant, and it maps directly onto a SQL schema | Building a response takes a few extra lookups |
| **Filter by provider group, not by rate** | The NPI and EIN indexes stay small | One extra check per candidate rate |
| **Build the store fully, then swap it in** | A search never sees half-loaded data, and no locking is needed | A future live reload would briefly hold two copies in memory |
| **Prices in whole cents; short codes for fixed values** | Exact money math and small rows | Values are converted back to text when displayed |
| **Make facility rates searchable by procedure code, and remove duplicates** | Without the first, most rates are missing; without the second, counts are badly wrong | Relies on a free-text field, so every facility rate is clearly labelled |
| **Load only `fidelis-ex` by default** | Faster startup for a demo; the index still decides which file is used | `-sources all` loads both |
| **Keep the search in the URL** | Searches can be shared and bookmarked, and are easy to test | URL parameter names become something to keep stable |
| **No Redis, Kafka or queue** | The data is already in the server's memory, there's one reader and one writer, and it changes monthly | Worth adding a job queue for scheduled loading of many insurers, and a shared cache only with several servers |
| **Go standard library only** | Nothing extra to install, audit or keep updated | A little more hand-written code |

### Assumptions

- Plans are linked to rates through the index. The rate file itself doesn't name plans.
- When a facility rate's free-text note contains something shaped like a procedure code, that's the procedure the rate applies to. The data is consistent about this across both files, but CMS doesn't define it.
- Only in-network rate files are loaded. Allowed-amount files are listed but not loaded.
- Bundled and capitated arrangements are loaded as ordinary rates and labelled. Their nested code lists aren't expanded (neither file uses them).
- Provider lists stored in separate remote files are counted and skipped (neither file uses them).
- Search is by exact code, with suggestions as you type. There's no free-text search on descriptions or provider names.

### Known limitations

- Only one insurer's index is set up. The design supports more, but that hasn't been tested.
- Duplicate rates are skipped in search but still held in memory, so the original data stays traceable. Removing them would save memory.
- There's no lookup of provider names or addresses by NPI (from the national NPPES registry), so individual NPIs are shown as numbers.
- Codes that only appear inside facility-rate notes have no description in the source file. The page labels them "Facility rates only".

## What I'd do with more time

1. **A database:** a separate loading job that writes to Postgres, so the API restarts instantly and can run on several servers.
2. **Faster restarts:** save the built store to disk and reuse it while the source file is unchanged.
3. **Scheduled refresh:** these files change monthly. Check for new versions in the background and swap in the new data without downtime.
4. **Packaging:** a Dockerfile, `docker compose up`, and CI that runs the tests, type checks and linting.
5. **Richer search:** provider names and locations from NPPES, filtering by plan, and comparing a code across files and insurers.
6. **Full CMS format support:** bundled and capitated codes, remote provider lists, and allowed-amount files.
7. **Page features:** CSV export, a price-distribution chart per code, and saved searches.
8. **Monitoring:** metrics for load time and search latency, and an OpenAPI spec for the API.

## Performance

Measured on an 8-core laptop (Ryzen 7 260) with the real `fidelis-ex` file:

| | |
| --- | --- |
| First start (download and load) | 3.6 s |
| Later starts (from the cache) | 0.5–2.6 s |
| Loading both files | 4.7 s, 539,801 prices, 148 MB |
| Search `45380` (53 rates, 52 providers) | 0.05 ms |
| Search `99213` (201 rates, 192 providers) | 0.34 ms |
| Search `99213` narrowed by EIN | 0.004 ms |
| Web page download size | 49 KB compressed |

## Where things are

```
server/                 Go backend
  cmd/explorer/         startup: settings, wiring, clean shutdown
  internal/toc/         reads the index and picks files
  internal/fetch/       downloads with caching and retries
  internal/ingest/      reads the rate file and cleans up the data
  internal/store/       in-memory tables, indexes, code and ID rules
  internal/search/      search logic (no HTTP, easy to test)
  internal/api/         HTTP handlers and error responses
  internal/testfixture/ a small sample file containing every data quirk
  data/index/           local copy of the Fidelis index
web/                    Vue frontend
  src/api/              API client and response types
  src/composables/      search, suggestions, loading status
  src/components/       search form, provider cards, rate tables, …
  src/lib/              validation, formatting, URL handling
CMS_GUIDE.md, schemas/, examples/   the original CMS guide
```

If you're reading the code, start with `server/internal/store/model.go` (the tables), then `server/internal/search/search.go` (the search itself).
