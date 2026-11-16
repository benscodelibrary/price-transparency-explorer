package ingest

import (
	"strings"
	"time"

	"price-explorer/internal/store"
)

// Builder accumulates the normalized rows of ONE file. Builders run in
// parallel (one goroutine per file) and share nothing except the interner,
// which is safe for concurrent use. Keys inside a builder are file-local;
// Merge rebases them onto global keys.
type Builder struct {
	FileKey uint16
	strings *store.Interner

	groups   []store.ProviderGroup
	entities []store.ProviderEntity
	items    []store.BillingItem
	rates    []store.Rate // final rows; GroupKey is a local group index

	// Rates wait here until the whole file is read, because a rate may
	// reference a provider group that appears later in the file.
	pending []pendingRate

	groupBySource map[uint32]uint32 // provider_group_id -> local group index
	nextInlineID  uint32

	Header store.SourceFile // header fields parsed from the file
	Counts store.FileCounts
}

// pendingRate is a parsed rate whose provider group is known only by the id
// used in the source file. linkRatesToGroups turns it into a store.Rate.
type pendingRate struct {
	rate          store.Rate // GroupKey not set yet
	sourceGroupID uint32     // provider_group_id from the file
}

// Inline provider groups (negotiated_rates[].provider_groups) have no id in
// the source, so they get synthetic ids from a range real ids never reach.
const inlineIDBase = 1 << 31

func NewBuilder(fileKey uint16, in *store.Interner) *Builder {
	return &Builder{
		FileKey:       fileKey,
		strings:       in,
		groupBySource: make(map[uint32]uint32),
		nextInlineID:  inlineIDBase,
		Counts:        store.FileCounts{Skipped: map[string]int{}},
	}
}

func (b *Builder) skip(reason string) { b.Counts.Skipped[reason]++ }

// addGroup registers a provider group under its source id and returns the id.
func (b *Builder) addGroup(sourceID uint32, inline bool, networks []string, groups []rawProviderGroup) uint32 {
	if _, dup := b.groupBySource[sourceID]; dup {
		b.skip("duplicate_provider_group_id")
		return sourceID
	}
	local := uint32(len(b.groups))
	g := store.ProviderGroup{FileKey: b.FileKey, Inline: inline}
	for _, n := range networks {
		if n = strings.TrimSpace(n); n != "" {
			g.NetworkNames = append(g.NetworkNames, n)
		}
	}
	if !inline {
		g.SourceGroupID = sourceID
	}
	for _, pg := range groups {
		g.EntityKeys = append(g.EntityKeys, uint32(len(b.entities)))
		b.entities = append(b.entities, b.entity(local, pg))
	}
	b.groups = append(b.groups, g)
	b.groupBySource[sourceID] = local
	return sourceID
}

func (b *Builder) addInlineGroup(groups []rawProviderGroup) uint32 {
	id := b.nextInlineID
	b.nextInlineID++
	return b.addGroup(id, true, nil, groups)
}

func (b *Builder) entity(groupLocal uint32, pg rawProviderGroup) store.ProviderEntity {
	e := store.ProviderEntity{
		GroupKey:     groupLocal,
		TINType:      store.ParseTINType(pg.TIN.Type),
		BusinessName: b.strings.String(strings.TrimSpace(pg.TIN.BusinessName)),
	}
	switch e.TINType {
	case store.TINEIN:
		ein, ok := store.NormalizeEIN(pg.TIN.Value)
		if !ok {
			b.skip("invalid_ein")
		}
		e.TIN = ein
	default:
		e.TIN = strings.TrimSpace(pg.TIN.Value)
	}
	for _, raw := range pg.NPI {
		if raw == "0" {
			e.TINOnly = true // contract at TIN level; "0" is a marker, not an NPI
			continue
		}
		npi, ok := store.ParseNPIDigits(string(raw))
		if !ok {
			b.skip("invalid_npi")
			continue
		}
		e.NPIs = append(e.NPIs, npi)
	}
	return e
}

