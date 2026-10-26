# 0003 — Users, permissions and API keys

Status: `Accepted`
Date: `2026-10-11`
Raised by: `coordinator`
Decided by: `owner` (record in `docs/decisions.md` as D-nnn)

## Context

D-006: named users with password login; grants over whole magazines, single
pages or ranges; API keys with the same grant shape plus read-only or
read-write; the REST API designed as internet-facing from the start. Weight is
`critical` (`AGENTS.md`, Process weight). Page identity is the `mpp` integer
of ADR 0002, so a magazine is the range `m00` to `mFF`. ADR 0001 allows
`golang.org/x/crypto` and `golang.org/x/time` and forbids logging secrets.

## Options

Password hashing:
1. **bcrypt** (`x/crypto/bcrypt`): self-describing hash, built-in
   constant-time compare, 72-byte input limit.
2. **argon2id** (`x/crypto/argon2`): stronger against GPU attack, but we must
   write the parameter encoding, parsing and comparison ourselves, which is
   more security code to get wrong.

Sessions:
1. **Server-side rows in SQLite** with a random cookie token: revocable,
   survives restart.
2. **Signed stateless cookies**: no table, but cannot be revoked without a
   denylist.

## Decision

bcrypt and server-side sessions.

**Passwords.** bcrypt, cost 11, to be measured on a Pi 4 (target 250-500 ms);
on login, a hash with a lower cost is rehashed. Length 12-72 bytes. Login takes
the same time and returns the same error for an unknown user (it compares
against a dummy hash) as for a wrong password.

**Sessions.** `sessions(token_hash BLOB PRIMARY KEY, user_id, created_at,
expires_at)`. The token is 32 random bytes, base64url, in cookie
`ceems_session` (`HttpOnly`, `SameSite=Strict`, `Path=/`, and `Secure` when
config `secureCookies` is true). Only its SHA-256 is stored. Absolute expiry is
14 days. Logout deletes the row. Disabling a user, changing a password or
changing the admin flag deletes all that user's sessions. For a cookie request
with an unsafe method: reject if `Sec-Fetch-Site` is present and not
`same-origin`, or if `Origin` is present and its host differs from the
request's. shortcut: hand-rolled; replace with `http.CrossOriginProtection`
when the Go floor reaches 1.25.

**API keys.** Shown once as `yfk_<id>_<secret>`: `id` is 8 random bytes in
hex (public, the lookup key), `secret` is 32 random bytes base64url.
`api_keys(id PRIMARY KEY, secret_hash BLOB, name, read_only, created_by,
created_at, expires_at NULL, revoked_at NULL, last_used_at)` stores
SHA-256(secret), compared in constant time. A slow hash is unnecessary at 256
bits of entropy. Sent as `Authorization: Bearer`. When that header is present
the cookie is ignored. Keys are created and revoked by admins and never carry
admin rights.

**Grants.** `grants(id, user_id NULL, key_id NULL, lo, hi, access)`, with
exactly one owner, `0x100 <= lo <= hi <= 0x8FF` and `access IN
('read','write')`. Write implies read. A magazine is `lo=m00, hi=mFF`; a single
page is `lo=hi`. Grants are not inherited between users and keys.

**One authorisation path.** Middleware on `/api/v1/*` (except
`POST /api/v1/login` and `/healthz`) resolves the request to a `Principal` or
returns 401. It loads the principal fresh from the database on every request,
so grant changes take effect immediately:

    type Grant struct{ Lo, Hi int; Write bool }
    type Principal struct {
        Actor    string  // "user:3", "key:<id>"; used in audit and logs
        Admin    bool    // users only
        ReadOnly bool    // keys only
        Grants   []Grant
    }
    func (p Principal) Can(mpp int, write bool) bool
    // Admin -> true; write && ReadOnly -> false; else any grant with
    // Lo <= mpp <= Hi and (!write || g.Write); default false.

Page routes register through one helper, `pageRoute(pattern, write, handler)`.
It parses `{mpp}`, calls `Can`, and hands the handler the checked `mpp`. No
page handler is registered any other way. No read access returns 404; read
without write returns 403. Lists return only readable pages. A multi-page
operation checks every page. Users, grants, keys, templates, bindings and the
run log are admin-only and refuse key principals. Generated pages refuse
writes (ADR 0002). The scheduler writes as actor `job:binding:<id>` through a
store function that takes the target `mpp` from the binding row, never from
its input.

**Rate limiting** (`x/time/rate`, in memory, idle entries evicted): login per
client IP and per username; all `/api/v1` traffic per client IP and per key;
429 when exhausted. The client IP is `RemoteAddr` only.

**Audit.** `audit(id, at, actor, action, target, detail, remote)`, written in
the same transaction as every successful state change: page create, update,
upload and delete (with SHA-256 of the new bytes); user, grant and key changes;
generated-page writes. Never holds passwords, hashes, tokens or key secrets.
Logins, successful and failed, go to slog with the IP; the attempted username
is logged only if it exists.

**Bootstrap.** `ceems admin create -config <path> <username>` creates an admin
with a random 20-character password printed once to stdout. It refuses an
existing username. `ceems admin reset-password <username>` does the same for
an existing user and deletes their sessions. Both are audited as actor `cli`.
There is no web setup screen. With no admin, `serve` logs the command to run.

## Consequences

- One `Can` function and one route helper are the only places authorisation
  lives, so the mutation checks below cover all of it.
- Plain HTTP is acceptable only over ZeroTier or localhost. Internet exposure
  needs TLS in front, `secureCookies: true`, and a later decision on
  trusting a proxy's client-IP header, without which per-IP limits see a single
  IP.
- Invariants a mutation check must cover (each must fail a test when the guard
  is removed):
  1. Anonymous request to any `/api/v1` route except login: 401.
  2. Principal with no covering grant: 404 on read and write; page absent
     from lists.
  3. Read grant only: write 403.
  4. Range bounds inclusive: `lo` and `hi` allowed, `lo-1` and `hi+1` denied.
  5. Read-only key holding a write grant: write denied.
  6. Revoked or expired key, wrong secret for a valid id, or expired session:
     401.
  7. Disabled user or changed password: existing sessions 401.
  8. Non-admin user with grants over all magazines, and any key: admin routes
     403.
  9. Cookie-authenticated unsafe request from a foreign origin: 403.
  10. Login and API limits return 429 after the threshold.
  11. Every successful write leaves exactly one audit row; a failed write
      leaves none.
  12. No password, session token or key secret is stored in plaintext or
      appears in audit rows or logs (scan the test DB and captured log).
  13. Writes to a generated page: 409; the scheduler cannot write any `mpp`
      other than its binding's.
  14. A grant change applies to the next request without a new login.
