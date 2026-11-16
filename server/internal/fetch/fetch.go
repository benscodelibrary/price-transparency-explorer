// Package fetch downloads machine-readable files to a local cache.
//
// Files are streamed to disk (never held in memory), hashed while they are
// written, and revalidated with ETag / Last-Modified so an unchanged file is
// not downloaded twice. If the network fails and a cached copy exists, the
// cached copy is used and the result says so.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Result describes the local copy of a fetched file.
type Result struct {
	Path      string
	SHA256    string
	Bytes     int64
	FetchedAt time.Time
	FromCache bool
}

// Progress is called as bytes arrive; total is -1 when unknown.
type Progress func(read, total int64)

type Fetcher struct {
	Client    *http.Client
	CacheDir  string
	Retries   int
	UserAgent string
	Log       *slog.Logger
}

func New(cacheDir string, log *slog.Logger) *Fetcher {
	return &Fetcher{
		// No overall client timeout: large files legitimately take minutes.
		// Connection-level timeouts guard against hung servers instead.
		Client: &http.Client{Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
		}},
		CacheDir:  cacheDir,
		Retries:   3,
		UserAgent: "price-explorer/1.0 (+https://github.com/CMSgov/price-transparency-guide)",
		Log:       log,
	}
}

type meta struct {
	URL          string    `json:"url"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	SHA256       string    `json:"sha256"`
	Bytes        int64     `json:"bytes"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// IsRemote reports whether location is an http(s) URL rather than a local path.
func IsRemote(location string) bool {
	u, err := url.Parse(location)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// Fetch returns a local copy of location. Local paths are used in place.
func (f *Fetcher) Fetch(ctx context.Context, location string, progress Progress) (Result, error) {
	if !IsRemote(location) {
		return f.local(location)
	}
	if err := os.MkdirAll(f.CacheDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create cache dir: %w", err)
	}
	dst := filepath.Join(f.CacheDir, cacheName(location))
	cached, hasCache := f.readMeta(dst, location)

	var lastErr error
	for attempt := 1; attempt <= max(1, f.Retries); attempt++ {
		res, err := f.download(ctx, location, dst, cached, hasCache, progress)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil || errors.Is(err, errPermanent) {
			break
		}
		backoff := time.Duration(1<<(attempt-1)) * time.Second
		f.Log.Warn("download failed, retrying", "url", location, "attempt", attempt, "backoff", backoff, "err", err)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
		}
	}
	if hasCache {
		f.Log.Warn("download failed, using cached copy", "url", location, "err", lastErr)
		return Result{Path: dst, SHA256: cached.SHA256, Bytes: cached.Bytes, FetchedAt: cached.FetchedAt, FromCache: true}, nil
	}
	return Result{}, fmt.Errorf("fetch %s: %w", location, lastErr)
}

var errPermanent = errors.New("permanent failure")

func (f *Fetcher) download(ctx context.Context, location, dst string, cached meta, hasCache bool, progress Progress) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", errPermanent, err)
	}
	req.Header.Set("User-Agent", f.UserAgent)
	if hasCache {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			req.Header.Set("If-Modified-Since", cached.LastModified)
		}
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && hasCache:
		f.Log.Info("cache is current", "url", location)
		return Result{Path: dst, SHA256: cached.SHA256, Bytes: cached.Bytes, FetchedAt: cached.FetchedAt, FromCache: true}, nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return Result{}, fmt.Errorf("%w: HTTP %s", errPermanent, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return Result{}, fmt.Errorf("HTTP %s", resp.Status)
	}

	// Write to a temp file and rename, so a crash never leaves a truncated
	// file that looks like a valid cache entry.
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.part")
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	h := sha256.New()
	counter := &countingWriter{total: resp.ContentLength, progress: progress}
	n, copyErr := io.Copy(io.MultiWriter(tmp, h, counter), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil {
		return Result{}, fmt.Errorf("download body: %w", copyErr)
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return Result{}, fmt.Errorf("short download: got %d of %d bytes", n, resp.ContentLength)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return Result{}, err
	}

	m := meta{
		URL: location, ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"),
		SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n, FetchedAt: time.Now().UTC(),
	}
	f.writeMeta(dst, m)
	return Result{Path: dst, SHA256: m.SHA256, Bytes: n, FetchedAt: m.FetchedAt}, nil
}

func (f *Fetcher) local(p string) (Result, error) {
	file, err := os.Open(p)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		return Result{}, err
	}
	st, _ := file.Stat()
	return Result{Path: p, SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n, FetchedAt: st.ModTime().UTC(), FromCache: true}, nil
}

func (f *Fetcher) readMeta(dst, location string) (meta, bool) {
	b, err := os.ReadFile(dst + ".meta.json")
	if err != nil {
		return meta{}, false
	}
	var m meta
	if json.Unmarshal(b, &m) != nil || m.URL != location {
		return meta{}, false
	}
	if st, err := os.Stat(dst); err != nil || st.Size() != m.Bytes {
		return meta{}, false
	}
	return m, true
}

func (f *Fetcher) writeMeta(dst string, m meta) {
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(dst+".meta.json", b, 0o644); err != nil {
		f.Log.Warn("could not write cache metadata", "err", err)
	}
}

// cacheName keeps the CMS file name (readable, already unique per payer/date).
func cacheName(location string) string {
	name := location
	if u, err := url.Parse(location); err == nil {
		name = path.Base(u.Path)
	}
	if unescaped, err := url.PathUnescape(name); err == nil {
		name = unescaped
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?* `, r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "download.json"
	}
	return name
}

type countingWriter struct {
	read, total int64
	progress    Progress
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.read += int64(len(p))
	if c.progress != nil {
		c.progress(c.read, c.total)
	}
	return len(p), nil
}
