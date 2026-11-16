import type { Rate } from '@/api/types'

const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' })
const usdWhole = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', maximumFractionDigits: 0 })
const int = new Intl.NumberFormat('en-US')

export function money(cents: number): string {
  return usd.format(cents / 100)
}

/** Compact money for axis labels and ranges: $1,234 rather than $1,234.00. */
export function moneyShort(cents: number): string {
  return cents >= 100_000 ? usdWhole.format(cents / 100) : usd.format(cents / 100)
}

export function rateValue(r: Rate): string {
  return r.is_percentage ? `${(r.cents / 100).toLocaleString('en-US')}%` : money(r.cents)
}

export function count(n: number): string {
  return int.format(n)
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return `${count(n)} ${n === 1 ? one : many}`
}

export function bytes(n: number): string {
  if (n < 0) return '?'
  if (n < 1024 * 1024) return `${Math.round(n / 1024)} KB`
  return `${(n / 1024 / 1024).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`
}

export function date(iso: string): string {
  const [y, m, d] = iso.split('-').map(Number)
  if (!y || !m || !d) return iso
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
}

export function capitalize(s: string): string {
  return s ? s[0]!.toUpperCase() + s.slice(1) : s
}

// CMS place-of-service codes most likely to appear in rate files.
// Full list: https://www.cms.gov/medicare/coding-billing/place-of-service-codes/code-sets
const placeOfService: Record<string, string> = {
  '01': 'Pharmacy',
  '02': 'Telehealth (not in home)',
  '10': 'Telehealth (in home)',
  '11': 'Office',
  '12': 'Home',
  '15': 'Mobile unit',
  '17': 'Walk-in retail clinic',
  '19': 'Outpatient hospital, off campus',
  '20': 'Urgent care',
  '21': 'Inpatient hospital',
  '22': 'Outpatient hospital, on campus',
  '23': 'Emergency room',
  '24': 'Ambulatory surgical center',
  '31': 'Skilled nursing facility',
  '32': 'Nursing facility',
  '41': 'Ambulance (land)',
  '49': 'Independent clinic',
  '50': 'Federally qualified health center',
  '53': 'Community mental health center',
  '57': 'Non-residential substance use facility',
  '65': 'End-stage renal disease facility',
  '71': 'Public health clinic',
  '72': 'Rural health clinic',
  '81': 'Independent laboratory',
  'CSTM-00': 'All places of service',
}

export function posName(code: string): string | undefined {
  return placeOfService[code]
}

export function shortHash(sha?: string): string {
  return sha ? sha.slice(0, 12) : ''
}
