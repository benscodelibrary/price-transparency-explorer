package store

import "testing"

func TestNormalizeCode(t *testing.T) {
	cases := []struct{ typ, raw, want string }{
		{"RC", "260", "0260"},
		{"RC", "45", "0045"},
		{"RC", "0450", "0450"},
		{"rc", " 300 ", "0300"},
		{"CPT", "99213", "99213"},
		{"HCPCS", "t4539", "T4539"},
		{"CPT", "450", "450"}, // only RC codes are padded at ingest
	}
	for _, c := range cases {
		if got := NormalizeCode(c.typ, c.raw); got != c.want {
			t.Errorf("NormalizeCode(%q, %q) = %q, want %q", c.typ, c.raw, got, c.want)
		}
	}
}

func TestNormalizeQueryCode(t *testing.T) {
	cases := map[string]string{"260": "0260", " 99213 ": "99213", "g0444": "G0444", "99 213": "99213", "0450": "0450"}
	for in, want := range cases {
		if got := NormalizeQueryCode(in); got != want {
			t.Errorf("NormalizeQueryCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinkedCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"80053", "80053", true},
		{" g0444 ", "G0444", true},
		{"0001U", "0001U", true},
		{"see contract", "", false},
		{"8005", "", false},
		{"800531", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := LinkedCode(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("LinkedCode(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEIN(t *testing.T) {
	if got, ok := NormalizeEIN("11-3471596"); !ok || got != "113471596" {
		t.Errorf("NormalizeEIN dashed = %q,%v", got, ok)
	}
	if _, ok := NormalizeEIN("11-347159"); ok {
		t.Error("8-digit EIN accepted")
	}
	if got := FormatEIN("113471596"); got != "11-3471596" {
		t.Errorf("FormatEIN = %q", got)
	}
}

func TestNPI(t *testing.T) {
	if !ValidNPI(1234567893) { // the CMS documentation's example NPI
		t.Error("1234567893 should be valid")
	}
	for _, n := range []uint64{1234567890, 123456789, 12345678930, 0} {
		if ValidNPI(n) {
			t.Errorf("ValidNPI(%d) = true", n)
		}
	}
	if _, ok := ParseNPIDigits("0123456789"); ok {
		t.Error("NPI starting with 0 accepted")
	}
	if n, ok := ParseNPIDigits("1396747861"); !ok || n != 1396747861 {
		t.Errorf("ParseNPIDigits = %d,%v", n, ok)
	}
}

func TestInterner(t *testing.T) {
	in := NewInterner()
	a := in.String("Alpha")
	if in.String("Alpha") != a || in.Get(a) != "Alpha" || in.String("") != 0 {
		t.Fatal("string interning broken")
	}
	s := in.Set([]string{"SH", "SJ"})
	if in.Set([]string{"SH", "SJ"}) != s || in.Set([]string{"SJ", "SH"}) == s || in.Set(nil) != 0 {
		t.Fatal("set interning broken")
	}
	if got := in.GetSet(s); len(got) != 2 || got[0] != "SH" {
		t.Fatalf("GetSet = %v", got)
	}
}
