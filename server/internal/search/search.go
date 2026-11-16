package search

import (
	"cmp"
	"slices"
	"strconv"
	"time"

	"price-explorer/internal/store"
)

const (
	maxRatesPerProvider = 100 // rates returned per provider card
	npiSampleSize       = 5   // NPIs listed per entity before "and N more"
)

// matchedRate is one rate that passed every filter.
type matchedRate struct {
	rateKey uint32 // position in Store.Rates
	linked  bool   // true when the code was found in a facility rate's additional_information
}

// providerMatches collects the matched rates of one provider group, plus
// the min/max used for sorting the result list.
type providerMatches struct {
	groupKey uint32 // position in Store.Groups
	rates    []matchedRate
	min, max int64
}

// Run executes q against s.
//
//  1. ByCode lookup: the required code is always the most selective step.
//  2. Provider filter: NPIs/EINs map to provider GROUPS (small indexes), and
//     the allowed set is a sorted slice checked by binary search.
//  3. One pass over candidate rates applies all filters and groups matches by provider.
//  4. Stats use every match; only the returned page is hydrated with names,
//     NPIs and interned sets, so response cost scales with page size.
func Run(s *store.Store, q Query) (Result, FieldErrors) {
	res := Result{
		Query:   echo(q),
		Results: []ProviderResult{},
		Page:    PageInfo{Page: q.Page, PageSize: q.PageSize},
	}

	fileFilter := -1
	if q.Source != "" {
		f, ok := s.FileByShortName(q.Source)
		if !ok {
			return res, FieldErrors{{Field: "source", Code: "unknown_source", Message: "Unknown source " + strconv.Quote(q.Source) + "."}}
		}
		fileFilter = int(f.Key)
	}
	res.Sources = sources(s, fileFilter)

	postings := s.ByCode[q.Code]
	if postings == nil {
		res.Hint = HintCodeNotFound
		res.Suggestions = s.SuggestCodes(q.Code[:min(3, len(q.Code))], 8)
		return res, nil
	}
	if ref, ok := s.Code(q.Code); ok {
		res.Code = &ref
	}

	var allowed []uint32
	filterProviders := len(q.NPIs) > 0 || len(q.EINs) > 0
	if filterProviders {
		allowed = allowedGroups(s, q)
		if len(allowed) == 0 {
			res.Hint = HintProviderNotFound
			return res, nil
		}
	}

	groups := map[uint32]*providerMatches{}
	cents := make([]int64, 0, len(postings.Direct)+len(postings.Linked))
	visit := func(rateKeys []uint32, linked bool) {
		for _, rateKey := range rateKeys {
			r := &s.Rates[rateKey]
			if filterProviders {
				if _, ok := slices.BinarySearch(allowed, r.GroupKey); !ok {
					continue
				}
			}
			if q.BillingClass != store.ClassUnknown && r.Class != q.BillingClass {
				continue
			}
			if fileFilter >= 0 && int(s.Groups[r.GroupKey].FileKey) != fileFilter {
				continue
			}
			g := groups[r.GroupKey]
			if g == nil {
				g = &providerMatches{groupKey: r.GroupKey, min: r.Cents, max: r.Cents}
				groups[r.GroupKey] = g
			}
			g.rates = append(g.rates, matchedRate{rateKey, linked})
			g.min, g.max = min(g.min, r.Cents), max(g.max, r.Cents)
			cents = append(cents, r.Cents)
			if linked {
				res.Summary.LinkedRates++
			} else {
				res.Summary.DirectRates++
			}
		}
	}
	if q.Match != MatchLinked {
		visit(postings.Direct, false)
	}
	if q.Match != MatchDirect {
		visit(postings.Linked, true)
	}

	if len(cents) == 0 {
		if filterProviders {
			res.Hint = HintNoProviderRates
		} else {
			res.Hint = HintFiltersExcluded
		}
		return res, nil
	}
	res.Summary.Rates = len(cents)
	res.Summary.Providers = len(groups)
	slices.Sort(cents)
	res.Summary.MinCents, res.Summary.MaxCents = cents[0], cents[len(cents)-1]
	res.Summary.MedianCents = median(cents)

	ordered := make([]*providerMatches, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	sortGroups(s, ordered, q.Sort)

	res.Page.TotalProviders = len(ordered)
	res.Page.TotalPages = (len(ordered) + q.PageSize - 1) / q.PageSize
	start := (q.Page - 1) * q.PageSize
	if start >= len(ordered) {
		return res, nil // valid page past the end: empty page, totals still set
	}
	for _, g := range ordered[start:min(start+q.PageSize, len(ordered))] {
		res.Results = append(res.Results, hydrate(s, g, q))
	}
	return res, nil
}

// allowedGroups returns the sorted group keys that satisfy the provider
// filter: (any listed NPI) AND (any listed EIN).
func allowedGroups(s *store.Store, q Query) []uint32 {
	union := func(lists [][]uint32) []uint32 {
		var out []uint32
		for _, l := range lists {
			out = append(out, l...)
		}
		slices.Sort(out)
		return slices.Compact(out)
	}
	var npiGroups, einGroups []uint32
	if len(q.NPIs) > 0 {
		lists := make([][]uint32, 0, len(q.NPIs))
		for _, n := range q.NPIs {
			lists = append(lists, s.ByNPI[n])
		}
		npiGroups = union(lists)
	}
	if len(q.EINs) > 0 {
		lists := make([][]uint32, 0, len(q.EINs))
		for _, e := range q.EINs {
			lists = append(lists, s.ByEIN[e])
		}
		einGroups = union(lists)
	}
	switch {
	case len(q.NPIs) > 0 && len(q.EINs) > 0:
		return intersectSorted(npiGroups, einGroups)
	case len(q.NPIs) > 0:
		return npiGroups
	default:
		return einGroups
	}
}

// intersectSorted is a linear merge of two sorted slices.
func intersectSorted(a, b []uint32) []uint32 {
	var out []uint32
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

func median(sorted []int64) int64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func sortGroups(s *store.Store, gs []*providerMatches, mode SortMode) {
	slices.SortFunc(gs, func(a, b *providerMatches) int {
		var c int
		switch mode {
		case SortRateDesc:
			c = cmp.Compare(b.max, a.max)
		case SortProvider:
			c = cmp.Compare(providerName(s, a.groupKey), providerName(s, b.groupKey))
		default:
			c = cmp.Compare(a.min, b.min)
		}
		if c == 0 {
			c = cmp.Compare(a.groupKey, b.groupKey) // deterministic pagination
		}
		return c
	})
}

func providerName(s *store.Store, groupKey uint32) string {
	for _, ek := range s.Groups[groupKey].EntityKeys {
		if n := s.Strings.Get(s.Entities[ek].BusinessName); n != "" {
			return n
		}
	}
	return "~" // unnamed groups sort last
}

func hydrate(s *store.Store, g *providerMatches, q Query) ProviderResult {
	grp := &s.Groups[g.groupKey]
	pr := ProviderResult{
		GroupKey: g.groupKey, SourceGroupID: grp.SourceGroupID, Source: s.Files[grp.FileKey].ShortName,
		NetworkNames: grp.NetworkNames, RateCount: len(g.rates), MinCents: g.min, MaxCents: g.max,
	}
	for _, ek := range grp.EntityKeys {
		pr.Entities = append(pr.Entities, entityDTO(s, &s.Entities[ek], q.NPIs))
	}

	matched := g.rates
	slices.SortFunc(matched, func(a, b matchedRate) int {
		if c := cmp.Compare(s.Rates[a.rateKey].Cents, s.Rates[b.rateKey].Cents); c != 0 {
			return c
		}
		return cmp.Compare(a.rateKey, b.rateKey)
	})
	if len(matched) > maxRatesPerProvider {
		matched, pr.RatesTruncated = matched[:maxRatesPerProvider], true
	}
	pr.Rates = make([]RateDTO, 0, len(matched))
	for _, m := range matched {
		pr.Rates = append(pr.Rates, rateDTO(s, m))
	}
	return pr
}

func entityDTO(s *store.Store, e *store.ProviderEntity, wanted []uint64) EntityDTO {
	d := EntityDTO{
		BusinessName: s.Strings.Get(e.BusinessName),
		TINType:      e.TINType.String(),
		TIN:          e.TIN,
		NPICount:     len(e.NPIs),
		NPISample:    make([]string, 0, min(npiSampleSize, len(e.NPIs))),
		TINOnly:      e.TINOnly,
	}
	if e.TINType == store.TINEIN {
		d.TIN = store.FormatEIN(e.TIN)
	}
	for _, n := range e.NPIs[:min(npiSampleSize, len(e.NPIs))] {
		d.NPISample = append(d.NPISample, strconv.FormatUint(n, 10))
	}
	for _, w := range wanted {
		if slices.Contains(e.NPIs, w) {
			d.MatchedNPIs = append(d.MatchedNPIs, strconv.FormatUint(w, 10))
		}
	}
	return d
}

func rateDTO(s *store.Store, m matchedRate) RateDTO {
	r := &s.Rates[m.rateKey]
	d := RateDTO{
		Cents:          r.Cents,
		IsPercentage:   r.Type == store.TypePercentage,
		Match:          "direct",
		NegotiatedType: r.Type.String(),
		BillingClass:   r.Class.String(),
		Setting:        r.Setting.String(),
		ServiceCodes:   orEmpty(s.Strings.GetSet(r.ServiceCodes)),
		Modifiers:      orEmpty(s.Strings.GetSet(r.Modifiers)),
	}
	if m.linked {
		item := &s.Items[r.ItemKey]
		d.Match = "linked"
		d.Via = &ViaDTO{CodeType: item.CodeType, Code: item.Code}
	}
	switch {
	case r.Expiration == store.OpenEnded:
		d.OpenEnded = true
	case r.Expiration > 0:
		exp := time.Unix(int64(r.Expiration)*86400, 0).UTC().Format("2006-01-02")
		d.Expires = &exp
	}
	return d
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func echo(q Query) QueryEcho {
	e := QueryEcho{Code: q.Code, NPI: []string{}, EIN: []string{}, Match: q.Match, Source: q.Source, Sort: q.Sort}
	for _, n := range q.NPIs {
		e.NPI = append(e.NPI, strconv.FormatUint(n, 10))
	}
	for _, ein := range q.EINs {
		e.EIN = append(e.EIN, store.FormatEIN(ein))
	}
	if q.BillingClass != store.ClassUnknown {
		e.BillingClass = q.BillingClass.String()
	}
	return e
}

func sources(s *store.Store, fileFilter int) []SourceRef {
	var out []SourceRef
	for _, f := range s.Files {
		if f.Status != store.FileLoaded && f.Status != store.FilePartial {
			continue
		}
		if fileFilter >= 0 && int(f.Key) != fileFilter {
			continue
		}
		plans := 0
		if int(f.StructureKey) < len(s.Structures) {
			plans = len(s.Structures[f.StructureKey].PlanKeys)
		}
		out = append(out, SourceRef{ShortName: f.ShortName, URL: f.URL, LastUpdatedOn: f.LastUpdatedOn, Plans: plans})
	}
	return out
}
