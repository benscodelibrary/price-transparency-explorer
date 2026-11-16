import { describe, expect, it } from 'vitest'
import { apiParams, emptyForm, formFromQuery, queryFromForm } from './searchParams'

describe('URL <-> form', () => {
  it('builds a clean query with normalized values and no defaults', () => {
    const f = { ...emptyForm(), code: ' 450 ', npi: '1234567893', ein: '11-3471596', match: 'linked' as const }
    expect(queryFromForm(f)).toEqual({ code: '0450', npi: '1234567893', ein: '113471596', match: 'linked' })
    expect(queryFromForm(f, 3).page).toBe('3')
    expect(queryFromForm({ ...emptyForm(), code: '99213' })).toEqual({ code: '99213' })
  })

  it('round-trips through the URL and ignores unknown enum values', () => {
    const f = formFromQuery({ code: '99213', ein: '113471596,201234567', sort: 'rate_desc', match: 'bogus' })
    expect(f.ein).toBe('11-3471596, 20-1234567')
    expect(f.sort).toBe('rate_desc')
    expect(f.match).toBe('all')
  })

  it('maps the route query to API params with a page size', () => {
    expect(apiParams({})).toBeNull()
    expect(apiParams({ code: '99213', page: '2', junk: 'x' })).toEqual({ code: '99213', page: '2', page_size: '25' })
  })
})
