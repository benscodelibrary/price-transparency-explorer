package store

import (
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Store is an immutable snapshot of all loaded data plus its lookup indexes.
type Store struct {
	Index      IndexInfo
	Structures []Structure
	Plans      []ReportingPlan
	Files      []SourceFile

	Groups   []ProviderGroup  // index = group_key
	Entities []ProviderEntity // index = entity_key
	Items    []BillingItem    // index = item_key
	Rates    []Rate           // index = rate_key

	ByCode map[string]*Postings // normalized code -> rate keys
	ByNPI  map[uint64][]uint32  // npi -> sorted, distinct group keys
	ByEIN  map[string][]uint32  // 9-digit EIN -> sorted, distinct group keys
	Codes  []CodeRef            // sorted by Code, for prefix search

	Strings *Interner
	BuiltAt time.Time

	// DuplicatePostings counts rate rows not indexed because an identical
	// rate was already indexed for the same code (see BuildIndexes).
	DuplicatePostings int
}

// postingKey identifies a rate as a user sees it. Payers repeat the same
// facility rate under many items: in fidelis-ex, revenue code 0490 appears as
// 162 items, and each provider's rate for a CPT code is repeated under all of
// them. Indexing one posting per distinct key turns 6,804 rows for 45380 into
// the 42 rates that actually differ.
type postingKey struct {
	code      string
	itemCode  string
	group     uint32
	cents     int64
	exp       int32
	svc, mods uint32
	typ       NegotiatedType
	class     BillingClass
	setting   Setting
}

// BuildIndexes derives every lookup structure from the base tables. It is
// called once, after all files are merged, and before the store is published.
func (s *Store) BuildIndexes() {
	s.ByCode = make(map[string]*Postings, len(s.Items))
	postings := func(code string) *Postings {
		p := s.ByCode[code]
		if p == nil {
			p = &Postings{}
			s.ByCode[code] = p
		}
		return p
	}
	// Rate keys are visited in ascending order, so every posting list is
	// already sorted without an extra pass. Identical rates for the same code
	// are indexed once; the build-time set is discarded afterwards.
	seen := make(map[postingKey]struct{}, len(s.Rates))
	s.DuplicatePostings = 0
	add := func(code string, item *BillingItem, r *Rate) bool {
		k := postingKey{code, item.Code, r.GroupKey, r.Cents, r.Expiration, r.ServiceCodes, r.Modifiers, r.Type, r.Class, r.Setting}
		if _, dup := seen[k]; dup {
			s.DuplicatePostings++
			return false
		}
		seen[k] = struct{}{}
		return true
	}
	for key := range s.Rates {
		r := &s.Rates[key]
		item := &s.Items[r.ItemKey]
		if add(item.Code, item, r) {
			p := postings(item.Code)
			p.Direct = append(p.Direct, uint32(key))
		}
		if r.LinkedCode != 0 {
			if linked := s.Strings.Get(r.LinkedCode); linked != item.Code && add(linked, item, r) {
				lp := postings(linked)
				lp.Linked = append(lp.Linked, uint32(key))
			}
		}
	}

	s.ByNPI = make(map[uint64][]uint32)
	s.ByEIN = make(map[string][]uint32)
	for _, e := range s.Entities {
		for _, npi := range e.NPIs {
			s.ByNPI[npi] = append(s.ByNPI[npi], e.GroupKey)
		}
		switch e.TINType {
		case TINEIN:
			s.ByEIN[e.TIN] = append(s.ByEIN[e.TIN], e.GroupKey)
		case TINNPI: // a TIN of type "npi" stands in for an SSN; it is an NPI
			if npi, ok := ParseNPIDigits(e.TIN); ok {
				s.ByNPI[npi] = append(s.ByNPI[npi], e.GroupKey)
			}
		}
	}
	for k, v := range s.ByNPI {
		s.ByNPI[k] = sortedDistinct(v)
	}
	for k, v := range s.ByEIN {
		s.ByEIN[k] = sortedDistinct(v)
	}

	s.buildCodeRefs()
	s.BuiltAt = time.Now().UTC()
}

func (s *Store) buildCodeRefs() {
	s.Codes = make([]CodeRef, 0, len(s.ByCode))
	for code, p := range s.ByCode {
		ref := CodeRef{Code: code, DirectCount: len(p.Direct), LinkedCount: len(p.Linked)}
		if len(p.Direct) > 0 {
			item := s.Items[s.Rates[p.Direct[0]].ItemKey]
			ref.CodeType, ref.Description = item.CodeType, item.Description
		} else {
			ref.CodeType = InferCodeType(code)
		}
		s.Codes = append(s.Codes, ref)
	}
	sort.Slice(s.Codes, func(i, j int) bool { return s.Codes[i].Code < s.Codes[j].Code })
}

// Code returns the CodeRef for an exact normalized code.
func (s *Store) Code(code string) (CodeRef, bool) {
	i := sort.Search(len(s.Codes), func(i int) bool { return s.Codes[i].Code >= code })
	if i < len(s.Codes) && s.Codes[i].Code == code {
		return s.Codes[i], true
	}
	return CodeRef{}, false
}

// SuggestCodes returns up to limit codes starting with prefix, by binary
// search over the sorted vocabulary: O(log n + limit).
func (s *Store) SuggestCodes(prefix string, limit int) []CodeRef {
	i := sort.Search(len(s.Codes), func(i int) bool { return s.Codes[i].Code >= prefix })
	out := make([]CodeRef, 0, limit)
	for ; i < len(s.Codes) && len(out) < limit && strings.HasPrefix(s.Codes[i].Code, prefix); i++ {
		out = append(out, s.Codes[i])
	}
	return out
}

// FileByShortName finds a loaded file by its short name (e.g. "fidelis-ex").
func (s *Store) FileByShortName(name string) (SourceFile, bool) {
	for _, f := range s.Files {
		if strings.EqualFold(f.ShortName, name) {
			return f, true
		}
	}
	return SourceFile{}, false
}

func sortedDistinct(v []uint32) []uint32 {
	slices.Sort(v)
	return slices.Compact(v)
}

// Holder publishes the current Store. Readers call Load() once per request and
// get a consistent snapshot; a reload builds a new Store and swaps it in
// atomically, so reads never block and never see a half-built store.
type Holder struct {
	p atomic.Pointer[Store]
}

func (h *Holder) Load() *Store   { return h.p.Load() }
func (h *Holder) Store(s *Store) { h.p.Store(s) }
