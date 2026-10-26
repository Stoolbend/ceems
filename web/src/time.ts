// Server timestamps are UTC RFC 3339 text; show them in the viewer's locale.
export function localTime(iso: string | null | undefined, fallback = ''): string {
  if (!iso) return fallback
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? fallback : d.toLocaleString()
}
