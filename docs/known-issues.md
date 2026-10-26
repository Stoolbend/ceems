# Known issues

Observed product defects that are not being fixed right now. Process problems
belong in the coordinator protocol (private process repository) under **Lessons that bind packets**, not
here. Close an entry by naming the task id that fixed it; do not delete it.

<!--
### KI-001 — <symptom in one line>

Found: <YYYY-MM-DD> by <task id or role>
Status: <open / fixed in T-nnn / won't fix (D-nnn)>
Evidence: <path:line, test name, or evidence reference>

<How to reproduce, what happens, what should happen. No credentials or raw
captures.>
-->

### KI-001 — Login redirect query is not checked to be a local path

Found: 2026-10-12 by T-007 conformance review
Status: fixed in T-016
Evidence: `web/src/LoginView.vue:19`

After login the view calls `router.replace(route.query.redirect ?? '/')`.
vue-router treats a plain path as app-relative, but a value such as
`//example.com` has not been tested and might navigate off-site. Accept only
values that start with a single `/`.

### KI-002 — The SPA keeps a cached user after the server session ends

Found: 2026-10-12 by the M4 design record (packet-architect)
Status: fixed in T-040
Evidence: `web/src/session.ts`

`ensureUser` caches the user after the first `/me`. When the session later
expires or is revoked on the server, the next API call fails with 401 and
the view shows an error until the page is reloaded, instead of returning to
the login screen. Fix: clear `user` and route to `/login` on any 401 in
`web/src/api.ts`.

### KI-003 — IPv4-mapped forwarded addresses are not matched by IPv4 proxy prefixes

Found: 2026-10-12 by T-020 developer and conformance review
Status: fixed in T-041
Note 2026-10-21: a mapped prefix of /96 or longer is now read as the IPv4 prefix it names, so `::ffff:0:0/96` trusts every IPv4 address. `::/0`, or a mapped prefix shorter than /96, no longer trusts a mapped-form header entry.
Evidence: `internal/httpapi/httpapi.go` (`clientIPFor`, `inPrefixes`)

`netip.Prefix.Contains` returns false for `::ffff:10.0.0.3` against
`10.0.0.0/8`. If an inner proxy appends its own address to
`X-Forwarded-For` in IPv4-mapped form, that entry counts as the client, so
every client behind that chain shares one rate-limit key and audit/log record
the proxy. A client cannot use this to spoof its IP. Fix: `addr.Unmap()`
before the prefix check in both places, plus one table case. Until then,
list the proxy in the form it emits.

### KI-004 — On Windows a blocked rename waits for the next page change

Found: 2026-10-12 by T-021 developer; judged acceptable by the conformance review
Status: open (Windows only; production is Linux tmpfs)
Note 2026-10-21: T-042 made the test poll `os.Stat` and read only after a change, but the flake remains: 1 of 20 and 1 of 40 repeat runs on `main` (`b8f8587`) still timed out. So the reader was not the only cause; the writer retry (a design change) is the remaining fix.
Evidence: `internal/output/output.go` (`Reconcile`, the `os.Rename` step)

On Windows, `os.Rename` fails with a sharing violation if another process
holds the target `P<MPP>.tti` open without `FILE_SHARE_DELETE` (Go's own
`os.ReadFile` does). The writer logs the error and retries only at the next
trigger, so the file stays stale until another page changes or `serve`
restarts. `TestEditReachesFileWithinAMinute` polls every 10 ms by reading the
file and so has a small Windows-only flake window. Fixes if it matters:
poll `os.Stat` in the test; a retry timer after a failed reconcile in the
writer (a design change).
