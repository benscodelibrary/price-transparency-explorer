// Package store holds the normalized, read-only, in-memory representation of
// the ingested Transparency in Coverage data.
//
// # How to read this package
//
// Tables are slices, and a row's key is its position in the slice. Rows point
// at each other with these keys instead of Go pointers:
//
//	rate  := s.Rates[rateKey]
//	group := s.Groups[rate.GroupKey]  // the provider group the rate belongs to
//	item  := s.Items[rate.ItemKey]    // the billing code it was filed under
//	name  := s.Strings.Get(s.Entities[group.EntityKeys[0]].BusinessName)
//
// Two space-saving conventions to know:
//   - Repeated text (business names, service codes, modifiers) is stored once
//     in s.Strings; rows hold a uint32 id. Use Strings.Get / Strings.GetSet.
//   - Small fixed vocabularies (billing class, negotiated type, setting) are
//     one-byte enums from enums.go; call .String() for the original text.
//
// Prices are int64 cents and dates are days since 1970, so comparisons and
// sorting never touch floats or strings.
//
// Why keys instead of pointers: about 540k rates fit in roughly 40 bytes
// each, the GC has almost nothing to scan, and lookups are a single index.
//
// A Store is built once by the ingest package and is never changed afterwards,
// so any number of goroutines may read it without locking.
package store

import (
	"math"
	"time"
)

// NoKey marks an absent optional key.
const NoKey = math.MaxUint32

// OpenEnded is the Expiration value for "9999-12-31" style evergreen
// contracts. Unknown or unparseable dates are stored as 0.
const OpenEnded int32 = math.MaxInt32

// Structure mirrors one table-of-contents reporting_structure: a set of plans
// that share the same in-network (and allowed-amount) files.
type Structure struct {
	Key              uint16   `json:"key"`
	PlanKeys         []uint16 `json:"plan_keys"`
	InNetworkURLs    []string `json:"in_network_urls"`
	AllowedAmountURL string   `json:"allowed_amount_url,omitempty"`
}

// ReportingPlan is one plan entry from the index. plan_id is NOT unique
// (cost-sharing variants share a HIOS id), so plans get a surrogate key.
type ReportingPlan struct {
	Key          uint16 `json:"key"`
	StructureKey uint16 `json:"structure_key"`
	Name         string `json:"plan_name"`
	ID           string `json:"plan_id"`
	IDType       string `json:"plan_id_type"`
	MarketType   string `json:"plan_market_type"`
	IssuerName   string `json:"issuer_name,omitempty"`
}

// FileStatus is the ingest outcome of one source file.
type FileStatus string

const (
	FilePending FileStatus = "pending"
	FileLoaded  FileStatus = "loaded"
	FilePartial FileStatus = "partial" // parse stopped midway; rows before the error are kept
	FileFailed  FileStatus = "failed"
)

// FileCounts records what an ingest produced, for /status and the README.
type FileCounts struct {
	ProviderGroups int            `json:"provider_groups"`
	Entities       int            `json:"provider_entities"`
	Items          int            `json:"items"`
	Rates          int            `json:"rates"`
	LinkedRates    int            `json:"linked_rates"`
	UnresolvedRefs int            `json:"unresolved_provider_refs"`
	Skipped        map[string]int `json:"skipped,omitempty"`
}

// SourceFile is one ingested machine-readable file, with enough metadata to
// trace every result back to exactly what was loaded.
type SourceFile struct {
	Key                 uint16     `json:"key"`
	StructureKey        uint16     `json:"structure_key"`
	ShortName           string     `json:"short_name"`
	Kind                string     `json:"kind"`
	URL                 string     `json:"url"`
	ReportingEntityName string     `json:"reporting_entity_name,omitempty"`
	ReportingEntityType string     `json:"reporting_entity_type,omitempty"`
	Version             string     `json:"version,omitempty"`
	LastUpdatedOn       string     `json:"last_updated_on,omitempty"`
	SHA256              string     `json:"sha256,omitempty"`
	Bytes               int64      `json:"bytes"`
	FetchedAt           time.Time  `json:"fetched_at"`
	FromCache           bool       `json:"from_cache"`
	Status              FileStatus `json:"status"`
	Error               string     `json:"error,omitempty"`
	Counts              FileCounts `json:"counts"`
}

// ProviderGroup is one provider_references entry, keyed globally because
// provider_group_id is only unique within a single file.
type ProviderGroup struct {
	FileKey       uint16
	SourceGroupID uint32 // original provider_group_id; 0 for inline groups
	Inline        bool   // came from negotiated_rates[].provider_groups, not a reference
	NetworkNames  []string
	EntityKeys    []uint32
}

// ProviderEntity is one TIN block inside a provider group.
type ProviderEntity struct {
	GroupKey     uint32
	TINType      TINType
	TIN          string // digits only
	BusinessName uint32 // interned string id
	NPIs         []uint64
	TINOnly      bool // source reported npi: [0] (contracted at TIN level only)
}

// BillingItem is one in_network element.
type BillingItem struct {
	FileKey         uint16
	Arrangement     Arrangement
	CodeType        string
	CodeTypeVersion string
	Code            string // normalized (RC padded to 4 digits, upper case)
	CodeRaw         string
	Name            string
	Description     string
}

// Rate is the hot table: one row per (negotiated_price x provider group).
// Fields are ordered and sized to keep the struct at 40 bytes.
type Rate struct {
	Cents        int64  // negotiated_rate in integer cents (or percent x 100)
	ItemKey      uint32 // -> BillingItem
	GroupKey     uint32 // -> ProviderGroup
	Expiration   int32  // days since Unix epoch; OpenEnded or 0 (unknown)
	LinkedCode   uint32 // interned additional_information code; 0 = none
	ServiceCodes uint32 // interned set id; 0 = none
	Modifiers    uint32 // interned set id; 0 = none
	Type         NegotiatedType
	Class        BillingClass
	Setting      Setting
}

// Postings are the inverted-index entries for one billing code.
type Postings struct {
	Direct []uint32 // rates whose item code == code
	Linked []uint32 // rates whose additional_information == code (RC facility rates)
}

// CodeRef is the search vocabulary used for typeahead and code headers.
type CodeRef struct {
	Code        string `json:"code"`
	CodeType    string `json:"type"`
	Description string `json:"description,omitempty"`
	DirectCount int    `json:"direct_count"`
	LinkedCount int    `json:"linked_count"`
}

// IndexInfo describes the table-of-contents file the store was built from.
type IndexInfo struct {
	Source              string `json:"source"` // URL or local path actually used
	ReportingEntityName string `json:"reporting_entity_name"`
	ReportingEntityType string `json:"reporting_entity_type"`
	LastUpdatedOn       string `json:"last_updated_on"`
	Version             string `json:"version"`
}
