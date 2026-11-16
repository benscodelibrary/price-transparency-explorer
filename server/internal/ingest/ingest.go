// Package ingest turns a table-of-contents index and its in-network files
// into a normalized store.Store.
//
// Pipeline: resolve index -> fetch each selected file -> stream-parse each file
// in its own goroutine -> merge -> build indexes. The result is returned to
// the caller, which publishes it atomically.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"price-explorer/internal/fetch"
	"price-explorer/internal/store"
	"price-explorer/internal/toc"
)

type Config struct {
	IndexURL  string   // remote index (source of truth)
	IndexFile string   // bundled local copy, used if IndexURL fails or is empty
	Sources   []string // in-network file short names to load; empty or "all" = every file
	Parallel  int      // max files processed at once
}

type Runner struct {
	Cfg     Config
	Fetcher *fetch.Fetcher
	Tracker *Tracker
	Log     *slog.Logger
}

// Run executes the full pipeline. It returns a usable store whenever at least
// one file loaded (possibly partially); the tracker records the final state.
func (r *Runner) Run(ctx context.Context) (*store.Store, error) {
	s, err := r.run(ctx)
	if err != nil {
		r.Tracker.finish(StateFailed, err.Error())
		return nil, err
	}
	state := StateReady
	for _, f := range s.Files {
		if f.Status != store.FileLoaded {
			state = StateDegraded
		}
	}
	r.Tracker.finish(state, "")
	return s, nil
}

func (r *Runner) run(ctx context.Context) (*store.Store, error) {
	idx, src, err := r.loadIndex(ctx)
	if err != nil {
		return nil, err
	}
	r.Tracker.setIndex(src)

	s := &store.Store{
		Strings: store.NewInterner(),
		Index: store.IndexInfo{
			Source: src, ReportingEntityName: idx.ReportingEntityName,
			ReportingEntityType: idx.ReportingEntityType, LastUpdatedOn: idx.LastUpdatedOn, Version: idx.Version,
		},
	}
	addStructures(s, idx)

	selected, unknown := toc.Select(idx.InNetworkFiles(), r.Cfg.Sources)
	if len(unknown) > 0 {
		r.Log.Warn("requested sources not found in index", "unknown", unknown)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no in-network files selected (sources=%v)", r.Cfg.Sources)
	}

	builders := make([]*Builder, len(selected))
	s.Files = make([]store.SourceFile, len(selected))
	for i, f := range selected {
		s.Files[i] = store.SourceFile{
			Key: uint16(i), StructureKey: uint16(f.StructureIndex), ShortName: f.ShortName,
			Kind: "in_network", URL: f.URL, Status: store.FilePending,
		}
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, r.Cfg.Parallel))
	for i, f := range selected {
		slot := r.Tracker.addFile(f.ShortName, f.URL)
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			builders[i] = r.loadFile(ctx, slot, &s.Files[i], s.Strings)
		}()
	}
	wg.Wait()

	loaded := 0
	for _, f := range s.Files {
		if f.Status == store.FileLoaded || f.Status == store.FilePartial {
			loaded++
		}
	}
	if loaded == 0 {
		return nil, errors.New("no in-network file could be loaded; see per-file errors on /status")
	}

	Merge(s, builders)
	s.BuildIndexes()
	r.Log.Info("store built", "files", loaded, "groups", len(s.Groups), "entities", len(s.Entities),
		"items", len(s.Items), "rates", len(s.Rates), "codes", len(s.Codes), "interned", s.Strings.Len())
	return s, nil
}

