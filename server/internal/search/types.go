package search

import "price-explorer/internal/store"

// This file holds the JSON shapes returned by /api/v1/rates. The search
// logic that fills them lives in search.go.

// Result is the /rates response body.
type Result struct {
	Query       QueryEcho        `json:"query"`
	Code        *store.CodeRef   `json:"code"`
	Summary     Summary          `json:"summary"`
	Results     []ProviderResult `json:"results"`
	Page        PageInfo         `json:"page"`
	Sources     []SourceRef      `json:"sources"`
	Hint        string           `json:"hint,omitempty"`
	Suggestions []store.CodeRef  `json:"suggestions,omitempty"`
}

type QueryEcho struct {
	Code         string    `json:"code"`
	NPI          []string  `json:"npi"`
	EIN          []string  `json:"ein"`
	Match        MatchMode `json:"match"`
	BillingClass string    `json:"billing_class,omitempty"`
	Source       string    `json:"source,omitempty"`
	Sort         SortMode  `json:"sort"`
}

// Summary is computed over ALL matching rates, not just the returned page.
type Summary struct {
	Rates       int   `json:"rates"`
	Providers   int   `json:"providers"`
	DirectRates int   `json:"direct_rates"`
	LinkedRates int   `json:"linked_rates"`
	MinCents    int64 `json:"min_cents"`
	MedianCents int64 `json:"median_cents"`
	MaxCents    int64 `json:"max_cents"`
}

type PageInfo struct {
	Page           int `json:"page"`
	PageSize       int `json:"page_size"`
	TotalProviders int `json:"total_providers"`
	TotalPages     int `json:"total_pages"`
}

type SourceRef struct {
	ShortName     string `json:"short_name"`
	URL           string `json:"url"`
	LastUpdatedOn string `json:"last_updated_on,omitempty"`
	Plans         int    `json:"plans"`
}

type ProviderResult struct {
	GroupKey       uint32      `json:"group_key"`
	SourceGroupID  uint32      `json:"source_group_id,omitempty"`
	Source         string      `json:"source"`
	NetworkNames   []string    `json:"network_names,omitempty"`
	Entities       []EntityDTO `json:"entities"`
	RateCount      int         `json:"rate_count"`
	MinCents       int64       `json:"min_cents"`
	MaxCents       int64       `json:"max_cents"`
	Rates          []RateDTO   `json:"rates"`
	RatesTruncated bool        `json:"rates_truncated,omitempty"`
}

type EntityDTO struct {
	BusinessName string   `json:"business_name,omitempty"`
	TINType      string   `json:"tin_type"`
	TIN          string   `json:"tin"`
	NPICount     int      `json:"npi_count"`
	NPISample    []string `json:"npi_sample"`
	MatchedNPIs  []string `json:"matched_npis,omitempty"`
	TINOnly      bool     `json:"tin_only,omitempty"`
}

type RateDTO struct {
	Cents          int64    `json:"cents"`
	IsPercentage   bool     `json:"is_percentage,omitempty"`
	Match          string   `json:"match"` // direct | linked
	Via            *ViaDTO  `json:"via,omitempty"`
	NegotiatedType string   `json:"negotiated_type"`
	BillingClass   string   `json:"billing_class"`
	Setting        string   `json:"setting"`
	ServiceCodes   []string `json:"service_codes"`
	Modifiers      []string `json:"modifiers"`
	Expires        *string  `json:"expires"` // nil when open-ended or unknown
	OpenEnded      bool     `json:"open_ended,omitempty"`
}

// ViaDTO names the revenue code a linked rate is filed under, e.g. RC 0490.
// The item's description is deliberately omitted: payers file one facility
// rate under many items whose descriptions name unrelated procedures, so it
// would mislabel the rate.
type ViaDTO struct {
	CodeType string `json:"code_type"`
	Code     string `json:"code"`
}

// Hints tell the UI why a search returned nothing, so it can say something
// more useful than "no results".
const (
	HintCodeNotFound     = "code_not_found"
	HintProviderNotFound = "provider_not_found"
	HintNoProviderRates  = "no_rates_for_provider"
	HintFiltersExcluded  = "no_rates_match_filters"
)
