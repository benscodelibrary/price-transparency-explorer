// Package search validates rate queries and executes them against a
// store.Store. It has no HTTP dependency, so it is unit-testable and
// benchmarkable on its own.
package search

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"price-explorer/internal/store"
)

type MatchMode string

const (
	MatchAll    MatchMode = "all"
	MatchDirect MatchMode = "direct" // item billing_code == code
	MatchLinked MatchMode = "linked" // RC facility rates whose additional_information == code
)

type SortMode string

const (
	SortRateAsc  SortMode = "rate_asc"
	SortRateDesc SortMode = "rate_desc"
	SortProvider SortMode = "provider"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
	maxIdentifiers  = 20
)

type Query struct {
	Code         string             `json:"code"`
	NPIs         []uint64           `json:"npi"`
	EINs         []string           `json:"ein"`
	Match        MatchMode          `json:"match"`
	BillingClass store.BillingClass `json:"-"`
	Source       string             `json:"source,omitempty"`
	Sort         SortMode           `json:"sort"`
	Page         int                `json:"page"`
	PageSize     int                `json:"page_size"`
}

// FieldError is a validation problem tied to one request parameter, so the
// UI can show it next to the right input.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FieldErrors []FieldError

func (e FieldErrors) Error() string {
	parts := make([]string, len(e))
	for i, f := range e {
		parts[i] = f.Field + ": " + f.Message
	}
	return strings.Join(parts, "; ")
}

// ParseQuery validates and normalizes request parameters with the same rules
// the ingest applied to source data. NPI and EIN accept repeated parameters
// or comma-separated lists.
func ParseQuery(v url.Values) (Query, FieldErrors) {
	var errs FieldErrors
	add := func(field, code, msg string) { errs = append(errs, FieldError{field, code, msg}) }

	q := Query{Match: MatchAll, Sort: SortRateAsc, Page: 1, PageSize: DefaultPageSize}

	q.Code = store.NormalizeQueryCode(v.Get("code"))
	switch {
	case q.Code == "":
		add("code", "required", "A billing code is required, e.g. 99213, G0444 or 0450.")
	case len(q.Code) < 3 || len(q.Code) > 7 || !isAlnum(q.Code):
		add("code", "invalid_code", "Billing codes are 3 to 7 letters or digits, e.g. 99213, G0444 or 0450.")
	}

	for _, raw := range listParam(v, "npi") {
		digits := strings.Map(func(r rune) rune {
			if r == ' ' || r == '-' {
				return -1
			}
			return r
		}, raw)
		n, err := strconv.ParseUint(digits, 10, 64)
		switch {
		case err != nil || len(digits) != 10:
			add("npi", "invalid_npi", fmt.Sprintf("%q is not an NPI. NPIs are 10 digits.", raw))
		case !store.ValidNPI(n):
			add("npi", "invalid_npi", fmt.Sprintf("%s fails the NPI check digit. Check for a typo.", digits))
		default:
			q.NPIs = append(q.NPIs, n)
		}
	}
	if len(q.NPIs) > maxIdentifiers {
		add("npi", "too_many", fmt.Sprintf("At most %d NPIs per search.", maxIdentifiers))
	}

	for _, raw := range listParam(v, "ein") {
		ein, ok := store.NormalizeEIN(raw)
		if !ok {
			add("ein", "invalid_ein", fmt.Sprintf("%q is not an EIN. EINs are 9 digits, e.g. 12-3456789.", raw))
			continue
		}
		q.EINs = append(q.EINs, ein)
	}
	if len(q.EINs) > maxIdentifiers {
		add("ein", "too_many", fmt.Sprintf("At most %d EINs per search.", maxIdentifiers))
	}

	if m := strings.ToLower(strings.TrimSpace(v.Get("match"))); m != "" {
		switch MatchMode(m) {
		case MatchAll, MatchDirect, MatchLinked:
			q.Match = MatchMode(m)
		default:
			add("match", "invalid_match", "match must be all, direct or linked.")
		}
	}

	if c := strings.TrimSpace(v.Get("billing_class")); c != "" {
		q.BillingClass = store.ParseBillingClass(c)
		if q.BillingClass == store.ClassUnknown {
			add("billing_class", "invalid_billing_class", "billing_class must be professional, institutional or both.")
		}
	}

	q.Source = strings.TrimSpace(v.Get("source"))

	if s := strings.ToLower(strings.TrimSpace(v.Get("sort"))); s != "" {
		switch SortMode(s) {
		case SortRateAsc, SortRateDesc, SortProvider:
			q.Sort = SortMode(s)
		default:
			add("sort", "invalid_sort", "sort must be rate_asc, rate_desc or provider.")
		}
	}

	q.Page = intParam(v, "page", 1, 1, 1_000_000, "page", &errs)
	q.PageSize = intParam(v, "page_size", DefaultPageSize, 1, MaxPageSize, "page_size", &errs)
	return q, errs
}

func listParam(v url.Values, key string) []string {
	var out []string
	for _, raw := range v[key] {
		for _, part := range strings.Split(raw, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func intParam(v url.Values, key string, def, lo, hi int, field string, errs *FieldErrors) int {
	raw := strings.TrimSpace(v.Get(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		*errs = append(*errs, FieldError{field, "out_of_range", fmt.Sprintf("%s must be a whole number from %d to %d.", key, lo, hi)})
		return def
	}
	return n
}

func isAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}
