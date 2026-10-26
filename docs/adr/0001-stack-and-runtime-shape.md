# 0001 — Stack and runtime shape

Status: `Accepted`
Date: `2026-10-11`
Raised by: `coordinator`
Decided by: `owner` (record in `docs/decisions.md` as D-nnn)

## Context

D-002 fixes Go, Vue 3, SQLite through a pure-Go driver, one binary and one
process holding web UI, REST API, scheduler and renderer. D-007 fixes
cross-compiling `linux/arm64` and `windows/amd64` with `CGO_ENABLED=0` from the
owner's Windows PC; D-001 fixes a Pi 4 on Bookworm Lite 64-bit. The owner wants
readability over speed, with tests proving behaviour. The generator being
replaced is Go 1.23 (`pagegen-highscores/go.mod:3`), stdlib plus the Sheets
client, configured by JSON (`main.go:72-78`, `main.go:168-236`) and built with
`CGO_ENABLED=0` (`build.ps1`). This ADR settles layout, configuration,
migrations, test tooling and logging, and names the conventions later packets
must not drift from.

## Options

1. **Standard library first**: `net/http` ServeMux method patterns,
   `database/sql` with `modernc.org/sqlite`, embedded SQL migrations tracked by
   `PRAGMA user_version`, JSON config, `log/slog`. Cost: a small hand-written
   migration runner and JSON helpers.
2. **Frameworks**: chi or echo, golang-migrate, viper with YAML, zap. Less hand
   code, five or more dependencies to keep current, nothing needed at this size.
3. **Code generation** (sqlc, OpenAPI generators): typed queries and clients,
   at the cost of a generator toolchain and generated files in every diff.

## Decision

Option 1. Everything it needs is in Go 1.23's standard library or two
`golang.org/x` modules, and every line is readable without knowing a framework.

**Layout** (module path `ceems`; only `cmd/ceems` is `package main`):

    cmd/ceems/        subcommands: serve, import, admin
    internal/config/    load and validate the config file
    internal/store/     open SQLite, run migrations, all SQL; migrations/NNNN_name.sql
    internal/tti/       TTI codec; imports nothing internal
    internal/tmpl/      template slot rendering; imports only tti
    internal/output/    vbit2 pages-directory writer
    internal/auth/      users, sessions, API keys, grants, authorisation
    internal/httpapi/   routes, handlers, middleware
    internal/jobs/      scheduler and the Google Sheets source (M3)
    web/                Vue 3 + Vite + TypeScript app; web/embed.go
    testdata/sample/    sample TTI pages, high-score template, vbit.conf (T-080)
    scripts/            build.ps1, test.ps1
    deploy/             systemd unit, example config (M2)

Imports point one way: `httpapi` and `jobs` use `store`, `auth`, `output` and
`tmpl`; `store` uses `tti`; nothing imports `httpapi`.

**UI embedding.** `web/embed.go` carries `//go:build ui` and
`//go:embed all:dist`. A sibling file without the tag serves a one-line
"UI not built" page, so `go test ./...` passes on a clean clone without npm.
`scripts/build.ps1` runs `npm ci` and `npm run build` in `web/`, then
`go build -tags ui -trimpath -ldflags "-s -w"` with `CGO_ENABLED=0` for both
targets into `dist/` (gitignored). In development the Vite dev server proxies
`/api` to the Go server.

**HTTP.** Go 1.22+ `ServeMux` patterns (`GET /api/v1/pages/{mpp}`); no router
dependency. REST lives under `/api/v1/`, JSON only. Errors are
`{"error":{"code":"<snake_case>","message":"<text>"}}`. `GET /healthz` is
anonymous. Every other non-`/api` GET serves the SPA with an `index.html`
fallback.

**Configuration.** One JSON file, path from `-config`, default `ceems.json`
in the working directory (on the Pi, `/etc/ceems/ceems.json` set by the
systemd unit). Initial keys: `listen`, `dataDir` (holds `ceems.db`),
`pagesDir`, `logLevel`. Decoded with `DisallowUnknownFields`; a missing or
invalid key fails start-up with an error naming the key. The file holds no
secret; a secret file (the Google key, Q-001) is referenced by path only.

**SQLite.** Driver `modernc.org/sqlite`, opened with `journal_mode=WAL`,
`foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`, and
`SetMaxOpenConns(1)`. shortcut: one connection serialises all queries; revisit
if a slow read visibly blocks writes. Times are TEXT, UTC, RFC 3339 with `Z`.
Page bytes are BLOB, never TEXT.

**Migrations.** Numbered `.sql` files embedded from
`internal/store/migrations/`, applied in order at start, each in its own
transaction that also sets `PRAGMA user_version`. Before migrating a
non-empty database, the runner writes `VACUUM INTO
'<dataDir>/ceems-pre-v<N>.db'`. Forward only. A database newer than the
binary refuses to start.

**Logging.** `log/slog` text handler to stderr (journald on the Pi). One line
per request: method, route pattern, status, duration, actor id. Never logged:
passwords, password hashes, session tokens, API key secrets, `Authorization`
and `Cookie` headers, request or response bodies, Google key contents, sheet
URLs or ids (log a binding's name instead).

**Tests.** Go: standard `testing` only, `httptest` for handlers, store tests
against a real migrated database in `t.TempDir()`, golden files under
`testdata/`. The race detector needs cgo and is not part of the gate. Web:
vitest with `@vue/test-utils` and jsdom, plus `vue-tsc --noEmit`.
`scripts/test.ps1` runs `go vet ./...`, `go test ./...`, `npm test` and the
type check, and fails if any of them fails.

**Frontend.** Single-file components with `<script setup lang="ts">`,
`vue-router`, no component library, no Pinia unless a packet justifies it. All
HTTP calls go through one module, `web/src/api.ts`.

## Consequences

- One binary per target, built and tested on Windows; no cgo toolchain, no
  container.
- Hand-written migration runner and JSON helpers, small and tested.
- Frozen conventions every later packet must respect, changed only by a new ADR:
  - the package layout and import direction above;
  - Go dependency allowlist: `modernc.org/sqlite`, `golang.org/x/crypto`,
    `golang.org/x/time`, `google.golang.org/api` (M3); anything else needs a
    coordinator decision;
  - `CGO_ENABLED=0` for every build;
  - `/api/v1/` prefix, the JSON error shape, `/healthz`;
  - migrations are append-only: never edit or renumber one that has merged;
  - UTC RFC 3339 TEXT times, BLOB page bytes;
  - the never-logged list above;
  - config decoding rejects unknown keys; new keys are added in the packet
    that needs them, with a default or a named start-up error.
