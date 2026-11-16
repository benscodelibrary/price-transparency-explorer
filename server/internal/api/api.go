// Package api exposes the store over a small, versioned, read-only JSON API.
package api

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"price-explorer/internal/ingest"
	"price-explorer/internal/search"
	"price-explorer/internal/store"
)

type Server struct {
	Holder      *store.Holder
	Tracker     *ingest.Tracker
	Log         *slog.Logger
	WebDir      string   // built SPA to serve at "/", optional
	CORSOrigins []string // allowed origins for local frontend development
}

// Handler returns the full HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/rates", s.ready(s.handleRates))
	mux.HandleFunc("GET /api/v1/codes", s.ready(s.handleCodes))
	mux.HandleFunc("GET /api/v1/sources", s.ready(s.handleSources))
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "No API route "+r.Method+" "+r.URL.Path+".", nil)
	})
	if s.WebDir != "" {
		if st, err := os.Stat(s.WebDir); err == nil && st.IsDir() {
			// "/" without a method: "GET /" would conflict with "/api/" in Go's
			// router (neither pattern is strictly more specific).
			mux.Handle("/", spaHandler(s.WebDir))
		}
	}
	return chain(mux, recoverer(s.Log), requestID, logger(s.Log), cors(s.CORSOrigins), gzipper)
}

type storeHandler func(w http.ResponseWriter, r *http.Request, st *store.Store)

// ready loads the current snapshot once per request, or answers 503 while
// ingest is still running (or failed) so clients know to retry.
func (s *Server) ready(h storeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := s.Holder.Load()
		if st == nil {
			snap := s.Tracker.Snapshot()
			msg := "Rate data is still loading. Try again in a few seconds."
			code := "not_ready"
			if snap.State == ingest.StateFailed {
				msg, code = "Rate data failed to load: "+snap.Error, "ingest_failed"
			} else {
				w.Header().Set("Retry-After", "5")
			}
			writeError(w, http.StatusServiceUnavailable, code, msg, map[string]any{"state": snap.State})
			return
		}
		h(w, r, st)
	}
}

func (s *Server) handleRates(w http.ResponseWriter, r *http.Request, st *store.Store) {
	q, errs := search.ParseQuery(r.URL.Query())
	if len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	if notModified(w, r, st) {
		return
	}
	start := time.Now()
	res, errs := search.Run(st, q)
	if len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	w.Header().Set("Server-Timing", fmt.Sprintf("search;dur=%.3f", float64(time.Since(start).Microseconds())/1000))
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCodes(w http.ResponseWriter, r *http.Request, st *store.Store) {
	prefix := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("prefix")))
	if prefix == "" {
		writeFieldErrors(w, search.FieldErrors{{Field: "prefix", Code: "required", Message: "prefix is required."}})
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			writeFieldErrors(w, search.FieldErrors{{Field: "limit", Code: "out_of_range", Message: "limit must be 1 to 50."}})
			return
		}
		limit = n
	}
	if notModified(w, r, st) {
		return
	}
	codes := st.SuggestCodes(prefix, limit)
	// A user typing a 3-digit revenue code ("045") means the padded form.
	if len(codes) < limit && len(prefix) <= 3 && isDigits(prefix) {
		codes = append(codes, st.SuggestCodes("0"+prefix, limit-len(codes))...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"prefix": prefix, "codes": codes})
}

type planGroup struct {
	PlanID     string   `json:"plan_id"`
	PlanIDType string   `json:"plan_id_type"`
	Market     string   `json:"plan_market_type"`
	Issuer     string   `json:"issuer_name,omitempty"`
	PlanNames  []string `json:"plan_names"`
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request, st *store.Store) {
	if notModified(w, r, st) {
		return
	}
	type structureDTO struct {
		Key              uint16      `json:"key"`
		InNetworkURLs    []string    `json:"in_network_urls"`
		AllowedAmountURL string      `json:"allowed_amount_url,omitempty"`
		Loaded           []string    `json:"loaded_files"`
		Plans            []planGroup `json:"plans"`
	}
	out := make([]structureDTO, 0, len(st.Structures))
	for _, ss := range st.Structures {
		d := structureDTO{Key: ss.Key, InNetworkURLs: ss.InNetworkURLs, AllowedAmountURL: ss.AllowedAmountURL, Loaded: []string{}}
		for _, f := range st.Files {
			if f.StructureKey == ss.Key && (f.Status == store.FileLoaded || f.Status == store.FilePartial) {
				d.Loaded = append(d.Loaded, f.ShortName)
			}
		}
		// plan_id is shared by cost-sharing variants, so group names under it.
		byID := map[string]int{}
		for _, pk := range ss.PlanKeys {
			p := st.Plans[pk]
			i, ok := byID[p.ID]
			if !ok {
				i = len(d.Plans)
				byID[p.ID] = i
				d.Plans = append(d.Plans, planGroup{PlanID: p.ID, PlanIDType: p.IDType, Market: p.MarketType, Issuer: p.IssuerName})
			}
			d.Plans[i].PlanNames = append(d.Plans[i].PlanNames, p.Name)
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"index": st.Index, "structures": out, "files": st.Files})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	body := map[string]any{"ingest": s.Tracker.Snapshot()}
	if st := s.Holder.Load(); st != nil {
		body["store"] = map[string]any{
			"built_at": st.BuiltAt, "files": len(st.Files), "provider_groups": len(st.Groups),
			"provider_entities": len(st.Entities), "items": len(st.Items), "rates": len(st.Rates),
			"codes": len(st.Codes), "npis": len(st.ByNPI), "eins": len(st.ByEIN),
			"duplicate_postings_skipped": st.DuplicatePostings,
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, body)
}

// notModified implements ETag revalidation. The store is immutable, so a
// response is fully determined by (store build time, path, query).
func notModified(w http.ResponseWriter, r *http.Request, st *store.Store) bool {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d|%s|%s", st.BuiltAt.UnixNano(), r.URL.Path, r.URL.RawQuery)
	etag := fmt.Sprintf(`W/"%x"`, h.Sum64())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=60")
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// spaHandler serves static files and falls back to index.html for client-side routes.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

type errorBody struct {
	Error struct {
		Code    string              `json:"code"`
		Message string              `json:"message"`
		Fields  []search.FieldError `json:"fields,omitempty"`
		Details map[string]any      `json:"details,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string, details map[string]any) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Details = code, msg, details
	writeJSON(w, status, b)
}

func writeFieldErrors(w http.ResponseWriter, errs search.FieldErrors) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Fields = "invalid_query", errs.Error(), errs
	writeJSON(w, http.StatusBadRequest, b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // the client went away; nothing useful to do
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
