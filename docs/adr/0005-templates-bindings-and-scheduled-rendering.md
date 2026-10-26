# 0005 — Templates, bindings and scheduled rendering

Status: `Accepted`
Date: `2026-10-12`
Raised by: `packet-architect` (decision record for the first three M3 packets)
Decided by: `owner` (recorded in `docs/decisions.md` as D-020)

## Context

M3 replaces the old high-score generator: templates with slot tokens are
rendered from Google Sheet rows into ordinary pages, hourly and on demand.
ADR 0002 fixed the template table, the slot grammar and width rule,
sanitising, blanking, PN rewriting, the 409 for human writes to a generated
page and detach-on-removal. ADR 0003 fixed that templates, bindings and the
run log are admin-only and refuse key principals, that the scheduler writes
as actor `job:binding:<id>` through a store function that takes the target
`mpp` from the binding row, and that generated-page writes are audited.
ADR 0004 made `SavePage`, `CreatePage` and `DeletePage` the only writers of
`pages` after import and said M3 would "reuse `SavePage` with their own
action". D-009 (Q-001 a) puts the Google service-account key in a file on
the Pi referenced by path in config, never in the database or the UI. D-018
moves the output writer after M4, so in M3 "leaves the previous rendered page
in place" is proved for the page row only.

Code that bears on it:

- `lockedRow` refuses any page whose `binding_id` is set
  (`internal/store/store.go`), so `SavePage` cannot write a generated page
  without weakening the rule for humans. The steps it shares with the other
  writers are unexported: `archive`, `updatePage`, `insertPage`
  (`internal/store/store.go`) and `insertAudit` (`internal/store/auth.go`).
  `insertPage` writes `binding_id NULL` and `updatePage` never touches it.
- `pageRoute` is the only authorisation helper and the principal carries
  only `Admin` today (`internal/httpapi/httpapi.go`).
- `config.Load` rejects unknown keys (`internal/config/config.go`).
- `tti.Parse` requires a `PN` line and caps rows at 40 cells
  (`internal/tti/tti.go`). T-009 added `Row`, `SetRow`, `Field`, `SetField`,
  which encode in the page's control-code form and insert absent rows.
- ADR 0001 lists `httpapi` and `jobs` as peers that both use `store`, `auth`,
  `output` and `tmpl`; it does not let `httpapi` import `jobs`. The binding
  routes must validate the source configuration and trigger a run, both of
  which live in `jobs`.
- The old generator builds the Sheets client from the key file with the
  read-only scope (`pagegen-highscores/main.go:125-129`), reads the range
  twice, formatted and unformatted with serial-number dates (`295-310`),
  keeps the first contiguous run of complete rows up to eight (`317-347`),
  formats "last updated" in host-local time (`820-826`), and prints the
  spreadsheet id in two error paths (`286`, `516`), which this project must
  not repeat (ADR 0001 never-logged list).

## Options

Writing the generated page:

1. **A sibling writer `SaveGenerated(bindingID, p, a)` sharing `SavePage`'s
   body.** It takes no `mpp`; it reads the target from the binding row,
   checks the page's `PN` and the page's `binding_id` against it, sets the
   actor and action itself, then runs the archive, derive and audit steps
   `SavePage` runs. Cost: a fourth writer, amending ADR 0004's frozen list.
2. **Extend `SavePage` with a binding parameter.** Touches the integrated
   callers for no gain over option 1.
3. **Write by raw SQL from `jobs`.** Rejected: duplicates the invariants ADR
   0004 put in one place.

Where the bindings table lives:

1. **With templates, in one migration, with store functions that attach and
   detach `pages.binding_id`.** `SaveGenerated` needs a bindings row to read
   the `mpp` from, so the table cannot come later.
2. **With the scheduler.** Rejected for that reason.

Sheets access:

1. **`google.golang.org/api/sheets/v4` with `option.WithCredentialsFile`**,
   as the old generator, behind a one-method `Source` interface; tests run
   the real client against an `httptest` server. Cost: about 25 indirect
   modules and a larger binary.
2. **Stdlib JWT signing and REST calls.** About 120 lines of our own
   authentication code; rejected against an allowlisted library the owner
   already runs.

Scheduling:

1. **One goroutine, one mutex**: every binding runs in id order once at
   start and at every top of the hour; run-now runs synchronously in the
   request under `TryLock` and answers 409 when a run is in progress.
2. **Per-binding goroutines and intervals.** More state; no criterion asks
   for it; six sequential reads take seconds.

`httpapi` and `jobs`:

