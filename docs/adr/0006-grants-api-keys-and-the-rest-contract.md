# 0006 — Grants, API keys and the REST API contract

Status: `Accepted`
Date: `2026-10-12`
Raised by: `packet-architect` (decision record for the M4 packets)
Decided by: `owner` (recorded in `docs/decisions.md` as D-023)

## Context

ADR 0003 fixed the grants table, the `Principal`/`Can` shape, the key
format `yfk_<id>_<secret>` with SHA-256 compared in constant time, that a
`Bearer` header makes the cookie ignored, that keys are never admin, the
rate limits, the audit rule and the invariants a mutation check must cover.
It left open the proxy client-IP trust, and D-015 left to M4 whether
bearer requests bypass `http.CrossOriginProtection`. ADR 0004 fixed the
page write routes, etag, `If-Match`, 412 and 428 that the REST API
inherits. ADR 0005 fixed `adminRoute` and the admin route list.

Code that bears on it (at `7cad66f`):

- `Principal` and `Can` are exactly ADR 0003's (`internal/auth/auth.go`);
  `pageRoute` already calls `Can` for read (404) and write (403), and
  `adminRoute` refuses non-admin (`internal/httpapi/httpapi.go`); the list
  filters by `Can` (`internal/httpapi/list.go`). `requireSession` never sets
  `Principal.Grants`, so M4 adds no new authorisation code, only the data
  that feeds it.
- `requireSession` is cookie-only and wraps every `/api/` path but
  `POST /api/v1/login`; the two limiters exist but serve only `login`.
- `http.CrossOriginProtection` wraps the whole mux.
- `clientIP` is `RemoteAddr` only and is used by `login` and by `audit()`
  (`internal/httpapi/pages.go`).
- `http.ListenAndServe` runs with no timeouts (`cmd/ceems/main.go`).
- The migration runner refuses numbering gaps (`internal/store/store.go`);
  0006 is the scheduler's (ADR 0005), so M4 starts at 0007.
- Every stored password hash is bcrypt cost 11; nothing of a lower cost
  exists to rehash.
- `SetPasswordHash` deletes every session of the user
  (`internal/store/auth.go`).
- The dependency allowlist is stdlib plus four modules (ADR 0001); stdlib
  has no YAML parser, so a machine-read OpenAPI file must be JSON.

## Options

Grants and keys schema:
1. **One migration, 0007, holding `grants` and `api_keys`** in ADR 0003's
   shapes. The grants `CHECK` for exactly one owner needs the keys table
   to exist, and SQLite cannot add a `CHECK` by `ALTER TABLE`.
2. **Grants in 0007, keys and `key_id` in 0008.** Loses the one-owner
   `CHECK`, or rebuilds the table in 0008.

Bearer requests and CrossOriginProtection:
1. **No bypass.** The check passes a request with neither `Sec-Fetch-Site`
   nor `Origin`, so curl and server-to-server clients are untouched; a
   browser page on another origin is refused and could not read the
   response anyway, since no CORS headers are sent.
2. **Bypass when `Authorization` is present.** Only useful with CORS,
   which nothing asks for.

The API contract:
1. **A hand-written `openapi.json` and a stdlib contract test** that
   checks the document's paths equal a route registry in both directions,
   drives every operation through `httptest`, and checks each response's
   status and JSON against a six-keyword subset of the schema.
2. **Generated clients or a validator module.** Outside the allowlist.

Client IP behind a proxy:
1. **Trust nothing.** Behind a reverse proxy every client shares one IP;
   the per-IP limits throttle everyone together and the login per-IP
   bucket can be exhausted for all users.
2. **`trustedProxies` config; rightmost `X-Forwarded-For` entry not in the
   list.** About 25 lines plus tests, and one function every IP consumer
   must use.

## Decision

Option 1 for schema, cross-origin and contract; option 2 for the client IP.

**Schema.** Migration `0007_grants.sql` creates `api_keys(id TEXT PRIMARY
KEY, secret_hash BLOB, name, read_only, created_by, created_at, expires_at
NULL, revoked_at NULL, last_used_at NULL)` and `grants(id, user_id NULL
REFERENCES users ON DELETE CASCADE, key_id TEXT NULL REFERENCES api_keys ON
DELETE CASCADE, lo, hi, access CHECK IN ('read','write'), CHECK exactly one
owner, CHECK 0x100 <= lo <= hi <= 0x8FF)`. `api_keys.id` is 16 lowercase
hex characters (8 random bytes). Nothing on `users` changes.

