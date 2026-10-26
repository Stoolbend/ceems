# Internet exposure checklist

What must be true before the ceems API faces the internet. Every item names
the code, config key or test that satisfies it, or says "not built" with what
blocks it. Citations were checked against the code at `24a9f56` on 2026-10-12;
re-check the line numbers if the code has moved.

Status key: **built** (code and a test exist), **operator** (a deployment task
outside this repository), **not built** (no code; the blocker is named).

## Transport and proxy

| # | Requirement | Status | Satisfied by |
|---|---|---|---|
| 1 | TLS terminates in a reverse proxy; the app listens on loopback behind it | operator | Config `listen` set to `127.0.0.1:<port>` (`internal/config/config.go:16`; the shipped `ceems.example.json:2` already uses `127.0.0.1:8080`). The proxy is the owner's choice and is not part of this repository. |
| 2 | Session cookie marked `Secure` | built | Config `secureCookies: true` (`internal/config/config.go:21`), passed to `httpapi.New` at `cmd/ceems/main.go:117` and applied in `(*api).cookie` (`internal/httpapi/httpapi.go:296`). It defaults to false, so it must be set when TLS is on. `TestLoginCookieAttributes` (`internal/httpapi/httpapi_test.go:519`) checks both values. |
| 3 | Proxy client IP is trusted only from listed proxies | built | Config `trustedProxies` (`internal/config/config.go:27`, parsed at `:73`) and `clientIPFor` (`internal/httpapi/httpapi.go:382`), which takes the rightmost untrusted `X-Forwarded-For` entry. Tests: `TestLoginLimitKeyedByForwardedIP` (`httpapi_test.go:688`), `TestAPILimitKeyedByForwardedIP` (`:706`), and the config tests `TestLoadTrustedProxies` (`internal/config/config_test.go:41`) and `TestLoadErrors` (`:68`). **Must list the proxy's address when a proxy is used**; with the list empty (the default) every client appears as the proxy and shares one rate-limit key. List only the proxy itself, since a listed address can impersonate any client IP. |
| 4 | IPv4-mapped proxy addresses | known gap | KI-003 (`docs/known-issues.md`): a proxy that writes `::ffff:a.b.c.d` is not matched by an IPv4 prefix, so clients behind it share one rate-limit key. A client cannot use this to spoof its IP. Workaround: list the proxy as it appears on the socket. The fix (`Unmap()` before the prefix check) is open. |
| 5 | Security headers (HSTS, `X-Content-Type-Options`, frame denial) | not built in the app | Set at the proxy. Values: `Strict-Transport-Security: max-age=31536000`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` (or `Content-Security-Policy: frame-ancestors 'none'`). The app sets none of these. Blocker: none, this is an operator task; adding them in the app would be a new task. |

## Sessions, CSRF and login

| # | Requirement | Status | Satisfied by |
|---|---|---|---|
| 6 | Cookie `HttpOnly`, `SameSite=Strict`, `Path=/`, 14-day absolute expiry | built | `(*api).cookie` (`internal/httpapi/httpapi.go:293-298`) and `sessionTTL = 14 * 24 * time.Hour` (`:27`), used for both cookie `Max-Age` (`:259`) and the stored session expiry (`:254`). `TestLoginCookieAttributes` asserts `HttpOnly`, `SameSiteStrictMode`, `Path "/"` and `MaxAge 1209600`. |
| 7 | CSRF: unsafe requests from foreign origins refused | built | `http.NewCrossOriginProtection()` wraps the whole mux (`httpapi.go:117-121`, 403 `cross_origin`). `TestCrossOriginPost403` (`httpapi_test.go:377`). Bearer requests do not bypass it (ADR 0006, "Cross-origin"). No CORS headers are sent. Requests with no `Origin` and no `Sec-Fetch-Site` (curl, scripts) pass by design, which is why bearer keys work. |
| 8 | Login redirect accepts only local paths (KI-001) | built | `localPath` in `web/src/LoginView.vue:16-17` accepts only a value starting with a single `/`; KI-001 is marked fixed in T-016. Vitest cases in `web/src/LoginView.test.ts`: "an off-site redirect goes home instead" (`:46`) and "a local redirect is followed" (`:52`). |
| 9 | Login limits per IP and per username | built | `ipLimit` (10 burst, one per 10 s) and `userLimit` (5 burst, one per 30 s) at `httpapi.go:63-64`, checked at `:223` and `:233`. `TestLoginRateLimitedPerIP` (`httpapi_test.go:392`), `TestLoginRateLimitedPerUsername` (`:404`). |
| 10 | API limits per IP and per key, answered with 429 | built | `apiIPLimit` (40 burst, 20/s) and `keyLimit` (20 burst, 5/s) at `httpapi.go:65-66`, checked at `:152` and `:188`. `TestAPIRateLimitedPerIP` (`internal/httpapi/keys_test.go:244`), `TestKeyRateLimited429` (`:230`). Limits are in memory and reset on restart. |
| 11 | Admin bootstrap only from the CLI on the host | built | `ceems admin create` (`cmd/ceems/main.go:27`, `:40`, `:58`). There is no web setup route; the server only logs a warning when no admin exists (`:151`). |
| 12 | Anonymous surface is exactly `/healthz`, `POST /api/v1/login` and the SPA files | built | Routes at `httpapi.go:110-115`; everything under `/api/` goes through `requireSession`. `TestAnonymousAPIIs401` (`httpapi_test.go:211`) covers the protected routes and an unknown `/api/v1/` path. |

## Keys, secrets and audit

| # | Requirement | Status | Satisfied by |
|---|---|---|---|
| 13 | Secrets never logged or stored in clear | built | ADR 0001 never-logged list (`docs/adr/0001-stack-and-runtime-shape.md:91`); bcrypt cost 11 (`internal/auth/auth.go:24`); SHA-256 for session tokens and key secrets (ADR 0003, ADR 0006). `TestNoSecretInResponsesOrStore` (`httpapi_test.go:446`). |
| 14 | Every write audited with actor and IP | built | Store write paths call `insertAudit` (`internal/store/auth.go:133` and its callers in `auth.go`, `keys.go`, `store.go`, `templates.go`). `TestKeyWriteAudited` (`keys_test.go:257`) covers key writes. The test covers key writes only, so for other write paths the evidence is the code path, not a per-route test. |
| 15 | Key revocation immediate; expiry honoured | built | `(*Store).LiveKey` (`internal/store/keys.go:108`) is checked on every key request (`internal/auth/auth.go:152`). `TestRevokedKey401` (`keys_test.go:99`), `TestExpiredKey401` (`:124`). |

## Server limits

| # | Requirement | Status | Satisfied by |
|---|---|---|---|
| 16 | Request header, body and idle timeouts | built | `http.Server` in `cmd/ceems/main.go:117-121`: `ReadHeaderTimeout` 10 s, `ReadTimeout` 30 s, `WriteTimeout` 60 s, `IdleTimeout` 120 s (T-018). No dedicated test; the values are set in `main`. |
| 17 | Body size caps | built | Every request-body reader in `internal/httpapi` is capped. Login uses `MaxBytesReader` at 4096 (`httpapi.go:228`). User and key JSON bodies go through `decodeSmall` at 4096 (`users.go:47-50`; four callers in `users.go`, one in `keys.go`). Page, template and binding bodies go through `readBody` at `maxPageBody`, 256 KiB (`pages.go:17`, `:84-85`; callers `pages.go:134`, `:163`, `templates.go:80`, `bindings.go:96`). No dedicated test. |

## Data

| # | Requirement | Status | Satisfied by |
|---|---|---|---|
| 18 | Database backup before migrations | built | The migration runner writes `ceems-pre-vN.db` in the data directory via `VACUUM INTO` (`internal/store/store.go:89-93`) whenever an existing database is upgraded. A fresh database (version 0) is not backed up. |
| 19 | Off-host copy of the database and pages | operator | Not built and not planned in the app. Copy `dataDir` (database and `ceems-pre-vN.db` files) and `pagesDir` off the host on a schedule of your choosing. |

## Pre-exposure smoke check

Run against the public URL after the proxy is up (`$BASE` is the public origin,
`$KEY` an API key created for the check, `$REVOKED` a key revoked beforehand).
Each step names the expected result.

1. Health is reachable anonymously: `curl -i $BASE/healthz` gives 200.
2. Anonymous API is refused: `curl -i $BASE/api/v1/me` gives 401 and no `Set-Cookie`.
3. Cross-origin write is refused: `curl -i -X POST -H "Origin: https://evil.example" -H "Content-Type: application/json" -d "{}" $BASE/api/v1/login` gives 403 with error code `cross_origin`.
4. Revoked key is refused: `curl -i -H "Authorization: Bearer $REVOKED" $BASE/api/v1/me` gives 401.
5. Rate limit engages: send about 15 bad logins from one client with `for i in $(seq 15); do curl -s -o /dev/null -w "%{http_code}\n" -X POST -H "Content-Type: application/json" -d '{"username":"x","password":"y"}' $BASE/api/v1/login; done`. Expect 401s, then 429 (the login IP burst is 10). If an unrelated second client is limited at the same time, every client shares one rate-limit key and `trustedProxies` is wrong (rows 3 and 4).
6. Contract is served to an authenticated caller: `curl -i -H "Authorization: Bearer $KEY" $BASE/api/v1/openapi.json` gives 200 with a JSON body. It is a session route (`httpapi.go:71`), so an anonymous request gives 401, not the document.
7. Cookie flags: log in once with `curl -i` and confirm `Set-Cookie` carries `HttpOnly; Secure; SameSite=Strict; Path=/`.
8. Headers: `curl -I $BASE/` shows the row 5 headers.
