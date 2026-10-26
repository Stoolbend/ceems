# 0004 — Page write path, versions and concurrency

Status: `Accepted`
Date: `2026-10-12`
Raised by: `packet-architect` (decision record for T-009, T-010, T-011)
Decided by: `owner` (recorded in `docs/decisions.md` as D-016)

## Context

ADR 0002 makes the page BLOB the truth, rewrites the derived columns in the
same transaction as every write, requires that saving identical bytes writes
nothing, and refuses a human or API write to a page with `binding_id` (409).
ADR 0003 requires an audit row in the same transaction as every page create,
update, upload and delete, carrying the SHA-256 of the new bytes, and allows
page routes only through `pageRoute`. D-009 (Q-005 b) keeps the previous
bytes of a page in `page_versions`, the last 20 per page, on every overwrite
and delete. D-013 deferred that table and the page audit rows to the first
packet that overwrites or deletes.

Three M1 packets write pages: the row editor (T-009), TTI upload (T-010) and
create/delete (T-011). The plan requires that two editors do not silently
overwrite each other and that a delete needs a confirmation token
(`docs/plan.md`, M1 "Create and delete a page"). The store serialises every
query on one connection (`internal/store/store.go:44`, `SetMaxOpenConns(1)`),
so a check-then-write inside one transaction is race-free. The import path
already derives every column from a parsed `*tti.Page` in one unexported
statement (`internal/store/store.go:200-206`), and T-007 provides the single
audit writer `insertAudit` and `store.Audit{Actor, Action, Target, Detail, Remote}`.

## Options

Write path:
1. **One migration and three store functions** (`SavePage`, `CreatePage`,
   `DeletePage`) sharing an unexported archive step and the existing insert
   and audit helpers. HTTP maps store errors and never re-checks the rules.
2. **A write per packet**: each handler composes its own transaction. Three
   copies of the version, derive and audit steps, each a place the invariants
   can drift.

Concurrency:
1. **SHA-256 of the stored bytes as an etag**, checked inside the write
   transaction against an `If-Match` header. Already computed for the audit
   detail; distinguishes two saves in the same second.
2. **`updated_at`**: cheaper, but second-granularity and not content-bound.
3. **None**: last write wins; `page_versions` is the only recovery. Adding a
   check later would change the PUT contract.

Delete confirmation:
1. **The etag, required in `If-Match`**: no new state; a caller must have read
   the page, and the token is bound to the content being deleted.
2. **A one-time token from a dedicated route and table**: equally effective,
   plus a table, a route and expiry logic.

## Decision

Option 1 in each case.

**Versions.** Migration `0004_page_versions.sql`:

    CREATE TABLE page_versions (
        id         INTEGER PRIMARY KEY AUTOINCREMENT,
        mpp        INTEGER NOT NULL,
        tti        BLOB    NOT NULL,
        updated_at TEXT    NOT NULL,   -- the retired row's own updated_at
        updated_by TEXT    NOT NULL    -- and updated_by
    );
    CREATE INDEX page_versions_mpp ON page_versions (mpp, id);

No foreign key to `pages`: versions outlive a delete. Who retired a version
and when is the audit row of the same transaction. Before every overwrite or
delete the current `pages` row is copied here, then the rows for that `mpp`
are pruned to the newest 20 by `id`.

**Store write functions** (`internal/store`), the only code that changes
`pages` after import:

    func (p *Page) ETag() string   // SHA-256 of the stored bytes, 64 lowercase hex
    func (s *Store) SavePage(mpp int, p *tti.Page, ifMatch string, a Audit) (changed bool, err error)
    func (s *Store) CreatePage(p *tti.Page, a Audit) error
    func (s *Store) DeletePage(mpp int, ifMatch string, a Audit) error

Each runs in one transaction. `SavePage` returns `ErrNotFound` for an absent
page, `ErrGenerated` when `binding_id` is set, `ErrStale` when `ifMatch` is
non-empty and differs from the stored etag, and an error when `p.MPP()` is
not `mpp`. When the new bytes equal the stored bytes no statement runs and
`changed` is false. Otherwise it archives, rewrites every derived column
(`description`, `subpages`, `transmit`, `updated_at`, `updated_by = a.Actor`)
from the parsed page, and writes one audit row. `CreatePage` inserts through
the import helper and audits; `ErrExists` when the `mpp` is stored.
`DeletePage` archives, deletes and audits, with the same `ErrNotFound`,
`ErrGenerated` and `ErrStale` rules. An empty `ifMatch` means no check in
every function; requiring one is an HTTP decision (below).

