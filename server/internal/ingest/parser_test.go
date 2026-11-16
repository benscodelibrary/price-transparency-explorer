package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"price-explorer/internal/store"
	tf "price-explorer/internal/testfixture"
)

func parseFixture(t *testing.T, refsFirst bool) *Builder {
	t.Helper()
	b := NewBuilder(0, store.NewInterner())
	if err := ParseInNetwork(context.Background(), strings.NewReader(tf.InNetwork(refsFirst)), b); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return b
}

// The parser must not depend on provider_references appearing first.
func TestParseOrderIndependent(t *testing.T) {
	for _, refsFirst := range []bool{true, false} {
		b := parseFixture(t, refsFirst)
		c := b.Counts
		if c.ProviderGroups != tf.ExpectGroups || c.Items != tf.ExpectItems || c.Rates != tf.ExpectRates ||
			c.LinkedRates != tf.ExpectLinkedRates || c.UnresolvedRefs != tf.ExpectUnresolved {
			t.Errorf("refsFirst=%v: counts = %+v", refsFirst, c)
		}
		if c.Skipped["malformed_in_network_item"] != 1 {
			t.Errorf("refsFirst=%v: malformed item not counted: %v", refsFirst, c.Skipped)
		}
		if b.Header.LastUpdatedOn != "2026-08-31" || b.Header.Version != "2.0.0" {
			t.Errorf("header not parsed: %+v", b.Header)
		}
	}
}

func TestParseNormalizes(t *testing.T) {
	b := parseFixture(t, true)

	var rc *store.BillingItem
	for i := range b.items {
		if b.items[i].CodeType == "RC" {
			rc = &b.items[i]
		}
	}
	if rc == nil || rc.Code != "0300" || rc.CodeRaw != "300" {
		t.Fatalf("RC item not padded: %+v", rc)
	}

	alpha := b.entities[0]
	if alpha.TIN != "113471596" || len(alpha.NPIs) != 2 || alpha.NPIs[0] != tf.NPIA {
		t.Errorf("alpha entity = %+v", alpha)
	}
	tinOnly := b.entities[1]
	if !tinOnly.TINOnly || len(tinOnly.NPIs) != 0 {
		t.Errorf("npi:[0] should mark TIN-only with no NPIs: %+v", tinOnly)
	}
	quoted := b.entities[3] // group 3 lists its NPI as a JSON string
	if len(quoted.NPIs) != 1 || quoted.NPIs[0] != tf.NPIA || quoted.TINType != store.TINNPI {
		t.Errorf("quoted NPI / npi-type TIN not handled: %+v", quoted)
	}

	var sawCents, sawOpen, sawDated bool
	for _, r := range b.rates {
		if r.Cents == 10010 {
			sawCents = true
		}
		if r.Expiration == store.OpenEnded {
			sawOpen = true
		} else if r.Expiration > 0 {
			sawDated = true
		}
	}
	if !sawCents || !sawOpen || !sawDated {
		t.Errorf("cents=%v openEnded=%v dated=%v", sawCents, sawOpen, sawDated)
	}
}

// A truncated file returns an error but keeps the rows read before it.
func TestParseTruncatedKeepsRows(t *testing.T) {
	full := tf.InNetwork(true)
	cut := full[:strings.Index(full, `"billing_code": "99215"`)]
	b := NewBuilder(0, store.NewInterner())
	if err := ParseInNetwork(context.Background(), strings.NewReader(cut), b); err == nil {
		t.Fatal("expected an error for a truncated file")
	}
	b.linkRatesToGroups()
	if len(b.rates) == 0 {
		t.Fatal("rows before the truncation were lost")
	}
}

func TestParseRejectsNonObject(t *testing.T) {
	b := NewBuilder(0, store.NewInterner())
	if err := ParseInNetwork(context.Background(), strings.NewReader(`[1,2]`), b); err == nil {
		t.Fatal("expected error")
	}
}

// Every CMS example in-network file in this repository must parse.
func TestParseCMSExamples(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "..", "examples", "in-network-rates", "*.json"))
	if len(files) == 0 {
		t.Skip("CMS examples not found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			r, err := os.Open(f)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b := NewBuilder(0, store.NewInterner())
			if err := ParseInNetwork(context.Background(), r, b); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if b.Counts.Rates == 0 {
				t.Errorf("no rates parsed: %+v", b.Counts)
			}
		})
	}
}
