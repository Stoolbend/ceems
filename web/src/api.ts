import { ref } from 'vue'

// The only module that calls fetch. Errors from the server's
// {"error":{"code","message"}} shape are thrown as ApiError.

// headerTemplate comes from the login and me documents; an API key's lacks it.
// indexPage is the page every index link points at, or null when the index link is off.
export type User = { id: number; username: string; admin: boolean; headerTemplate?: string; indexPage?: string | null }
type MeDoc = { user: User; headerTemplate?: string; indexPage?: string | null }
const withSite = (d: MeDoc): User => ({ ...d.user, headerTemplate: d.headerTemplate, indexPage: d.indexPage })

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

let unauthorized: (() => void) | undefined
// Called on a 401 from any call except login (a credentials error) and /me (the normal anonymous answer).
export function onUnauthorized(fn: () => void) { unauthorized = fn }

// Requests in flight; the loading dialog shows while it stays above zero.
export const pending = ref(0)

// A Blob body is sent unchanged as application/octet-stream; anything else as JSON.
export async function request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  pending.value++
  try {
    return await send<T>(method, path, body, headers)
  } finally {
    pending.value--
  }
}

async function send<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  const raw = body instanceof Blob
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? headers : { ...headers, 'Content-Type': raw ? 'application/octet-stream' : 'application/json' },
    body: body === undefined ? undefined : raw ? body : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/v1/login' && path !== '/api/v1/me') unauthorized?.()
    throw new ApiError(res.status, data?.error?.code ?? 'unknown', data?.error?.message ?? res.statusText)
  }
  return data as T
}

export async function login(username: string, password: string): Promise<User> {
  return withSite(await request<MeDoc>('POST', '/api/v1/login', { username, password }))
}

export async function logout(): Promise<void> {
  await request<void>('POST', '/api/v1/logout')
}

// The system name is public: the login page shows it too.
export const getSite = () => request<{ systemName: string }>('GET', '/api/v1/site')

export async function me(): Promise<User> {
  return withSite(await request<MeDoc>('GET', '/api/v1/me'))
}

export type PageSummary = {
  mpp: string
  description: string
  subpages: number
  transmit: boolean
  updatedAt: string
  updatedBy: string
}

export async function listPages(): Promise<PageSummary[]> {
  return (await request<{ pages: PageSummary[] }>('GET', '/api/v1/pages')).pages
}

export type Subpage = {
  cycleSeconds: number | null
  cycleMode: string | null
  links: string[] | null
  rows: (string | null)[] // 25 entries, index = row; null when the subpage has no such line
}

export type Page = {
  mpp: string
  description: string
  transmit: boolean
  updatedAt: string
  updatedBy: string
  etag: string
  subpages: Subpage[]
  binding: { id: number; name: string } | null // set when a binding generates the page
  fastextGroup: { id: number; name: string; paged: boolean } | null // set when a fastext group owns the coloured links
  canWrite: boolean // whether the caller may write this page
}

export async function getPage(mpp: string): Promise<Page> {
  return request<Page>('GET', `/api/v1/pages/${mpp}`)
}

export async function putPage(page: Page): Promise<Page> {
  return request<Page>('PUT', `/api/v1/pages/${page.mpp}`, page, { 'If-Match': `"${page.etag}"` })
}

export async function uploadTTI(mpp: string, file: File): Promise<Page> {
  return request<Page>('PUT', `/api/v1/pages/${mpp}/tti`, file)
}

export async function createPage(mpp: string): Promise<Page> {
  return request<Page>('POST', `/api/v1/pages/${mpp}`)
}

// The page's etag in If-Match is the server's confirmation token.
export async function deletePage(mpp: string, etag: string): Promise<void> {
  await request<void>('DELETE', `/api/v1/pages/${mpp}`, undefined, { 'If-Match': `"${etag}"` })
}

export type TemplateSummary = { id: number; name: string; slotCount: number }

export type Slot = { name: string; sub: number; row: number; col: number; width: number; align: 'left' | 'centre' | 'right' }

export type Template = {
  id: number
  name: string
  rowRules: Record<string, string>
  slots: Slot[]
  tti: string // base64
}

