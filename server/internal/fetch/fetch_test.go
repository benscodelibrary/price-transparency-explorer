package fetch

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestFetchCachesAndRevalidates(t *testing.T) {
	var hits, full atomic.Int32
	down := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if down.Load() {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full.Add(1)
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	f := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.Retries = 2
	url := srv.URL + "/2026-08-31_x_plan_in-network.json"

	first, err := f.Fetch(context.Background(), url, nil)
	if err != nil || first.FromCache || first.Bytes != 11 || first.SHA256 == "" {
		t.Fatalf("first fetch: %+v %v", first, err)
	}
	if b, _ := os.ReadFile(first.Path); string(b) != `{"ok":true}` {
		t.Fatalf("cached body = %q", b)
	}

	second, err := f.Fetch(context.Background(), url, nil)
	if err != nil || !second.FromCache || second.SHA256 != first.SHA256 || full.Load() != 1 {
		t.Fatalf("revalidation should hit 304: %+v %v (full downloads %d)", second, err, full.Load())
	}

	down.Store(true)
	f.Retries = 1 // skip backoff sleeps; retry behaviour is not under test here
	third, err := f.Fetch(context.Background(), url, nil)
	if err != nil || !third.FromCache {
		t.Fatalf("server down should fall back to cache: %+v %v", third, err)
	}
}

func TestFetchFailsWithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	f := New(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := f.Fetch(context.Background(), srv.URL+"/missing.json", nil); err == nil {
		t.Fatal("expected error")
	}
}
