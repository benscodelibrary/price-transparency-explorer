package ingest

import (
	"sync"
	"time"
)

// State is the overall ingest lifecycle reported on /status.
type State string

const (
	StateLoading  State = "loading"
	StateReady    State = "ready"    // every selected file loaded
	StateDegraded State = "degraded" // some files failed or are partial; search works on the rest
	StateFailed   State = "failed"   // nothing loaded
)

type Phase string

const (
	PhaseQueued      Phase = "queued"
	PhaseDownloading Phase = "downloading"
	PhaseParsing     Phase = "parsing"
	PhaseDone        Phase = "done"
	PhasePartial     Phase = "partial"
	PhaseFailed      Phase = "failed"
)

type FileProgress struct {
	ShortName  string `json:"short_name"`
	URL        string `json:"url"`
	Phase      Phase  `json:"phase"`
	BytesDone  int64  `json:"bytes_done"`
	BytesTotal int64  `json:"bytes_total"`
	Error      string `json:"error,omitempty"`
}

type Snapshot struct {
	State      State          `json:"state"`
	Error      string         `json:"error,omitempty"`
	Index      string         `json:"index,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Files      []FileProgress `json:"files"`
}

// Tracker is the mutable, concurrency-safe ingest progress shown on /status.
// It is separate from the immutable Store on purpose: progress changes many
// times per second during ingest, the Store never changes once published.
type Tracker struct {
	mu sync.RWMutex
	s  Snapshot
}

func NewTracker() *Tracker {
	return &Tracker{s: Snapshot{State: StateLoading, StartedAt: time.Now().UTC()}}
}

func (t *Tracker) Snapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := t.s
	cp.Files = append([]FileProgress(nil), t.s.Files...)
	return cp
}

func (t *Tracker) setIndex(src string) {
	t.mu.Lock()
	t.s.Index = src
	t.mu.Unlock()
}

func (t *Tracker) addFile(shortName, url string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.Files = append(t.s.Files, FileProgress{ShortName: shortName, URL: url, Phase: PhaseQueued, BytesTotal: -1})
	return len(t.s.Files) - 1
}

func (t *Tracker) update(i int, fn func(*FileProgress)) {
	t.mu.Lock()
	fn(&t.s.Files[i])
	t.mu.Unlock()
}

func (t *Tracker) finish(state State, err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	t.s.State, t.s.Error, t.s.FinishedAt = state, err, &now
	t.s.DurationMS = now.Sub(t.s.StartedAt).Milliseconds()
}
