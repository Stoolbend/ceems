import { afterEach, expect, test, vi } from 'vitest'
import { ApiError, onUnauthorized, pending, request } from './api'

const handler = vi.fn()
onUnauthorized(handler)

afterEach(() => {
  vi.unstubAllGlobals()
  handler.mockReset()
})

function respond(status: number, code: string) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code, message: 'm' } }), { status })))
}

test('a 401 on an ordinary call calls the handler once and still throws', async () => {
  respond(401, 'unauthorized')
  await expect(request('GET', '/api/v1/pages')).rejects.toMatchObject({ status: 401, code: 'unauthorized' })
  expect(handler).toHaveBeenCalledOnce()
})

test('a 401 from login is a credentials error, not an expiry', async () => {
  respond(401, 'invalid_credentials')
  await expect(request('POST', '/api/v1/login', {})).rejects.toMatchObject({ status: 401, code: 'invalid_credentials' })
  expect(handler).not.toHaveBeenCalled()
})

test('a 401 from /me is the anonymous answer', async () => {
  respond(401, 'unauthorized')
  await expect(request('GET', '/api/v1/me')).rejects.toBeInstanceOf(ApiError)
  expect(handler).not.toHaveBeenCalled()
})

test('a 403 does not call the handler', async () => {
  respond(403, 'forbidden')
  await expect(request('GET', '/api/v1/users')).rejects.toMatchObject({ status: 403 })
  expect(handler).not.toHaveBeenCalled()
})

test('pending rises during a request and returns to 0 after success, 204, error and a rejected fetch', async () => {
  const seen: number[] = []
  const track = (res: () => Promise<Response>) => vi.fn(() => { seen.push(pending.value); return res() })
  vi.stubGlobal('fetch', track(async () => new Response('{}', { status: 200 })))
  await request('GET', '/x')
  vi.stubGlobal('fetch', track(async () => new Response(null, { status: 204 })))
  await request('GET', '/x')
  vi.stubGlobal('fetch', track(async () => new Response('{}', { status: 500 })))
  await expect(request('GET', '/x')).rejects.toBeInstanceOf(ApiError)
  vi.stubGlobal('fetch', track(async () => { throw new TypeError('network') }))
  await expect(request('GET', '/x')).rejects.toBeInstanceOf(TypeError)
  expect(seen).toEqual([1, 1, 1, 1])
  expect(pending.value).toBe(0)
})
