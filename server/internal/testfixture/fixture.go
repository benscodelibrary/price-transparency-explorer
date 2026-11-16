// Package testfixture provides a small in-network file that exercises every
// data quirk the real Fidelis files have (and a few they don't), for tests
// across packages.
package testfixture

import (
	"fmt"
	"strings"

	"price-explorer/internal/store"
)

// Valid (check-digit correct) NPIs used by the fixture.
var (
	NPIA = withCheckDigit(123456789) // group 1, and group 3 (as a quoted string)
	NPIB = withCheckDigit(198765432) // group 1
	NPIC = withCheckDigit(155555555) // group 2 and the inline group
	NPID = withCheckDigit(177777777) // group 3's TIN of type "npi"
)

const (
	EINAlpha  = "11-3471596" // group 1
	EINBetaTO = "22-2222222" // group 2, npi: [0] (TIN-only)
	EINBeta2  = "33-3333333" // group 2
	EINInline = "44-4444444" // inline provider group on CPT 99215
)

func withCheckDigit(body uint64) uint64 {
	for d := uint64(0); d < 10; d++ {
		if store.ValidNPI(body*10 + d) {
			return body*10 + d
		}
	}
	panic("unreachable")
}

const refs = `"provider_references": [
  {"provider_group_id": 1, "network_name": [], "provider_groups": [
    {"npi": [NPIA, NPIB], "tin": {"type": "ein", "value": "11-3471596", "business_name": "Alpha Clinic"}}]},
  {"provider_group_id": 2, "provider_groups": [
    {"npi": [0], "tin": {"type": "ein", "value": "22-2222222", "business_name": "Beta TIN Only"}},
    {"npi": [NPIC], "tin": {"type": "ein", "value": "33-3333333", "business_name": "Beta Two"}}]},
  {"provider_group_id": 3, "provider_groups": [
    {"npi": ["NPIA"], "tin": {"type": "npi", "value": "NPID"}}]}
]`

const items = `"in_network": [
  {"negotiation_arrangement": "ffs", "name": "OFFICE VISIT", "billing_code_type": "CPT", "billing_code_type_version": "2022",
   "billing_code": "99213", "description": "Office visit, established patient",
   "negotiated_rates": [
     {"provider_references": [1], "negotiated_prices": [
       {"negotiated_type": "negotiated", "negotiated_rate": 100.10, "expiration_date": "9999-12-31", "service_code": ["11"], "billing_class": "professional", "setting": "outpatient"},
       {"negotiated_type": "negotiated", "negotiated_rate": 90, "expiration_date": "2027-01-31", "service_code": ["11"], "billing_class": "professional", "billing_code_modifier": ["25"], "setting": "outpatient"}]},
     {"provider_references": [2], "negotiated_prices": [
       {"negotiated_type": "fee schedule", "negotiated_rate": 80.5, "expiration_date": "9999-12-31", "billing_class": "professional", "setting": "outpatient"}]}]},
  {"negotiation_arrangement": "ffs", "name": "Lab", "billing_code_type": "RC", "billing_code_type_version": "2022",
   "billing_code": "300", "description": "Laboratory",
   "negotiated_rates": [{"provider_references": [2], "negotiated_prices": [
     {"negotiated_type": "negotiated", "negotiated_rate": 27.38, "expiration_date": "9999-12-31", "service_code": ["22"], "billing_class": "institutional", "additional_information": "99213", "setting": "outpatient"},
     {"negotiated_type": "negotiated", "negotiated_rate": 12, "expiration_date": "9999-12-31", "billing_class": "institutional", "additional_information": "see contract", "setting": "outpatient"}]}]},
  {"negotiation_arrangement": "ffs", "name": "Lab again", "billing_code_type": "RC", "billing_code_type_version": "2022",
   "billing_code": "300", "description": "An unrelated procedure label (payers repeat RC items like this)",
   "negotiated_rates": [{"provider_references": [2], "negotiated_prices": [
     {"negotiated_type": "negotiated", "negotiated_rate": 27.38, "expiration_date": "9999-12-31", "service_code": ["22"], "billing_class": "institutional", "additional_information": "99213", "setting": "outpatient"}]}]},
  {"billing_code_type": "CPT", "billing_code": "99214", "negotiated_rates": "malformed"},
  {"negotiation_arrangement": "ffs", "billing_code_type": "CPT", "billing_code": "99215", "description": "Inline provider group",
   "negotiated_rates": [{"provider_groups": [{"npi": [NPIC], "tin": {"type": "ein", "value": "44-4444444", "business_name": "Inline Group"}}],
     "negotiated_prices": [{"negotiated_type": "negotiated", "negotiated_rate": 200, "expiration_date": "2026-12-31", "billing_class": "professional", "setting": "outpatient"}]}]},
  {"negotiation_arrangement": "ffs", "billing_code_type": "CPT", "billing_code": "99216",
   "negotiated_rates": [{"provider_references": [99], "negotiated_prices": [
     {"negotiated_type": "negotiated", "negotiated_rate": 1, "billing_class": "professional", "setting": "outpatient"}]}]}
]`

const header = `"reporting_entity_name": "Test Payer", "reporting_entity_type": "Health Insurance Issuer",
"last_updated_on": "2026-08-31", "version": "2.0.0",
"unused_key": {"nested": [1, 2, {"deep": [3]}]}`

// InNetwork returns the fixture file. refsFirst controls whether
// provider_references appears before or after in_network.
func InNetwork(refsFirst bool) string {
	body := items + ",\n" + refs
	if refsFirst {
		body = refs + ",\n" + items
	}
	return expand("{\n" + header + ",\n" + body + "\n}")
}

func expand(s string) string {
	return strings.NewReplacer(
		"NPIA", fmt.Sprint(NPIA), "NPIB", fmt.Sprint(NPIB),
		"NPIC", fmt.Sprint(NPIC), "NPID", fmt.Sprint(NPID),
	).Replace(s)
}

// Expected counts for InNetwork, shared by tests.
const (
	ExpectGroups      = 4 // 3 referenced + 1 inline
	ExpectItems       = 5 // 99213, RC 0300 twice, 99215, 99216 (99214 is malformed)
	ExpectRates       = 7 // 99216's only rate references a missing group
	ExpectLinkedRates = 2 // "see contract" is not a code
	ExpectUnresolved  = 1
	// The second RC 0300 item repeats a rate already indexed under 0300
	// (direct) and under 99213 (linked): both postings are skipped.
	ExpectDuplicatePostings = 2
)