export async function listTemplates(): Promise<TemplateSummary[]> {
  return (await request<{ templates: TemplateSummary[] }>('GET', '/api/v1/templates')).templates
}

// tti is the file's base64; rowRules maps a row number ("17") to a slot name.
export async function createTemplate(name: string, tti: string, rowRules: Record<string, string>): Promise<Template> {
  return request<Template>('POST', '/api/v1/templates', { name, tti, rowRules })
}

// Same body as createTemplate; the server refuses with 409 when a binding would be left with a slot it has no value for.
export async function updateTemplate(id: number, name: string, tti: string, rowRules: Record<string, string>): Promise<Template> {
  return request<Template>('PUT', `/api/v1/templates/${id}`, { name, tti, rowRules })
}

export async function deleteTemplate(id: number): Promise<void> {
  await request<void>('DELETE', `/api/v1/templates/${id}`)
}

export async function getTemplate(id: number): Promise<Template> {
  return request<Template>('GET', `/api/v1/templates/${id}`)
}

export type Run = { id: number; actor: string; startedAt: string; finishedAt: string; outcome: 'ok' | 'failed'; error: string }

// The list never carries source or static; the sheet id is only in a single Binding.
export type BindingSummary = { id: number; name: string; templateId: number; templateName: string; mpp: string; lastRun: Run | null }

export type SourceConfig = { sheet: string; range: string; columns: string[]; rows: number; format: Record<string, string> }

// static maps a fixed slot name to its cells (0-127).
export type BindingBody = { name: string; templateId: number; mpp: string; source: SourceConfig; static: Record<string, string> }

export type Binding = BindingBody & { id: number; templateName: string }

/** The slot names a source fills: each column with '#' replaced by 1..rows, plus "updated". Mirrors jobs.DataKeys. */
export function dataKeys(columns: string[], rows: number): Set<string> {
  const keys = new Set(['updated'])
  for (let i = 1; i <= rows; i++) for (const c of columns) keys.add(c.replaceAll('#', String(i)))
  return keys
}

export async function listBindings(): Promise<{ sheetsConfigured: boolean; bindings: BindingSummary[] }> {
  return request('GET', '/api/v1/bindings')
}

export async function getBinding(id: number): Promise<Binding> {
  return request<Binding>('GET', `/api/v1/bindings/${id}`)
}

export async function createBinding(b: BindingBody): Promise<Binding> {
  return request<Binding>('POST', '/api/v1/bindings', b)
}

export async function updateBinding(id: number, b: BindingBody): Promise<Binding> {
  return request<Binding>('PUT', `/api/v1/bindings/${id}`, b)
}

export async function deleteBinding(id: number): Promise<void> {
  await request<void>('DELETE', `/api/v1/bindings/${id}`)
}

// A failed run still resolves; its outcome says so.
export async function runBinding(id: number): Promise<Run> {
  return request<Run>('POST', `/api/v1/bindings/${id}/run`)
}

export async function listRuns(id: number): Promise<Run[]> {
  return (await request<{ runs: Run[] }>('GET', `/api/v1/bindings/${id}/runs`)).runs
}

// lo and hi are three uppercase hex digits, like an mpp.
export type GrantDoc = { id: number; lo: string; hi: string; access: 'read' | 'write' }

export type UserDoc = { id: number; username: string; admin: boolean; disabled: boolean; createdAt: string; grants: GrantDoc[] }

export async function listUsers(): Promise<UserDoc[]> {
  return (await request<{ users: UserDoc[] }>('GET', '/api/v1/users')).users
}

// The password is shown once and is not kept anywhere.
export async function createUser(username: string, admin: boolean): Promise<{ user: UserDoc; password: string }> {
  return request('POST', '/api/v1/users', { username, admin })
}

export async function setDisabled(id: number, disabled: boolean): Promise<UserDoc> {
  return (await request<{ user: UserDoc }>('PUT', `/api/v1/users/${id}`, { disabled })).user
}

export async function resetPassword(id: number): Promise<string> {
  return (await request<{ password: string }>('POST', `/api/v1/users/${id}/reset-password`)).password
}