1. **Let `httpapi` import `jobs`.** No cycle: `jobs` never imports `httpapi`.
2. **Inject functions into `httpapi.New`.** Avoids the import at the cost of
   a parameter per capability (validate source, run now) that grows with M3.

## Decision

Option 1 in each case.

**Schema.** Migration `0005_templates.sql`:

    CREATE TABLE templates (
        id        INTEGER PRIMARY KEY,
        name      TEXT    NOT NULL UNIQUE,
        tti       BLOB    NOT NULL,
        row_rules TEXT    NOT NULL DEFAULT '{}'    -- JSON object "row" -> slot name
    );
    CREATE TABLE bindings (
        id          INTEGER PRIMARY KEY,
        name        TEXT    NOT NULL UNIQUE,
        template_id INTEGER NOT NULL REFERENCES templates(id),
        mpp         INTEGER NOT NULL UNIQUE CHECK (mpp BETWEEN 0x100 AND 0x8FF),
        source      TEXT    NOT NULL DEFAULT '{}', -- JSON, shape below
        static      TEXT    NOT NULL DEFAULT '{}'  -- JSON object slot -> value, cells 0-127
    );

Migration `0006_runs.sql`:

    CREATE TABLE runs (
        id          INTEGER PRIMARY KEY AUTOINCREMENT,
        binding_id  INTEGER NOT NULL REFERENCES bindings(id) ON DELETE CASCADE,
        actor       TEXT    NOT NULL,   -- 'schedule' or the user:<id> who pressed Run now
        started_at  TEXT    NOT NULL,
        finished_at TEXT    NOT NULL,
        outcome     TEXT    NOT NULL CHECK (outcome IN ('ok', 'failed')),
        error       TEXT    NOT NULL DEFAULT ''
    );
    CREATE INDEX runs_binding ON runs (binding_id, id);

There is no foreign key from `pages.binding_id`; the store keeps it equal to
the owning binding's id. A template referenced by a binding cannot be deleted
(`ErrInUse`, 409). The run log keeps the newest 100 rows per binding (D-020).

**Binding lifecycle.** `CreateBinding` inserts the row and, in the same
transaction, attaches the page: an existing unbound page gets `binding_id`
set; a bound page is `ErrBound` (409); an absent page is created as
`tti.NewPage(mpp)` with `binding_id` set, so a bound page always exists. A
binding's `mpp` is immutable; moving a binding is delete and recreate.
`DeleteBinding` sets `binding_id` to NULL on its page, which keeps its last
content and becomes editable, then deletes the row; its runs cascade.
Audit actions `template.create|update|delete` and
`binding.create|update|delete`, targets `template:<id>` and `binding:<id>`,
detail the name only, never `source` or `static`.

**Amendment to ADR 0004: a fourth page writer and a fifth audit action.**
`SaveGenerated(bindingID int64, p *tti.Page, a Audit) (changed bool, err
error)` joins `SavePage`, `CreatePage` and `DeletePage` as the only writers
of `pages` after import. It reads `mpp` from `bindings[id]` (`ErrNotFound`),
requires `p.MPP()` to equal it, requires `pages[mpp].binding_id == id`
(`ErrNotBound`), sets `a.Actor = "job:binding:<id>"` and
`a.Action = "page.generate"` regardless of the caller's values, and then
runs the body `SavePage` runs after `lockedRow`: identical bytes write
nothing; otherwise archive to `page_versions`, rewrite every derived column,
one audit row with `page:<MPP>` and `sha256:<hex>`. That body is lifted into
one unexported function both writers call. `lockedRow`, and so `SavePage`
and `DeletePage` for humans and the API, are unchanged: a bound page still
answers 409. `page.generate` joins the four actions of ADR 0004 as the fifth
and last page audit action. ADR 0004's sentence "M3's generated-page writes
reuse `SavePage` with their own action" is satisfied by the shared body, not
by a call to `SavePage`.

**Rendering (`internal/tmpl`, imports only `tti`).** `Parse(ttiBytes,
rowRules)` validates with `tti.Parse` and scans the decoded cells of `OL`
rows 1-24 of every subpage with
`\{([\^>]?)([a-z][a-z0-9_-]*)(\.*)\}`; width is the match length, minimum 3;
`row_rules` keys are `"1"`-`"24"` and values existing slot names. `Clean`
drops cells below 0x20 and 0x7F, maps `£` to 0x23 and any other non-ASCII
rune to `?`. `Render(mpp, static, data)` passes `data` through `Clean`,
requires `static` values to be cells 0-127, errors on a key in both maps, on
a slot with no key, and ignores extra keys. For every subpage it rewrites
each `PN` body to `%03X` of `mpp` plus the subpage digits; writes every row
1-24 through `SetRow` so the page's control-code form is kept: an absent row
becomes 40 spaces; a slot's value is clamped to its width then padded by
alignment (centre puts the odd space on the right); a row with slots is
blanked when every slot value was empty before padding; a `row_rules` entry
alone decides its row, blanking exactly when the named slot's value is
empty; rows with neither are kept as they are. Rows 0 and 25-29 and every
non-`OL` line pass through, so the page transmits iff the template's `PS`
does. The result parses and round-trips.

