import { describe, expect, it } from 'vitest'
import { formatEIN, isValidNPI, normalizeCode, validateCode, validateEINs, validateNPIs } from './validate'

describe('normalizeCode', () => {
  it('pads 3-digit revenue codes like the server', () => {
    expect(normalizeCode('450')).toBe('0450')
    expect(normalizeCode(' g0444 ')).toBe('G0444')
    expect(normalizeCode('99 213')).toBe('99213')
    expect(normalizeCode('0450')).toBe('0450')
  })
})

describe('validateCode', () => {
  it('requires a 3-7 character alphanumeric code', () => {
    expect(validateCode('')).toMatch(/Enter a billing code/)
    expect(validateCode('9')).toMatch(/3 to 7/)
    expect(validateCode('99-213')).toMatch(/3 to 7/)
    expect(validateCode('99213')).toBeNull()
    expect(validateCode('450')).toBeNull()
  })
})

describe('NPI', () => {
  it('checks the Luhn digit over the 80840 prefix', () => {
    expect(isValidNPI('1234567893')).toBe(true) // CMS documentation example
    expect(isValidNPI('1649258286')).toBe(true) // from the Fidelis file
    expect(isValidNPI('1234567890')).toBe(false)
    expect(isValidNPI('0234567893')).toBe(false)
  })

  it('accepts comma or space separated lists and reports the first bad one', () => {
    expect(validateNPIs('1234567893, 1649258286')).toEqual({ values: ['1234567893', '1649258286'], error: null })
    expect(validateNPIs('').values).toEqual([])
    expect(validateNPIs('12345').error).toMatch(/10 digits/)
    expect(validateNPIs('1234567890').error).toMatch(/check digit/)
  })
})

describe('EIN', () => {
  it('accepts with or without the dash', () => {
    expect(validateEINs('11-3471596 113471596')).toEqual({ values: ['113471596', '113471596'], error: null })
    expect(validateEINs('11-347159').error).toMatch(/9 digits/)
    expect(validateEINs('11A3471596').error).toMatch(/9 digits/)
    expect(formatEIN('113471596')).toBe('11-3471596')
  })
})