export async function createGrant(userId: number, lo: string, hi: string, access: 'read' | 'write'): Promise<GrantDoc> {
  return request<GrantDoc>('POST', `/api/v1/users/${userId}/grants`, { lo, hi, access })
}

export async function deleteGrant(id: number): Promise<void> {
  await request<void>('DELETE', `/api/v1/grants/${id}`)
}

// On success the server ends every session of the user, this one included.
export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await request<void>('PUT', '/api/v1/me/password', { currentPassword, newPassword })
}

// A key document never carries the token or its hash; the times are RFC 3339 UTC.
export type KeyDoc = {
  id: string
  name: string
  readOnly: boolean
  createdBy: string
  createdAt: string
  expiresAt: string | null
  revokedAt: string | null
  lastUsedAt: string | null
  grants: GrantDoc[]
}

export async function listKeys(): Promise<KeyDoc[]> {
  return (await request<{ keys: KeyDoc[] }>('GET', '/api/v1/keys')).keys
}

// The token is shown once and is not kept anywhere. expiresAt is RFC 3339, or omitted for no expiry.
export async function createKey(name: string, readOnly: boolean, expiresAt?: string): Promise<{ key: KeyDoc; token: string }> {
  return request('POST', '/api/v1/keys', { name, readOnly, ...(expiresAt ? { expiresAt } : {}) })
}

// Revoking keeps the key and its grants; revoking again does nothing.
export async function revokeKey(id: string): Promise<void> {
  await request<void>('DELETE', `/api/v1/keys/${id}`)
}

export async function createKeyGrant(keyId: string, lo: string, hi: string, access: 'read' | 'write'): Promise<GrantDoc> {
  return request<GrantDoc>('POST', `/api/v1/keys/${keyId}/grants`, { lo, hi, access })
}

// pagesDir is the config file's value: shown, never sent back.
export type Settings = {
  systemName: string
  pagesDir: string
  vbitConf: boolean
  headerTemplate: string
  initialTeletextPage: string
  rowAdaptiveMode: boolean
  linesPerField: number
  magazinePriority: string
  statusDisplay: string
  indexPageEnabled: boolean
  indexPage: string
}

export async function getSettings(): Promise<Settings> {
  return request<Settings>('GET', '/api/v1/settings')
}

// Sends the whole document without pagesDir; the response is the saved settings.
export async function putSettings(s: Settings): Promise<Settings> {
  const { pagesDir: _, ...body } = s
  return request<Settings>('PUT', '/api/v1/settings', body)
}

// links are the four coloured keys (red, green, yellow, cyan). A paged group's prev and next slots (0-3)
// hold 8ff in links; a static group has null slots. texts are the four link texts (decoded cells, up to 9 each);
// the group writes row 24 of its pages from them, and a page that leaves it keeps its links and gets an empty row 24.
export type FastextMember = { mpp: string; description: string }

export type FastextGroupBody = { name: string; paged: boolean; links: string[]; prevSlot: number | null; nextSlot: number | null; default: boolean; texts: string[] }

export type FastextGroup = FastextGroupBody & { id: number; pages: FastextMember[] }

export async function getFastextGroups(): Promise<FastextGroup[]> {
  return (await request<{ groups: FastextGroup[] }>('GET', '/api/v1/fastext-groups')).groups
}

export async function createFastextGroup(b: FastextGroupBody): Promise<FastextGroup> {
  return request<FastextGroup>('POST', '/api/v1/fastext-groups', b)
}

export async function updateFastextGroup(id: number, b: FastextGroupBody): Promise<FastextGroup> {
  return request<FastextGroup>('PUT', `/api/v1/fastext-groups/${id}`, b)
}

// Members keep their current links.
export async function deleteFastextGroup(id: number): Promise<void> {
  await request<void>('DELETE', `/api/v1/fastext-groups/${id}`)
}

// Sets the whole membership in order; a page in another group is 409 in_group.
export async function setFastextGroupPages(id: number, mpps: string[]): Promise<FastextGroup> {
  return request<FastextGroup>('PUT', `/api/v1/fastext-groups/${id}/pages`, { pages: mpps })
}
