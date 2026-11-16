package toc

import (
	"os"
	"testing"
)

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"https://www.centene.com/content/dam/centene/Centene%20Corporate/json/DOCUMENT/2026-08-31_centene-management-company-llc_fidelis-ex_in-network.json": "fidelis-ex",
		"data/2026-08-31_acme_gold-plan_in-network.json": "gold-plan",
		"plain.json": "plain",
	}
	for in, want := range cases {
		if got := ShortName(in); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The bundled Fidelis index lists two in-network files across two structures.
func TestFidelisIndex(t *testing.T) {
	f, err := os.Open("../../data/index/2026-08-31_fidelis_index.json")
	if err != nil {
		t.Skip("bundled index not found")
	}
	defer f.Close()
	idx, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	files := idx.InNetworkFiles()
	if len(files) != 2 || files[0].ShortName != "fidelis-ex" || files[1].ShortName != "fidelis-es" {
		t.Fatalf("files = %+v", files)
	}
	plans := 0
	for _, s := range idx.ReportingStructure {
		plans += len(s.ReportingPlans)
	}
	if plans != 94 {
		t.Errorf("plans = %d, want 94", plans)
	}
	sel, unknown := Select(files, []string{"FIDELIS-EX", "nope"})
	if len(sel) != 1 || len(unknown) != 1 {
		t.Errorf("Select = %v, unknown %v", sel, unknown)
	}
	if all, _ := Select(files, []string{"all"}); len(all) != 2 {
		t.Error("all should select every file")
	}
}
