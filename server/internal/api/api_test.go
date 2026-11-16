package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"price-explorer/internal/ingest"
	"price-explorer/internal/search"
	"price-explorer/internal/store"
	tf "price-explorer/internal/testfixture"
)

func newServer(t *testing.T, loaded bool) http.Handler {
	t.Helper()
	h := &store.Holder{}
	if loaded {
		s, err := ingest.BuildFromReaders(context.Background(),
			ingest.NamedReader{ShortName: "test-file", R: strings.NewReader(tf.InNetwork(true))})
		if err != nil {
			t.Fatal(err)
		}
		h.Store(s)
	}
	return (&Server{Holder: h, Tracker: ingest.NewTracker(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()
}

func get(h http.Handler, path string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNotReady(t *testing.T) {
	rec := get(newServer(t, false), "/api/v1/rates?code=99213")
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), `"not_ready"`) {
		t.Errorf("body = %s", rec.Body)
	}
	if rec := get(newServer(t, false), "/api/v1/status"); rec.Code != http.StatusOK {
		t.Errorf("status endpoint must work while loading: %d", rec.Code)
	}
}

func TestValidationErrors(t *testing.T) {
	rec := get(newServer(t, true), "/api/v1/rates?npi=123")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, f := range body.Error.Fields {
		fields[f.Field] = true
	}
	if body.Error.Code != "invalid_query" || !fields["code"] || !fields["npi"] {
		t.Errorf("error body = %+v", body.Error)
	}
}

func TestRatesOK(t *testing.T) {
	h := newServer(t, true)
	rec := get(h, "/api/v1/rates?code=99213&ein="+tf.EINAlpha)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res search.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Summary.Rates != 2 || len(res.Results) != 1 || res.Query.EIN[0] != tf.EINAlpha {
		t.Errorf("result = %+v", res.Summary)
	}
	if rec.Header().Get("Server-Timing") == "" || rec.Header().Get("X-Request-ID") == "" {
		t.Error("missing Server-Timing or X-Request-ID")
	}

	// Conditional request: the store is immutable, so the ETag is stable.
	etag := rec.Header().Get("ETag")
	if again := get(h, "/api/v1/rates?code=99213&ein="+tf.EINAlpha, "If-None-Match", etag); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Errorf("revalidation: status %d, body %d bytes", again.Code, again.Body.Len())
	}
}

func TestGzip(t *testing.T) {
	rec := get(newServer(t, true), "/api/v1/rates?code=99213", "Accept-Encoding", "gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("response not compressed")
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	var res search.Result
	if err := json.NewDecoder(zr).Decode(&res); err != nil || res.Summary.Rates != 4 {
		t.Errorf("decode gzip body: %v %+v", err, res.Summary)
	}
}

// Serving the SPA must coexist with the API routes, and unknown paths must
// fall back to index.html for client-side routing.
func TestServesSPA(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Holder: &store.Holder{}, Tracker: ingest.NewTracker(), WebDir: dir,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Handler()

	if rec := get(h, "/"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app") {
		t.Errorf("index: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/some/client/route?code=99213"); !strings.Contains(rec.Body.String(), "<html>app") {
		t.Errorf("SPA fallback: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/app.js"); !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("static asset: %d", rec.Code)
	}
	if rec := get(h, "/api/v1/status"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ingest") {
		t.Errorf("API must still route with the SPA mounted: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST / = %d", rec.Code)
	}
}

func TestCodesAndSources(t *testing.T) {
	h := newServer(t, true)
	rec := get(h, "/api/v1/codes?prefix=9921")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"99213"`) {
		t.Errorf("codes: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/codes?prefix=030"); !strings.Contains(rec.Body.String(), `"0300"`) {
		t.Errorf("3-digit RC prefix should find padded code: %s", rec.Body)
	}
	if rec := get(h, "/api/v1/sources"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "test-file") {
		t.Errorf("sources: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown route: %d", rec.Code)
	}
}
