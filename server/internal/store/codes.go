package store

import "strings"

// Identifier and code normalization shared by ingest (source data) and search
// (user input). Applying the same rules on both sides is what makes lookups
// exact: "260" in the file and "0260" typed by a user meet at "0260".

// NormalizeCode normalizes a billing code from the source file.
func NormalizeCode(codeType, raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if strings.EqualFold(strings.TrimSpace(codeType), "RC") && isDigits(code) && len(code) < 4 {
		code = strings.Repeat("0", 4-len(code)) + code
	}
	return code
}

// NormalizeQueryCode normalizes a user-typed code. A bare 3-digit number can
// only be a revenue code, so it is padded the same way as RC codes at ingest.
func NormalizeQueryCode(raw string) string {
	code := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if isDigits(code) && len(code) == 3 {
		code = "0" + code
	}
	return code
}

// LinkedCode extracts a CPT/HCPCS code from a revenue-code price's
// additional_information. CMS defines that field as free text, so only values
// shaped exactly like a code are indexed; anything else is ignored.
func LinkedCode(info string) (string, bool) {
	code := strings.ToUpper(strings.TrimSpace(info))
	if len(code) != 5 {
		return "", false
	}
	switch {
	case isDigits(code): // CPT Category I, e.g. 45380
	case isLetter(code[0]) && isDigits(code[1:]): // HCPCS Level II, e.g. G0444
	case isDigits(code[:4]) && isLetter(code[4]): // CPT Category II/III, e.g. 0001U
	default:
		return "", false
	}
	return code, true
}

// InferCodeType guesses the code system for codes seen only as linked codes.
func InferCodeType(code string) string {
	switch {
	case len(code) == 5 && isLetter(code[0]) && isDigits(code[1:]):
		return "HCPCS"
	case len(code) == 5:
		return "CPT"
	case len(code) == 4 && isDigits(code):
		return "RC"
	}
	return ""
}

// NormalizeEIN strips punctuation; a valid EIN has exactly 9 digits.
func NormalizeEIN(raw string) (string, bool) {
	d := digitsOnly(raw)
	return d, len(d) == 9
}

// FormatEIN renders 9 digits as NN-NNNNNNN.
func FormatEIN(ein string) string {
	if len(ein) != 9 {
		return ein
	}
	return ein[:2] + "-" + ein[2:]
}

// ParseNPIDigits accepts a 10-digit NPI that does not start with 0. It does
// not check the Luhn digit: source data is kept as reported.
func ParseNPIDigits(raw string) (uint64, bool) {
	d := digitsOnly(raw)
	if len(d) != 10 || d[0] == '0' || len(d) != len(strings.TrimSpace(raw)) {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(d); i++ {
		n = n*10 + uint64(d[i]-'0')
	}
	return n, true
}

// ValidNPI reports whether n is 10 digits, starts 1-9 and has a valid check
// digit (Luhn over the "80840" prefix, per the CMS NPI standard). Used to
// validate user input before searching.
func ValidNPI(n uint64) bool {
	if n < 1_000_000_000 || n > 9_999_999_999 {
		return false
	}
	check := int(n % 10)
	body := n / 10
	sum := 24 // Luhn contribution of the constant prefix 80840
	double := true
	for i := 0; i < 9; i++ {
		d := int(body % 10)
		body /= 10
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return (10-sum%10)%10 == check
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isLetter(b byte) bool { return b >= 'A' && b <= 'Z' }

func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
