package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Raw shapes mirror the CMS in-network schema. Only fields we use are
// declared; json ignores the rest (bundled_codes, covered_services, ...).

type rawProviderRef struct {
	ProviderGroupID flexNumber         `json:"provider_group_id"`
	NetworkName     []string           `json:"network_name"`
	ProviderGroups  []rawProviderGroup `json:"provider_groups"`
	Location        string             `json:"location"`
}

type rawProviderGroup struct {
	NPI []flexNumber `json:"npi"`
	TIN struct {
		Type         string `json:"type"`
		Value        string `json:"value"`
		BusinessName string `json:"business_name"`
	} `json:"tin"`
}

type rawItem struct {
	NegotiationArrangement string              `json:"negotiation_arrangement"`
	Name                   string              `json:"name"`
	BillingCodeType        string              `json:"billing_code_type"`
	BillingCodeTypeVersion string              `json:"billing_code_type_version"`
	BillingCode            string              `json:"billing_code"`
	Description            string              `json:"description"`
	NegotiatedRates        []rawNegotiatedRate `json:"negotiated_rates"`
}

type rawNegotiatedRate struct {
	ProviderReferences []flexNumber         `json:"provider_references"`
	ProviderGroups     []rawProviderGroup   `json:"provider_groups"`
	NegotiatedPrices   []rawNegotiatedPrice `json:"negotiated_prices"`
}

type rawNegotiatedPrice struct {
	NegotiatedType        string   `json:"negotiated_type"`
	NegotiatedRate        *float64 `json:"negotiated_rate"`
	ExpirationDate        string   `json:"expiration_date"`
	ServiceCode           []string `json:"service_code"`
	BillingClass          string   `json:"billing_class"`
	Setting               string   `json:"setting"`
	BillingCodeModifier   []string `json:"billing_code_modifier"`
	AdditionalInformation string   `json:"additional_information"`
}

// flexNumber accepts a JSON number or a numeric string. Real-world files are
// inconsistent about quoting identifiers, and one bad element should not fail
// a whole file.
type flexNumber string

func (f *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		unq, err := strconv.Unquote(s)
		if err != nil {
			return err
		}
		s = strings.TrimSpace(unq)
	}
	*f = flexNumber(s)
	return nil
}

func (f flexNumber) Uint32() (uint32, bool) {
	n, err := strconv.ParseUint(string(f), 10, 32)
	return uint32(n), err == nil && n < inlineIDBase
}

// ParseInNetwork stream-decodes one in-network file into b.
//
// json.Decoder is used token by token for the outer object and arrays, and
// Decode() for one provider reference or one in_network item at a time. Peak
// memory is therefore one element, not the whole (100+ MB) document.
//
// Errors: an element with the wrong shape is skipped and counted; a syntax
// error (truncated or corrupt file) stops parsing and is returned, and the
// rows read before it remain in the builder (the file is "partial").
func ParseInNetwork(ctx context.Context, r io.Reader, b *Builder) error {
	dec := json.NewDecoder(r)
	if err := expectDelim(dec, '{'); err != nil {
		return fmt.Errorf("in-network file: %w", err)
	}
	for dec.More() {
		key, err := readKey(dec)
		if err != nil {
			return err
		}
		switch key {
		case "reporting_entity_name":
			err = dec.Decode(&b.Header.ReportingEntityName)
		case "reporting_entity_type":
			err = dec.Decode(&b.Header.ReportingEntityType)
		case "last_updated_on":
			err = dec.Decode(&b.Header.LastUpdatedOn)
		case "version":
			err = dec.Decode(&b.Header.Version)
		case "provider_references":
			err = eachElement(ctx, dec, b, "provider_reference", func() error {
				var ref rawProviderRef
				if err := dec.Decode(&ref); err != nil {
					return err
				}
				if ref.Location != "" && len(ref.ProviderGroups) == 0 {
					b.skip("remote_provider_reference") // fetched lazily in a future version
					return nil
				}
				id, ok := ref.ProviderGroupID.Uint32()
				if !ok {
					b.skip("invalid_provider_group_id")
					return nil
				}
				b.addGroup(id, false, ref.NetworkName, ref.ProviderGroups)
				return nil
			})
		case "in_network":
			err = eachElement(ctx, dec, b, "in_network_item", func() error {
				var it rawItem
				if err := dec.Decode(&it); err != nil {
					return err
				}
				b.addItem(&it)
				return nil
			})
		default:
			err = skipValue(dec) // unknown or unused keys, e.g. reporting_plans
		}
		if err != nil {
			return err
		}
	}
	b.linkRatesToGroups()
	return nil
}

// eachElement iterates a JSON array, calling fn for each element. Type errors
// inside one element are counted and skipped; anything else is fatal.
func eachElement(ctx context.Context, dec *json.Decoder, b *Builder, what string, fn func() error) error {
	if err := expectDelim(dec, '['); err != nil {
		return fmt.Errorf("%s array: %w", what, err)
	}
	for n := 0; dec.More(); n++ {
		if n%256 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				// Decode consumed the whole element before reporting this, so
				// the stream is still positioned at the next element.
				b.skip("malformed_" + what)
				continue
			}
			return fmt.Errorf("%s #%d at byte %d: %w", what, n, dec.InputOffset(), err)
		}
	}
	_, err := dec.Token() // closing ]
	return err
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("expected %q, got %v", want, tok)
	}
	return nil
}

func readKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected object key, got %v", tok)
	}
	return key, nil
}

// skipValue discards the next value without buffering it, by counting
// nesting depth over tokens. A json.RawMessage would buffer the whole value,
// which could be large for unknown arrays.
func skipValue(dec *json.Decoder) error {
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
		if depth == 0 {
			return nil
		}
	}
}