**Sheets source (`internal/jobs`).** Config key `googleKeyFile` (string,
optional). Empty means the source is `Unconfigured`, whose `Read` returns
`ErrNoKey`; set, the path must be a regular file at start (error naming
`googleKeyFile`) and `sheets.NewService(option.WithCredentialsFile(path),
option.WithScopes(SpreadsheetsReadonlyScope))` is built at start so a
malformed key fails fast; only the library ever reads the file.

    type Rows struct{ Formatted [][]string; Raw [][]any }
    type Source interface{ Read(ctx context.Context, sheetID, rng string) (Rows, error) }

`Read` performs two `Values.Get` calls, `FORMATTED_VALUE` and
`UNFORMATTED_VALUE` with `SERIAL_NUMBER`, over a range without a sheet name
(the API resolves the first sheet; no metadata call). Errors are classified
to fixed texts: `*googleapi.Error` 404/403 is `ErrSheetNotFound`, other codes
`ErrUnreachable` with code and message; `*url.Error` is `ErrUnreachable`
with the inner error only; any other error type (including the token
exchange's) is `ErrUnreachable` with the Go type name only; a response with
no values is `ErrEmpty`. Every text that came from the library has the sheet
id replaced by `<sheet>`. The binding's `source` JSON is

    {"sheet": "<id>", "range": "C5:E", "columns": ["p#name","p#score","p#date"],
     "rows": 8, "format": {"p#date": "date"}}

decoded with unknown keys refused. `Values(rows, cfg)` applies the old
generator's rule: a row is complete when every configured column's formatted
text is non-empty; leading incomplete rows are skipped; the first incomplete
row after a complete one ends the entries; at most `rows` entries; every key
`column x 1..rows` is present, empty when there is no entry, so blanking
works. `ErrMissingColumns` when no row has every configured column. `format`
names entries in a registry `formats map[string]func(formatted string, raw
any) (string, error)`; M3 ships `text`; the Port packet adds `date` and
`number`. The scheduler adds `data["updated"]` as `02 Jan 15:04` in
host-local time (D-020).

**Scheduler (`internal/jobs.Runner`).** One goroutine: run every binding once
at start, then at every top of the hour on the wall clock, in id order, under
one mutex. `RunNow(ctx, id, actor)` takes the mutex with `TryLock` and
returns `ErrBusy` (409 `run_in_progress`) when a run is in progress. A run,
with a 60 s timeout, parses the template, reads the source, maps values,
renders in memory, sets `DE` to the binding's name, calls `SaveGenerated`,
then inserts one `runs` row whatever happened. Nothing is written before
`SaveGenerated`, so an error anywhere earlier leaves the previous page row,
its `updated_at`, audit and versions untouched. Logs name the binding, never
the sheet id or the `source` JSON.

**Amendment to ADR 0001: import direction.** `httpapi` may import `jobs`
(for `ParseSourceConfig` and `Runner.RunNow`). `jobs` must never import
`httpapi`. The rest of the direction is unchanged: `httpapi` and `jobs` use
`store`, `auth`, `output` and `tmpl`; `store` uses `tti`; `tmpl` imports only
`tti`.

**HTTP.** A second route helper beside `pageRoute`, `adminRoute(mux,
pattern, h)`: a principal without `Admin` gets 403 `forbidden`. Every
template, binding and run route registers through it and no other way; keys
are never admin (ADR 0003), so it refuses them once M4 adds them. Routes:
`GET|POST /api/v1/templates`, `GET|PUT|DELETE /api/v1/templates/{id}`,
`GET /api/v1/templates/{id}/tti`; `GET|POST /api/v1/bindings`,
`GET|PUT|DELETE /api/v1/bindings/{id}`, `POST /api/v1/bindings/{id}/run`,
`GET /api/v1/bindings/{id}/runs`. The binding list omits `source`; the single
binding document returns it to the admin who entered it. The page document
gains `"binding": {"id","name"} | null`; the editor shows a bound page
read-only. How static values are entered in the UI is open (Q-013) and not
decided here.

**Weight.** The templates packet is `critical` (D-020) because `adminRoute`
is authorisation; its mutation checks gate.

## Consequences

- Six high-score pages become ordinary rows in `pages`, so the M2 output
  writer, downloads, versions and the page list need no generated-page code.
- Hourly runs rewrite each generated page because the `updated` text
  changes, so `page_versions` for those pages holds about the last 20 hours.
- The Sheets dependency adds roughly 25 indirect modules and some megabytes
  to the Pi binary; it is the only M3 module addition.
- Templates are uploaded as `.tti` files, not edited row by row in the
  browser; the workflow is edit a page, download, upload as template.
- Mosaic bytes that happen to read as `{abc}` make a template fail its render
  until the row is changed; the template document lists parsed slots so this
  is visible at upload.
- Frozen, changed only by a new ADR:
  - the `templates`, `bindings` and `runs` columns above; the 100-per-binding
    run cap; a binding's `mpp` immutable; `CreateBinding` attaches or creates
    the page and `DeleteBinding` detaches it, both in one transaction;
  - `SaveGenerated` as the fourth and last writer of `pages` after import,
    sharing `SavePage`'s body, reading `mpp` from the binding row, refusing a
    page for another `mpp` or an unbound page, and setting actor
    `job:binding:<id>` and action `page.generate` itself; `lockedRow`
    unchanged for `SavePage` and `DeletePage`;
  - `page.generate` as the fifth page audit action, with ADR 0004's target and
    detail forms; the template and binding audit actions with name-only detail;
  - the slot grammar's regex form, `Clean`, the two-map `Render` contract
    (static may carry control codes, data is cleaned, a key in both or a slot
    without a key fails, extra keys ignored), every row 1-24 written, `PN`
    rewritten, everything else passed through;
  - `googleKeyFile` as the only place the key is referenced; the key read
    only by the Sheets library; `Source` and `Rows` as the scheduler's
    boundary; error classification to fixed texts with the sheet id scrubbed;
    the `source` JSON keys `sheet`, `range`, `columns`, `rows`, `format` and
    the `#` convention; the complete-row rule of `Values`;
  - one runner, one mutex, runs at start and at each top of the hour,
    `TryLock` for run-now, one `runs` row per run written after the page
    write, nothing written before `SaveGenerated`;
  - `adminRoute` as the only way an admin route is registered; the route list
    above; the page document's `binding` field;
  - `httpapi` may import `jobs`; `jobs` never imports `httpapi`.
- Mutation checks later packets must keep green: a missing slot key fails the
  render; `SaveGenerated` refuses a wrong `mpp` and an unbound page and
  ignores the caller's actor; a human write to a bound page is 409; a
  non-admin gets 403 on every admin route; a failed fetch or a render error
  after fetch leaves the page row, audit and versions unchanged; a second
  run-now during a run is 409; no log line, audit row, run row or database
  byte holds key material or a sheet id; deleting a binding detaches its page.
- Open and not decided here: Q-013 (static-value entry in the UI); the Port
  packet's golden file is a reviewed expected output, not the old binary's
  (ADR 0002 makes them differ by design: blanked rather than omitted rows,
  row 7 held to 40 cells).

## Amendment 2026-10-12 (D-028): a binding's page moves with it

`UpdateBinding` may change a binding's `mpp`. It then becomes a writer of
`pages`: it changes the row's `mpp`, plus bytes renumbered in `PN` only,
never content, through the shared `tti.SetMPP`. This replaces "a binding's
`mpp` is immutable". A move onto any existing page returns `ErrExists`
(409 `exists`) and changes nothing. `page_versions` rows follow the
number; no archive row is written for the move. Each move writes a
`binding.move` audit row with detail `OLD->NEW`. A move can take a page
across grant ranges. ADR 0004's five page actions and four content writers
are unchanged.

## Amendment 2026-10-23 (D-060): templates can be replaced in place

This replaces the consequence "the workflow is edit a page, download,
upload as template". `PUT /api/v1/templates/{id}` replaces a template's
name, file and row rules in place, and the Templates screen offers it as
Replace. The update is refused with 409 `in_use`, before anything is
written, when a binding using the template would be left with a slot it
fills from neither its sheet columns nor its fixed values. The message
names each such binding, its page and its missing slots. Removing a slot,
or changing a slot's width, alignment or position, is never refused. No
store, runner or route changes; the frozen items above are unchanged. A
binding saved at the same moment as its template's update can slip past
the check; the worst outcome is one failed run that keeps the previous
page.
