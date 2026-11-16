// Package config reads settings from flags, falling back to environment
// variables, falling back to defaults that work from the server/ directory.
package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultIndexURL = "https://www.centene.com/content/dam/centene/Centene%20Corporate/json/DOCUMENT/2026-08-31_fidelis_index.json"

type Config struct {
	Addr          string
	IndexURL      string
	IndexFile     string
	Sources       []string
	CacheDir      string
	WebDir        string
	CORSOrigins   []string
	Parallel      int
	LogLevel      string
	ShutdownGrace time.Duration
}

func Load(args []string) (Config, error) {
	fs := flag.NewFlagSet("explorer", flag.ContinueOnError)
	c := Config{}
	var sources, origins string
	fs.StringVar(&c.Addr, "addr", env("ADDR", ":8080"), "listen address")
	fs.StringVar(&c.IndexURL, "index-url", env("INDEX_URL", DefaultIndexURL), "table-of-contents index URL (empty = local file only)")
	fs.StringVar(&c.IndexFile, "index-file", env("INDEX_FILE", "data/index/2026-08-31_fidelis_index.json"), "bundled index used when the URL is unavailable")
	fs.StringVar(&sources, "sources", env("SOURCES", "fidelis-ex"), "comma-separated in-network file short names to load, or 'all'")
	fs.StringVar(&c.CacheDir, "cache-dir", env("CACHE_DIR", "data/cache"), "download cache directory")
	fs.StringVar(&c.WebDir, "web-dir", env("WEB_DIR", "../web/dist"), "built frontend to serve at / (skipped if missing)")
	fs.StringVar(&origins, "cors-origins", env("CORS_ORIGINS", "http://localhost:5173"), "comma-separated origins allowed to call the API")
	fs.IntVar(&c.Parallel, "parallel", envInt("PARALLEL", 2), "files ingested in parallel")
	fs.StringVar(&c.LogLevel, "log-level", env("LOG_LEVEL", "info"), "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	c.Sources = split(sources)
	c.CORSOrigins = split(origins)
	c.ShutdownGrace = 10 * time.Second
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
