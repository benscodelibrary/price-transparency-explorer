package store

import "strings"

// Small closed sets are stored as uint8 enums: one byte per rate instead of a
// 16-byte string header, and cheap equality checks in the filter loop.
// Index 0 is always "unknown" so unexpected source values are kept, not lost.

type (
	NegotiatedType uint8
	BillingClass   uint8
	Setting        uint8
	Arrangement    uint8
	TINType        uint8
)

var (
	negotiatedTypeNames = []string{"unknown", "negotiated", "derived", "fee schedule", "percentage", "per diem"}
	billingClassNames   = []string{"unknown", "professional", "institutional", "both"}
	settingNames        = []string{"unknown", "inpatient", "outpatient", "both"}
	arrangementNames    = []string{"unknown", "ffs", "bundle", "capitation"}
	tinTypeNames        = []string{"unknown", "ein", "npi"}
)

const (
	TypePercentage NegotiatedType = 4

	ClassUnknown BillingClass = 0

	TINUnknown TINType = 0
	TINEIN     TINType = 1
	TINNPI     TINType = 2
)

func lookup(names []string, v string) uint8 {
	v = strings.ToLower(strings.TrimSpace(v))
	for i, n := range names {
		if n == v {
			return uint8(i)
		}
	}
	return 0
}

func name(names []string, i uint8) string {
	if int(i) < len(names) {
		return names[i]
	}
	return names[0]
}

func ParseNegotiatedType(s string) NegotiatedType {
	return NegotiatedType(lookup(negotiatedTypeNames, s))
}
func ParseBillingClass(s string) BillingClass { return BillingClass(lookup(billingClassNames, s)) }
func ParseSetting(s string) Setting           { return Setting(lookup(settingNames, s)) }
func ParseArrangement(s string) Arrangement   { return Arrangement(lookup(arrangementNames, s)) }
func ParseTINType(s string) TINType           { return TINType(lookup(tinTypeNames, s)) }

func (t NegotiatedType) String() string { return name(negotiatedTypeNames, uint8(t)) }
func (c BillingClass) String() string   { return name(billingClassNames, uint8(c)) }
func (s Setting) String() string        { return name(settingNames, uint8(s)) }
func (a Arrangement) String() string    { return name(arrangementNames, uint8(a)) }
func (t TINType) String() string        { return name(tinTypeNames, uint8(t)) }
