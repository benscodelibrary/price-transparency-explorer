// Client-side validation mirrors the server's rules (server/internal/store/codes.go
// and server/internal/search/query.go). Users get instant, field-level feedback
// and the server never sees a request it would reject; the server still
// validates everything, so these checks are a convenience, not a guarantee.

/** Normalizes a billing code exactly like the server: upper case, no spaces,
 * and a bare 3-digit number padded to a 4-digit revenue code ("450" -> "0450"). */
export function normalizeCode(raw: string): string {
  const code = raw.replace(/\s+/g, '').toUpperCase()
  return /^\d{3}$/.test(code) ? `0${code}` : code
}

export function validateCode(raw: string): string | null {
  const code = normalizeCode(raw)
  if (!code) return 'Enter a billing code, for example 99213, 45380 or 0450.'
  if (!/^[A-Z0-9]{3,7}$/.test(code)) return 'Billing codes are 3 to 7 letters or digits, for example 99213, G0444 or 0450.'
  return null
}

/** Splits a free-text list on commas, semicolons or whitespace. */
export function splitList(raw: string): string[] {
  return raw
    .split(/[,;\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

/** Luhn check over the "80840" prefix, per the CMS NPI standard. */
export function isValidNPI(npi: string): boolean {
  if (!/^[1-9]\d{9}$/.test(npi)) return false
  const digits = `80840${npi.slice(0, 9)}`
  let sum = 0
  for (let i = digits.length - 1, double = true; i >= 0; i--, double = !double) {
    let d = Number(digits[i])
    if (double) {
      d *= 2
      if (d > 9) d -= 9
    }
    sum += d
  }
  return (10 - (sum % 10)) % 10 === Number(npi[9])
}

export interface ListResult {
  values: string[]
  error: string | null
}

export function validateNPIs(raw: string): ListResult {
  const values: string[] = []
  for (const token of splitList(raw)) {
    const npi = token.replace(/-/g, '')
    if (!/^\d{10}$/.test(npi)) {
      return { values, error: `"${token}" is not an NPI. NPIs are 10 digits.` }
    }
    if (!isValidNPI(npi)) {
      return { values, error: `${npi} fails the NPI check digit. Check for a typo.` }
    }
    values.push(npi)
  }
  if (values.length > 20) return { values, error: 'Search at most 20 NPIs at once.' }
  return { values, error: null }
}

export function validateEINs(raw: string): ListResult {
  const values: string[] = []
  for (const token of splitList(raw)) {
    const digits = token.replace(/\D/g, '')
    if (digits.length !== 9 || /[^\d-]/.test(token)) {
      return { values, error: `"${token}" is not an EIN. EINs are 9 digits, for example 12-3456789.` }
    }
    values.push(digits)
  }
  if (values.length > 20) return { values, error: 'Search at most 20 EINs at once.' }
  return { values, error: null }
}

export function formatEIN(digits: string): string {
  return /^\d{9}$/.test(digits) ? `${digits.slice(0, 2)}-${digits.slice(2)}` : digits
}