// loadFile fetches and parses one file. It never panics and never returns an
// error: the outcome is recorded on the SourceFile and in the tracker, and a
// builder is returned only if rows were produced.
func (r *Runner) loadFile(ctx context.Context, slot int, sf *store.SourceFile, in *store.Interner) (b *Builder) {
	log := r.Log.With("file", sf.ShortName)
	fail := func(status store.FileStatus, phase Phase, err error) {
		sf.Status, sf.Error = status, err.Error()
		r.Tracker.update(slot, func(p *FileProgress) { p.Phase, p.Error = phase, err.Error() })
		log.Error("file ingest failed", "status", status, "err", err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Error("panic during ingest", "panic", rec, "stack", string(debug.Stack()))
			fail(store.FileFailed, PhaseFailed, fmt.Errorf("internal error: %v", rec))
			b = nil
		}
	}()

	r.Tracker.update(slot, func(p *FileProgress) { p.Phase = PhaseDownloading })
	var reported int64
	res, err := r.Fetcher.Fetch(ctx, sf.URL, func(read, total int64) {
		if read-reported >= 1<<20 || read == total { // about once per MB
			reported = read
			r.Tracker.update(slot, func(p *FileProgress) { p.BytesDone, p.BytesTotal = read, total })
		}
	})
	if err != nil {
		fail(store.FileFailed, PhaseFailed, err)
		return nil
	}
	sf.SHA256, sf.Bytes, sf.FetchedAt, sf.FromCache = res.SHA256, res.Bytes, res.FetchedAt, res.FromCache

	file, err := os.Open(res.Path)
	if err != nil {
		fail(store.FileFailed, PhaseFailed, err)
		return nil
	}
	defer file.Close()

	r.Tracker.update(slot, func(p *FileProgress) { p.Phase, p.BytesDone, p.BytesTotal = PhaseParsing, 0, res.Bytes })
	reader := &progressReader{r: file, fn: func(n int64) {
		r.Tracker.update(slot, func(p *FileProgress) { p.BytesDone = n })
	}}

	b = NewBuilder(sf.Key, in)
	parseErr := ParseInNetwork(ctx, reader, b)
	sf.ReportingEntityName = b.Header.ReportingEntityName
	sf.ReportingEntityType = b.Header.ReportingEntityType
	sf.LastUpdatedOn = b.Header.LastUpdatedOn
	sf.Version = b.Header.Version
	if parseErr != nil {
		b.linkRatesToGroups() // keep everything read before the error
		sf.Counts = b.Counts
		if len(b.rates) == 0 {
			fail(store.FileFailed, PhaseFailed, parseErr)
			return nil
		}
		fail(store.FilePartial, PhasePartial, parseErr)
		return b
	}
	sf.Status, sf.Counts = store.FileLoaded, b.Counts
	r.Tracker.update(slot, func(p *FileProgress) { p.Phase, p.BytesDone = PhaseDone, res.Bytes })
	log.Info("file loaded", "groups", b.Counts.ProviderGroups, "items", b.Counts.Items,
		"rates", b.Counts.Rates, "linked_rates", b.Counts.LinkedRates, "skipped", b.Counts.Skipped)
	return b
}

// loadIndex prefers the remote index (the source of truth) and falls back to
// the bundled copy so the app still starts when the payer site is down.
func (r *Runner) loadIndex(ctx context.Context) (*toc.Index, string, error) {
	var errs []string
	if r.Cfg.IndexURL != "" {
		res, err := r.Fetcher.Fetch(ctx, r.Cfg.IndexURL, nil)
		if err == nil {
			idx, err := parseIndexFile(res.Path)
			if err == nil {
				return idx, r.Cfg.IndexURL, nil
			}
			errs = append(errs, err.Error())
		} else {
			errs = append(errs, err.Error())
		}
		r.Log.Warn("remote index unavailable, falling back to local copy", "err", errs[len(errs)-1])
	}
	if r.Cfg.IndexFile != "" {
		idx, err := parseIndexFile(r.Cfg.IndexFile)
		if err == nil {
			return idx, r.Cfg.IndexFile, nil
		}
		errs = append(errs, err.Error())
	}
	return nil, "", fmt.Errorf("could not load index: %s", strings.Join(errs, "; "))
}

func parseIndexFile(p string) (*toc.Index, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return toc.Parse(f)
}

func addStructures(s *store.Store, idx *toc.Index) {
	for i, st := range idx.ReportingStructure {
		ss := store.Structure{Key: uint16(i)}
		for _, f := range st.InNetworkFiles {
			ss.InNetworkURLs = append(ss.InNetworkURLs, f.Location)
		}
		if st.AllowedAmountFile != nil {
			ss.AllowedAmountURL = st.AllowedAmountFile.Location
		}
		for _, p := range st.ReportingPlans {
			key := uint16(len(s.Plans))
			s.Plans = append(s.Plans, store.ReportingPlan{
				Key: key, StructureKey: uint16(i), Name: strings.TrimSpace(p.PlanName), ID: p.PlanID,
				IDType: p.PlanIDType, MarketType: p.PlanMarketType, IssuerName: p.IssuerName,
			})
			ss.PlanKeys = append(ss.PlanKeys, key)
		}
		s.Structures = append(s.Structures, ss)
	}
}

// NamedReader is an already-open in-network stream.
type NamedReader struct {
	ShortName string
	R         io.Reader
}

// BuildFromReaders builds a store directly from streams, skipping the index
// and download steps. Used by tests and benchmarks.
func BuildFromReaders(ctx context.Context, files ...NamedReader) (*store.Store, error) {
	s := &store.Store{Strings: store.NewInterner()}
	builders := make([]*Builder, len(files))
	for i, f := range files {
		b := NewBuilder(uint16(i), s.Strings)
		if err := ParseInNetwork(ctx, f.R, b); err != nil {
			return nil, fmt.Errorf("%s: %w", f.ShortName, err)
		}
		builders[i] = b
		s.Files = append(s.Files, store.SourceFile{
			Key: uint16(i), ShortName: f.ShortName, Kind: "in_network", Status: store.FileLoaded,
			LastUpdatedOn: b.Header.LastUpdatedOn, Counts: b.Counts,
		})
	}
	Merge(s, builders)
	s.BuildIndexes()
	return s, nil
}

// progressReader reports bytes consumed by the parser, about once per MB.
type progressReader struct {
	r       io.Reader
	n, last int64
	fn      func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if p.n-p.last >= 1<<20 || err == io.EOF {
		p.last = p.n
		p.fn(p.n)
	}
	return n, err
}