**Users and grants.** Admin routes, all through `adminRoute`: list users
with their grants; create a user (random 20-character password shown once,
the CLI's path); `PUT /api/v1/users/{id}` with `{"disabled"}`, refusing
your own user (409 `self`); reset password (random, shown once, deletes
sessions); `POST /api/v1/users/{id}/grants` and `DELETE
/api/v1/grants/{id}`. Grants are created and deleted, never updated. There
is no admin-flag change and no user delete. Grant `lo` and `hi` travel as
three hex digits like `mpp`. `auth.UserPrincipal` loads the user's grants
on every request from `requireSession`, so a grant change applies to the
next request. The username rule moves to `auth.CheckUsername`.

**Self-service password change.** `PUT /api/v1/me/password` with
`{"currentPassword","newPassword"}`: a key principal is 403; the request
is limited by the login per-username limiter (429); the current password
is verified through `auth.Login` (wrong: 400 `wrong_password`, no state
change); the new password is 12-72 bytes (`auth.HashPassword`); the write
is `SetPasswordHash`, so every session of the user ends, the current one
included, and the response (204) also clears the cookie; audit
`user.change_password`, target `user:<id>`, detail empty.

**No rehash on login.** ADR 0003's "a hash with a lower cost is rehashed"
is not built: every hash is cost 11. When the cost changes, the packet
that changes it adds a store function that rewrites the hash without
deleting sessions; `SetPasswordHash` keeps deleting them.

**API keys.** `auth.NewKey` makes the id, the token and the hash;
`auth.ParseBearer` accepts only `Bearer yfk_<id>_<secret>`;
`auth.KeyPrincipal` looks the id up among live keys (`revoked_at IS NULL`
and `expires_at` unset or in the future), compares `SHA-256(secret)` with
`crypto/subtle.ConstantTimeCompare`, loads the key's grants and returns
`Principal{Actor: "key:<id>", ReadOnly, Grants}`; `Admin` is never set.
Only the hash of the presented secret is compared, so the stored hash
presented as a secret fails. `last_used_at` is written on every
authenticated key request. Admin routes: list, create (`name`, `readOnly`,
optional `expiresAt`; the token is in the create response only), `DELETE
/api/v1/keys/{id}` revokes (sets `revoked_at`, keeps the row and its
grants; idempotent), `POST /api/v1/keys/{id}/grants`. A read-only key may
hold a write grant; `Can` denies the write.

**Authentication order in `requireSession`.** Per-IP API limit first (429
before any database read); then, if `Authorization` is present at all,
even malformed, the bearer path and never the cookie; else the cookie path.
A key principal's `GET /api/v1/me` returns `{"user":null,"key":{"id",
"name","readOnly"}}`; its `POST /api/v1/logout` is 403. `logout` deletes
the session named by the request context's token hash, never by re-reading
the cookie.

**Rate limits** (constants; changing them is a decision entry and a code
change): login per IP and per username as T-007 built them; `/api/` per
client IP 20 requests/second, burst 40; per key 5 requests/second, burst
20; 429 `rate_limited`. In memory, the existing `limiter`.

**Cross-origin.** `http.CrossOriginProtection` stays in front of every
request, bearer ones included. No CORS headers are sent. This settles
D-015's open point; ADR 0003's invariant 9 is unchanged.

**Client IP.** Config key `trustedProxies`: a list of IP addresses or
CIDR prefixes, default empty, each validated at start (an invalid entry
fails start-up naming the key and the entry). One function computes the
client IP for a request: if the connection's address is not in the list,
it is `RemoteAddr`'s host and `X-Forwarded-For` is ignored; if it is, it
is the rightmost `X-Forwarded-For` entry that parses as an address and is
not itself in the list; when no entry qualifies, `RemoteAddr`'s host. The
login and API limits, audit `remote` and the request log line all take
the IP from that function and from nowhere else. This replaces ADR 0003's
"the client IP is `RemoteAddr` only".

**Server.** `cmd/ceems` serves through `http.Server` with
`ReadHeaderTimeout` 10 s, `ReadTimeout` 30 s, `WriteTimeout` 60 s,
`IdleTimeout` 120 s.

**Route registry and contract.** `pageRoute`, `adminRoute` and a third
helper `sessionRoute` (for `logout`, `me`, `me/password`, the page list
and `openapi.json`) record every registration in `api.routes`. The only
handlers outside the registry are `GET /healthz`, `POST /api/v1/login`
and the `/api/` not-found fallback. `internal/httpapi/openapi.json`
(OpenAPI 3.1, JSON) documents every registered route plus the two
anonymous ones, is embedded, and is served at `GET /api/v1/openapi.json`
behind `requireSession`. Shapes are the existing documents verbatim, every
object with `additionalProperties: false`; tags `session`, `pages`,
`admin`, `meta`; security schemes `cookieAuth` and `bearerAuth`, with
`admin` operations listing `cookieAuth` only. The contract test asserts
path-and-method equality with the registry, runs at least one scenario per
operation against an `httptest.Server`, requires every observed status to
be documented, and checks JSON responses with a checker that knows exactly
`type` (including the `["object","null"]` form), `required`, `properties`
with `additionalProperties: false`, `items`, `enum` and `$ref` into
`#/components/schemas`. It is a coverage check, not a validator.

