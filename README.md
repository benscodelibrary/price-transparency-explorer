# Price Transparency Explorer

Look up what an insurer has agreed to pay providers for a procedure. Enter a billing code (for example `45380`, a colonoscopy with biopsy), optionally narrow it to one provider by NPI or EIN, and see every negotiated rate, grouped by provider.

The data comes from Fidelis Care (Centene), published under the CMS **Transparency in Coverage** rule. The app reads Fidelis's index file, uses it to find the in-network rate file, loads that file into memory, and serves a search API and a web page from a single server.

- **Backend:** Go, using only the standard library.
- **Frontend:** Vue 3 with TypeScript and Vite.

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

```bash
cd server && go run ./cmd/explorer      # terminal 1: API on port 8080
cd web && npm run dev                   # terminal 2: page on port 5173 with live reload
```

### Tests

```bash
cd server && go test ./...
cd web && npm test && npm run typecheck
```

### Settings

The most useful ones are below. Each is a flag or an environment variable, and `go run ./cmd/explorer -h` lists them all.

| Flag | Env | Default | What it does |
| --- | --- | --- | --- |
| `-sources` | `SOURCES` | `fidelis-ex` | Which rate files to load. `fidelis-es` is the second file; `all` loads both. |
| `-addr` | `ADDR` | `:8080` | Address to listen on |
| `-index-url` | `INDEX_URL` | the Centene index URL | Where to find the index file. A bundled copy is used if it can't be reached. |

> **Windows note:** if Smart App Control is on, Windows blocks freshly compiled Go programs with "An Application Control policy has blocked this file". Running the backend in WSL avoids this. The frontend is not affected.

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

1. **Find the file.** The index lists Fidelis's plans and the rate file that covers each group of plans. The server reads it (falling back to a bundled copy if the website is down) and picks the file to load.
2. **Download.** The file is saved to a local cache with a checksum, so restarts don't download it again. Failed downloads are retried, and if the network is down the cached copy is used.
3. **Read and clean up.** The file is read one entry at a time instead of all at once, so memory stays low even for very large files. Codes, identifiers and prices are normalized along the way.
4. **Store.** The cleaned data goes into in-memory tables (files, plans, provider groups, providers, billing codes, rates), plus three lookup indexes: billing code → rates, NPI → providers, EIN → providers. The finished store is swapped in all at once, so a search never sees half-loaded data.
5. **Search.** The API looks up the code, narrows to the requested providers, works out summary stats over all matches, and returns one page of results grouped by provider. Each result says which file it came from.

The server accepts requests right away. Until the data is loaded, searches return "not ready yet" and the page shows loading progress.

**API.** `GET /api/v1/rates?code=45380&npi=…&ein=…` is the search: `code` is required, and `npi` and `ein` are optional. `/api/v1/codes` powers the search-box suggestions, `/api/v1/sources` lists the loaded files, and `/api/v1/status` reports loading progress. When a search finds nothing, the response says why, and invalid input gets an error naming the field.

**Web page.** The search lives in the URL, so it can be bookmarked, shared, and survives a refresh. The page checks input before sending (NPI check digit, EIN with or without a dash, 3-digit revenue codes), explains empty results, labels hospital facility rates, highlights the providers you filtered for, and works on phones.

### What I found in the data

Two findings shaped the design most:

- **Most hospital rates are hidden from a plain code search.** They're filed under revenue codes, with the procedure code in a free-text note. A colonoscopy (`45380`) has 11 rates under its own code and 42 more found only through these notes. The app indexes both and labels these as facility rates.
- **The same facility rate is repeated many times.** One revenue code appears as 162 separate entries, with each provider's rate copied under all of them. That turned 42 real rates into 6,804 rows. The app removes exact duplicates while loading (324,923 in total).

Smaller cleanups: 3-digit revenue codes are padded to 4, provider group ids are made unique across files, an NPI of `0` is treated as "contracted at the tax-ID level", and invalid NPIs are skipped and counted.

---

## Design decisions and tradeoffs

- **In memory, no database.** The brief allows it, the data fits in about 150 MB, and searches take well under a millisecond. The main tradeoff: data reloads on every start (a few seconds) and only one server can run. Postgres with the same tables is the next step.
- **Read the file piece by piece**, so memory stays low however large the file is.
- **Separate tables linked by position.** A provider group with about 1,200 NPIs is stored once instead of on every rate, and the tables map directly onto SQL.
- **NPI and EIN lookups point to provider groups, not rates**, which keeps those indexes small.
- **Build the data fully, then swap it in**, so a search never sees half-loaded data.
- **Prices in whole cents**, for exact sorting and medians.
- **Find facility rates by procedure code, and remove duplicates.** This relies on a free-text field, so facility rates are clearly labelled.
- **No Redis, Kafka or queue.** The data is already in memory and changes monthly.

### Assumptions

- Plans are linked to rates through the index. The rate file itself doesn't name plans.
- When a facility rate's note contains something shaped like a procedure code, that's the procedure the rate applies to. The data is consistent about this, but CMS doesn't define it.
- Only in-network rate files are loaded, not allowed-amount files.
- Bundled and capitated arrangements are loaded as ordinary rates and labelled; their nested code lists aren't expanded (neither file uses them).
- Search is by exact code, with suggestions as you type. There's no free-text search on descriptions or provider names.

### Known limitations

- Only one insurer's index is set up. The design supports more, but that hasn't been tested.
- Individual NPIs are shown as numbers, without provider names or addresses.
- Codes that only appear inside facility-rate notes have no description in the source file.

## What I'd do with more time

1. **A database:** a separate loading job that writes to Postgres, so the API restarts instantly and can run on several servers.
2. **Faster restarts:** save the built store to disk and reuse it while the source file is unchanged.
3. **Scheduled refresh:** these files change monthly. Check for new versions in the background and swap in the new data without downtime.
4. **Packaging:** a Dockerfile, `docker compose up`, and CI that runs the tests and type checks.
5. **Richer search:** provider names and locations from the national NPI registry, filtering by plan, and comparing a code across insurers.
6. **Full CMS format support:** bundled and capitated codes, remote provider lists, and allowed-amount files.
7. **Page features:** CSV export, a price-distribution chart per code, and saved searches.

## Where things are

```
server/                 Go backend
  cmd/explorer/         startup and shutdown
  internal/toc/         reads the index and picks files
  internal/fetch/       downloads with caching and retries
  internal/ingest/      reads the rate file and cleans up the data
  internal/store/       in-memory tables and lookup indexes
  internal/search/      search logic
  internal/api/         HTTP handlers
web/                    Vue frontend
  src/components/       search form, provider cards, rate tables, …
  src/composables/      search, suggestions, loading status
```
