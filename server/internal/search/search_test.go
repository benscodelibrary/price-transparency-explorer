package search

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"price-explorer/internal/ingest"
	"price-explorer/internal/store"
	tf "price-explorer/internal/testfixture"
)

func fixtureStore(t testing.TB) *store.Store {
	t.Helper()
	s, err := ingest.BuildFromReaders(context.Background(),
		ingest.NamedReader{ShortName: "test-file", R: strings.NewReader(tf.InNetwork(false))})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func run(t *testing.T, s *store.Store, raw string) Result {
	t.Helper()
	v, _ := url.ParseQuery(raw)
	q, errs := ParseQuery(v)
	if len(errs) > 0 {
		t.Fatalf("ParseQuery(%q): %v", raw, errs)
	}
	res, errs := Run(s, q)
	if len(errs) > 0 {
		t.Fatalf("Run(%q): %v", raw, errs)
	}
	return res
}

func TestSearchDirectAndLinked(t *testing.T) {
	s := fixtureStore(t)
	res := run(t, s, "code=99213")
	sum := res.Summary
	// 3 professional rates + 1 facility rate (RC 0300 with additional_information 99213)
	if sum.Rates != 4 || sum.DirectRates != 3 || sum.LinkedRates != 1 || sum.Providers != 2 {
		t.Fatalf("summary = %+v", sum)
	}
	if sum.MinCents != 2738 || sum.MaxCents != 10010 || sum.MedianCents != 8525 {
		t.Errorf("stats = %+v (median of 2738,8050,9000,10010 is 8525)", sum)
	}
	if res.Code == nil || res.Code.Description == "" || res.Code.DirectCount != 3 || res.Code.LinkedCount != 1 {
		t.Errorf("code ref = %+v (the repeated RC item must not add a second linked rate)", res.Code)
	}
	if s.DuplicatePostings != tf.ExpectDuplicatePostings {
		t.Errorf("duplicate postings = %d, want %d", s.DuplicatePostings, tf.ExpectDuplicatePostings)
	}

	linked := run(t, s, "code=99213&match=linked")
	r := linked.Results[0].Rates[0]
	if linked.Summary.Rates != 1 || r.Match != "linked" || r.Via == nil || r.Via.Code != "0300" || r.Via.CodeType != "RC" {
		t.Errorf("linked rate = %+v via %+v", r, r.Via)
	}
	if direct := run(t, s, "code=99213&match=direct"); direct.Summary.Rates != 3 {
		t.Errorf("direct = %+v", direct.Summary)
	}
}

func TestSearchProviderFilters(t *testing.T) {
	s := fixtureStore(t)

	byEIN := run(t, s, "code=99213&ein="+tf.EINAlpha)
	if byEIN.Summary.Rates != 2 || byEIN.Results[0].Entities[0].TIN != tf.EINAlpha {
		t.Errorf("EIN filter: %+v", byEIN.Summary)
	}
	if run(t, s, "code=99213&ein=113471596").Summary.Rates != 2 {
		t.Error("EIN without dash should match")
	}

	byNPI := run(t, s, fmt.Sprintf("code=99213&npi=%d", tf.NPIC))
	if byNPI.Summary.Rates != 2 || byNPI.Summary.Providers != 1 {
		t.Errorf("NPI filter: %+v", byNPI.Summary)
	}
	var matched bool
	for _, e := range byNPI.Results[0].Entities {
		matched = matched || len(e.MatchedNPIs) == 1
		if e.TIN == tf.EINBetaTO && !e.TINOnly {
			t.Error("TIN-only entity not flagged")
		}
	}
	if !matched {
		t.Error("matched NPI not reported")
	}

	// NPI AND EIN must be in the same group: NPIA is in groups 1 and 3, the EIN in group 2.
	if res := run(t, s, fmt.Sprintf("code=99213&npi=%d&ein=%s", tf.NPIA, tf.EINBetaTO)); res.Hint != HintProviderNotFound {
		t.Errorf("disjoint NPI/EIN hint = %q", res.Hint)
	}
	// A TIN of type "npi" is searchable as an NPI. Group 3 has no 99213 rates.
	if res := run(t, s, fmt.Sprintf("code=99213&npi=%d", tf.NPID)); res.Hint != HintNoProviderRates {
		t.Errorf("npi-type TIN hint = %q", res.Hint)
	}
	// Inline provider groups are searchable too.
	if res := run(t, s, "code=99215&ein="+tf.EINInline); res.Summary.Rates != 1 {
		t.Errorf("inline group: %+v", res.Summary)
	}
}

func TestSearchNormalizationAndHints(t *testing.T) {
	s := fixtureStore(t)
	if res := run(t, s, "code=300"); res.Query.Code != "0300" || res.Summary.Rates != 2 {
		t.Errorf("3-digit RC query: code=%s %+v", res.Query.Code, res.Summary)
	}
	if res := run(t, s, "code=12345"); res.Hint != HintCodeNotFound || res.Results == nil {
		t.Errorf("unknown code: hint=%q results=%v", res.Hint, res.Results)
	}
	if res := run(t, s, "code=99215&billing_class=institutional"); res.Hint != HintFiltersExcluded {
		t.Errorf("filters hint = %q", res.Hint)
	}
	if res := run(t, s, "code=99213&billing_class=institutional"); res.Summary.Rates != 1 {
		t.Errorf("class filter = %+v", res.Summary)
	}
}

func TestSearchPaginationAndSort(t *testing.T) {
	s := fixtureStore(t)
	p1 := run(t, s, "code=99213&page_size=1")
	p2 := run(t, s, "code=99213&page_size=1&page=2")
	if p1.Page.TotalPages != 2 || len(p1.Results) != 1 || len(p2.Results) != 1 || p1.Results[0].GroupKey == p2.Results[0].GroupKey {
		t.Fatalf("pagination: %+v / %+v", p1.Page, p2.Page)
	}
	if p1.Results[0].MinCents > p2.Results[0].MinCents {
		t.Error("rate_asc not ascending by provider minimum")
	}
	if past := run(t, s, "code=99213&page=9"); len(past.Results) != 0 || past.Summary.Rates != 4 {
		t.Error("page past the end should be empty but keep totals")
	}
	desc := run(t, s, "code=99213&sort=rate_desc")
	if desc.Results[0].MaxCents != 10010 {
		t.Errorf("rate_desc first max = %d", desc.Results[0].MaxCents)
	}
	rates := desc.Results[0].Rates
	if rates[0].Cents > rates[len(rates)-1].Cents {
		t.Error("rates within a provider should be ascending")
	}
	var dated, open bool
	for _, r := range rates {
		dated = dated || (r.Expires != nil && *r.Expires == "2027-01-31")
		open = open || r.OpenEnded
	}
	if !dated || !open {
		t.Errorf("expiration rendering: dated=%v open=%v", dated, open)
	}
}

func TestParseQueryValidation(t *testing.T) {
	cases := map[string]string{
		"":                           "code",
		"code=9":                     "code",
		"code=99213&npi=123":         "npi",
		"code=99213&npi=1234567890":  "npi", // bad check digit
		"code=99213&ein=12-345":      "ein",
		"code=99213&match=foo":       "match",
		"code=99213&page_size=500":   "page_size",
		"code=99213&sort=x":          "sort",
		"code=99213&billing_class=x": "billing_class",
	}
	for raw, field := range cases {
		v, _ := url.ParseQuery(raw)
		_, errs := ParseQuery(v)
		if len(errs) == 0 || errs[0].Field != field {
			t.Errorf("ParseQuery(%q) errs = %v, want field %q", raw, errs, field)
		}
	}
	v, _ := url.ParseQuery("code=99213&npi=1234567893, 1234567893&ein=11-3471596")
	q, errs := ParseQuery(v)
	if len(errs) > 0 || len(q.NPIs) != 2 || q.EINs[0] != "113471596" {
		t.Errorf("list params: %+v %v", q, errs)
	}
}

func TestUnknownSource(t *testing.T) {
	s := fixtureStore(t)
	v, _ := url.ParseQuery("code=99213&source=nope")
	q, _ := ParseQuery(v)
	if _, errs := Run(s, q); len(errs) != 1 || errs[0].Field != "source" {
		t.Errorf("errs = %v", errs)
	}
}

// BenchmarkSearchReal runs against a real in-network file when
// PRICE_BENCH_FILE points at one, e.g. the cached fidelis-ex download.
func BenchmarkSearchReal(b *testing.B) {
	path := os.Getenv("PRICE_BENCH_FILE")
	if path == "" {
		b.Skip("set PRICE_BENCH_FILE to a real in-network file")
	}
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	s, err := ingest.BuildFromReaders(context.Background(), ingest.NamedReader{ShortName: "bench", R: f})
	f.Close()
	if err != nil {
		b.Fatal(err)
	}
	for _, raw := range []string{"code=45380", "code=99213", "code=99213&ein=11-3471596", "code=0450&sort=rate_desc"} {
		v, _ := url.ParseQuery(raw)
		q, _ := ParseQuery(v)
		b.Run(raw, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Run(s, q)
			}
		})
	}
}