func (b *Builder) addItem(it *rawItem) {
	code := store.NormalizeCode(it.BillingCodeType, it.BillingCode)
	if code == "" {
		b.skip("missing_billing_code")
		return
	}
	itemKey := uint32(len(b.items))
	b.items = append(b.items, store.BillingItem{
		FileKey:         b.FileKey,
		Arrangement:     store.ParseArrangement(it.NegotiationArrangement),
		CodeType:        strings.ToUpper(strings.TrimSpace(it.BillingCodeType)),
		CodeTypeVersion: strings.TrimSpace(it.BillingCodeTypeVersion),
		Code:            code,
		CodeRaw:         it.BillingCode,
		Name:            strings.TrimSpace(it.Name),
		Description:     strings.TrimSpace(it.Description),
	})

	for i := range it.NegotiatedRates {
		nr := &it.NegotiatedRates[i]
		refs := make([]uint32, 0, len(nr.ProviderReferences)+1)
		for _, raw := range nr.ProviderReferences {
			id, ok := raw.Uint32()
			if !ok {
				b.skip("invalid_provider_reference")
				continue
			}
			refs = append(refs, id)
		}
		if len(nr.ProviderGroups) > 0 {
			refs = append(refs, b.addInlineGroup(nr.ProviderGroups))
		}
		if len(refs) == 0 {
			b.skip("rate_without_provider")
			continue
		}
		for j := range nr.NegotiatedPrices {
			p := &nr.NegotiatedPrices[j]
			if p.NegotiatedRate == nil {
				b.skip("missing_negotiated_rate")
				continue
			}
			r := store.Rate{
				Cents:        toCents(*p.NegotiatedRate),
				ItemKey:      itemKey,
				Expiration:   parseExpiration(p.ExpirationDate),
				ServiceCodes: b.strings.Set(compactStrings(p.ServiceCode)),
				Modifiers:    b.strings.Set(compactStrings(p.BillingCodeModifier)),
				Type:         store.ParseNegotiatedType(p.NegotiatedType),
				Class:        store.ParseBillingClass(p.BillingClass),
				Setting:      store.ParseSetting(p.Setting),
			}
			if linked, ok := store.LinkedCode(p.AdditionalInformation); ok {
				r.LinkedCode = b.strings.String(linked)
			}
			// One row per referenced group: in this data every price block has
			// exactly one reference, so this never duplicates price data.
			for _, ref := range refs {
				b.pending = append(b.pending, pendingRate{rate: r, sourceGroupID: ref})
			}
		}
	}
}

// linkRatesToGroups moves pending rates into b.rates, swapping each source
// group id for the local group index. It runs after the whole file is read,
// so correctness does not depend on whether provider_references appears
// before or after in_network. Calling it twice is harmless.
func (b *Builder) linkRatesToGroups() {
	for _, p := range b.pending {
		local, ok := b.groupBySource[p.sourceGroupID]
		if !ok {
			b.Counts.UnresolvedRefs++
			continue
		}
		p.rate.GroupKey = local
		b.rates = append(b.rates, p.rate)
	}
	b.pending = nil
	b.Counts.ProviderGroups = len(b.groups)
	b.Counts.Entities = len(b.entities)
	b.Counts.Items = len(b.items)
	b.Counts.Rates = len(b.rates)
	b.Counts.LinkedRates = 0
	for _, r := range b.rates {
		if r.LinkedCode != 0 {
			b.Counts.LinkedRates++
		}
	}
}

// Merge appends each builder's rows to s, rebasing local keys onto global
// keys. Builders are merged in a fixed order so keys are deterministic.
func Merge(s *store.Store, builders []*Builder) {
	for _, b := range builders {
		if b == nil {
			continue
		}
		groupBase := uint32(len(s.Groups))
		entityBase := uint32(len(s.Entities))
		itemBase := uint32(len(s.Items))

		for _, g := range b.groups {
			for i := range g.EntityKeys {
				g.EntityKeys[i] += entityBase
			}
			s.Groups = append(s.Groups, g)
		}
		for _, e := range b.entities {
			e.GroupKey += groupBase
			s.Entities = append(s.Entities, e)
		}
		s.Items = append(s.Items, b.items...)
		for _, r := range b.rates {
			r.ItemKey += itemBase
			r.GroupKey += groupBase
			s.Rates = append(s.Rates, r)
		}
	}
}

func toCents(v float64) int64 {
	if v >= 0 {
		return int64(v*100 + 0.5)
	}
	return int64(v*100 - 0.5)
}

var epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

func parseExpiration(s string) int32 {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	if t.Year() >= 9999 {
		return store.OpenEnded
	}
	return int32(t.Sub(epoch).Hours() / 24)
}

func compactStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, strings.ToUpper(s))
		}
	}
	return out
}