**Audit actions.** The caller supplies `Actor`, `Action` and `Remote`; the
store sets `Target` to `page:<MPP>` (three uppercase hex digits) and `Detail`
to `sha256:<hex>` of the bytes written, or for a delete of the bytes removed.
Actions are exactly `page.create`, `page.update` (row editor), `page.upload`
and `page.delete`. M3's generated-page writes reuse `SavePage` with their own
action; nothing writes `pages` another way.

**Etag and `If-Match`.** `GET /api/v1/pages/{mpp}` returns `"etag"`. The
write routes `PUT /api/v1/pages/{mpp}` (rows document), `PUT
/api/v1/pages/{mpp}/tti` (upload) and `DELETE /api/v1/pages/{mpp}` read an
optional `If-Match` header holding that value, surrounding double quotes
stripped. A mismatch is 412 `precondition_failed` and writes nothing. On the
two PUTs an absent header means no check, so a script may replace a page
without a prior read; `page_versions` keeps what it replaced. The browser
editor always sends it; the browser upload does not, because an upload is a
deliberate wholesale replacement.

**Delete confirmation.** `DELETE` without `If-Match` is 428
`confirmation_required` and reads nothing. The etag is the confirmation
token: it cannot be produced without reading the page, and it binds the
delete to the exact content the caller saw, so a stray, replayed or stale
request cannot delete. The handler's empty check is the only "no token"
guard, so one mutation covers it.

**Create and delete routes.** `POST /api/v1/pages/{mpp}` creates and `DELETE
/api/v1/pages/{mpp}` deletes, both through `pageRoute` with `write = true`.
Whoever `Can(mpp, write)` may create or delete; there is no admin check
(D-016). A number outside magazines 1-8 never reaches a handler: `pageRoute`
answers 404. A taken number is 409 `exists`.

**Blank page** (D-016). `tti.NewPage(mpp)` returns, with CRLF terminators:

    DE,
    PN,<MPP>00
    SC,0000
    PS,0000
    CT,8,T
    FL,8ff,8ff,8ff,8ff,8ff,8ff
    OL,1,<40 spaces>  …  OL,24,<40 spaces>

Transmit is off so creating never changes what is on air; every body row is
written as 40 spaces so the first transmit replaces whatever the television
last showed at that number (D-009); there is no `OL,0` because the header is
vbit2's (D-014).

**Other error mapping**, shared by every page write: `ErrGenerated` 409
`generated_page`; `tti.Parse` failures 400 `invalid_tti` with the codec
message verbatim; an upload whose `PN` differs from the target 400
`mpp_mismatch` naming both numbers; bodies over 256 KiB 413 `too_large`.

## Consequences

- Every page write, including M3's renderer and M4's REST API, goes through
  `SavePage`, `CreatePage` or `DeletePage`, so the version, derive, audit,
  identical-bytes and generated-page invariants live in one place and one
  store test suite proves them.
- M4's OpenAPI document inherits `etag`, `If-Match`, 412 and 428 as written
  here; revision history and undo (unscheduled) can be built over
  `page_versions` without a schema change.
- Versions are capped at 20 per page; older content is gone. A page deleted
  and recreated starts a new version history under the same `mpp`.
- Concurrency control is opt-in for API callers on the two PUTs. A client
  that omits `If-Match` can overwrite a concurrent edit; the previous bytes
  survive in `page_versions`.
- Frozen, changed only by a new ADR: the `page_versions` columns and the
  20-row cap; the three store functions as the only writers of `pages` after
  import; the four audit actions and the `page:<MPP>` / `sha256:<hex>`
  target and detail forms; the etag as SHA-256 hex of the stored bytes; the
  `If-Match` / 412 contract and 428 on a tokenless delete; `POST` and
  `DELETE /api/v1/pages/{mpp}` through `pageRoute` with write access as the
  only permission rule; the blank page contents above.
- Mutation checks later packets must keep green: a delete without `If-Match`
  fails a test; an overwrite or delete without an archive or audit row fails
  a test; identical bytes leave no audit or version row; a write to a page
  with `binding_id` is refused.
- If T-007 integrates with different names for `insertAudit`, `Audit` or
  `pageRoute`, the packets follow the integrated code; the contract above is
  unchanged.