**Amendments to ADR 0003**, stated explicitly:
1. Migration numbering: grants and keys arrive in one migration, 0007,
   after the scheduler's 0006.
2. "A hash with a lower cost is rehashed on login" is withdrawn until a
   cost change exists.
3. "The client IP is `RemoteAddr` only" is replaced by the
   `trustedProxies` rule above.
4. The rate-limit rule gains its numbers; the API limits apply in
   `requireSession`, before authentication for the IP and after it for
   the key.
5. "Keys are created and revoked by admins" is sharpened: revocation is
   `DELETE`, rows are never deleted, `last_used_at` is written on every
   request.
6. Key principals on session routes: `me` returns a key document, `logout`
   is 403.
7. Grants have no update; users have no admin-flag change through HTTP.
8. Changing a password through the UI exists and, as ADR 0003 says,
   deletes all of the user's sessions.
Everything else in ADR 0003, including the invariants list, stands.

## Consequences

- Authorisation still lives in `Can`, `pageRoute` and `adminRoute`; M4
  adds data (grants, key principals) and the tests that prove it, so the
  mutation tables of T-016 to T-020 and ADR 0003's invariants cover it.
- Integrators get one JSON document, served by the instance, that is
  guaranteed to list every route; a packet that adds a route without
  editing the document fails the contract test.
- A route registered by a raw `mux.HandleFunc` escapes the registry and
  its table-driven tests; conformance review of every later packet checks
  `New` for direct registrations.
- Behind a proxy that is not listed in `trustedProxies`, every client
  still shares one IP; the exposure checklist says so. A listed address can
  impersonate any client IP, so only the proxy itself is listed.
- Cross-origin browser integrations are ruled out until a later ADR adds
  CORS and a bypass together.
- Frozen, changed only by a new ADR:
  - the 0007 schema above; `api_keys.id` as 16 hex characters;
  - `UserPrincipal` and `KeyPrincipal` as the only builders of a
    `Principal`; a key principal never admin; grants never inherited;
  - any `Authorization` header means the cookie is not read; the
    authentication order in `requireSession`; `logout` by context token
    hash; the key `me` document and key `logout` 403;
  - `DELETE /api/v1/keys/{id}` as revocation; rows never deleted;
    `last_used_at` per request; the token only in the create response;
  - `PUT /api/v1/me/password` as the only self-service password route, its
    checks, its full session deletion, and `user.change_password`;
  - random passwords shown once for create and reset; no rehash on login;
  - CrossOriginProtection over every request; no CORS;
  - `trustedProxies` semantics and the single client-IP function as the
    only source of an IP for limits, audit and logs;
  - the existence of the rate-limit constants (values by decision entry);
  - the `http.Server` timeouts' existence;
  - the registry as the only registration path, its three helpers and
    the three exceptions; `openapi.json` at its location, covering every
    route, served behind the session; the six-keyword checker (extending
    it is a decision entry).
- Mutation checks later packets must keep green: every table in T-016 to
  T-020, in particular: removing either `Can` call in `pageRoute` fails a
  test naming each page route; an unlisted route fails the contract test;
  a revoked, expired or wrong-secret key is 401; the cookie is not read
  when a bearer header is present; `X-Forwarded-For` from an untrusted
  address is ignored.
