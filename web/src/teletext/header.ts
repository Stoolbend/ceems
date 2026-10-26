export const DEFAULT_HEADER = '        CeeMS        mpp        %H:%M:%S'

export function headerRow(template: string, mpp: string, now: Date): string {
  const two = (n: number) => String(n).padStart(2, '0')
  const parts: Record<string, string> = {
    H: two(now.getHours()),
    M: two(now.getMinutes()),
    S: two(now.getSeconds()),
  }
  return template
    .replace('mpp', mpp.toUpperCase())
    .replace(/%([HMS])/g, (_, k: string) => parts[k])
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** The 40-cell header vbit2 draws: cells 0-7 blank, then the 32-cell template with its ten codes filled in. Each code is as wide as its result, so the template's 32 cells stay 32. */
export function vbitHeader(template: string, mpp: string, now: Date): string {
  const two = (n: number) => String(n).padStart(2, '0')
  const day = two(now.getDate())
  const codes: Record<string, string> = {
    '%%#': mpp.toUpperCase(),
    '%%a': DAYS[now.getDay()],
    '%%b': MONTHS[now.getMonth()],
    '%d': day,
    '%e': day.replace(/^0/, ' '),
    '%m': two(now.getMonth() + 1),
    '%y': two(now.getFullYear() % 100),
    '%H': two(now.getHours()),
    '%M': two(now.getMinutes()),
    '%S': two(now.getSeconds()),
  }
  return ' '.repeat(8) + template.replace(/%%[#ab]|%[demyHMS]/g, (c) => codes[c])
}
