package store

import (
	"strings"
	"sync"
)

// Interner deduplicates repeated strings (business names, linked codes) and
// string sets (modifiers, place-of-service codes) into uint32 ids, so a Rate
// stores 4-byte ids instead of slice headers repeated ~540k times.
//
// Id 0 is reserved for "none". Interning is safe for concurrent use while the
// store is being built (one goroutine per file); after the build the interner
// is only read, and reads take the same lock, which is uncontended.
type Interner struct {
	mu      sync.RWMutex
	ids     map[string]uint32
	strings []string
	sets    [][]string // parallel to strings: the split form of set entries
}

const setSep = "\x1f"

func NewInterner() *Interner {
	return &Interner{ids: map[string]uint32{"": 0}, strings: []string{""}, sets: [][]string{nil}}
}

// String interns s and returns its id (0 for the empty string).
func (in *Interner) String(s string) uint32 {
	return in.intern(s, nil)
}

// Set interns an ordered string set and returns its id (0 for an empty set).
func (in *Interner) Set(values []string) uint32 {
	if len(values) == 0 {
		return 0
	}
	return in.intern(setSep+strings.Join(values, setSep), values)
}

func (in *Interner) intern(key string, set []string) uint32 {
	if key == "" {
		return 0
	}
	in.mu.RLock()
	id, ok := in.ids[key]
	in.mu.RUnlock()
	if ok {
		return id
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	if id, ok := in.ids[key]; ok {
		return id
	}
	id = uint32(len(in.strings))
	in.ids[key] = id
	in.strings = append(in.strings, key)
	if set != nil {
		set = append([]string(nil), set...) // own the backing array
	}
	in.sets = append(in.sets, set)
	return id
}

// Get returns the interned string for id.
func (in *Interner) Get(id uint32) string {
	in.mu.RLock()
	defer in.mu.RUnlock()
	if int(id) < len(in.strings) {
		return in.strings[id]
	}
	return ""
}

// GetSet returns the interned set for id. Callers must not modify it.
func (in *Interner) GetSet(id uint32) []string {
	in.mu.RLock()
	defer in.mu.RUnlock()
	if int(id) < len(in.sets) {
		return in.sets[id]
	}
	return nil
}

// Len reports how many distinct values are interned (including "none").
func (in *Interner) Len() int {
	in.mu.RLock()
	defer in.mu.RUnlock()
	return len(in.strings)
}
