# 0002 — Page storage and output pipeline

Status: `Accepted`
Date: `2026-10-11`
Raised by: `coordinator`
Decided by: `owner` (record in `docs/decisions.md` as D-nnn)

## Context

D-003: SQLite is the record, TTI is the import, export and output format, and a
downloaded page must re-upload without loss. D-004: a page's magazine is the
first digit of its number. D-005: rows are edited as text, mosaics elsewhere,
so files from other editors arrive by upload. D-008: vbit2 is unmodified; this
app only writes its pages directory. TTI files carry lines this app does not
edit (`DS`, `OL,28` in the old generator's template), mix 7-bit `ESC` and 8-bit
control codes, and may use CRLF or LF. The current generator replaces bare
tokens (`main.go:380-412`), centres or pads players 1-3 and 4-8 to 9 cells
(`main.go:393-400`), and drops rows 17-22 by entry count (`main.go:430-439`).
It does not hold the grid: `game-name` is a 9-cell token given up to 32
characters (`main.go:381`, template line 14). Bare tokens also collide with
vbit2's own header substitutions such as `mpp` (template line 8). It writes
files in place, not atomically (`main.go:157`).

## Options

1. **Canonical TTI bytes per page plus derived index columns.** Byte identity
   holds by construction; the codec edits only the lines that change. Cost:
   queries over row content need the codec, which nothing needs yet.
2. **Fully normalised rows** (page, subpage, row, metadata tables). Queryable,
   but byte identity requires recording line order, endings, unknown lines and
   escape form per cell, which rebuilds option 1 in a harder shape.

## Decision

Option 1.

**Storage.** `pages(mpp INTEGER PRIMARY KEY CHECK (mpp BETWEEN 0x100 AND
0x8FF), tti BLOB NOT NULL, description TEXT, subpages INTEGER, transmit
INTEGER, binding_id INTEGER NULL, updated_at TEXT, updated_by TEXT)`. `mpp` is
magazine x 256 + page; the magazine is `mpp >> 8` and is not stored. The API
and UI write it as three uppercase hex digits (`4A0`), accepted in either case.
The BLOB is the truth. Index columns are re-derived from it by the codec in the
same transaction as every write. `transmit` is 1 when any subpage's `PS` has
bit 15 set. A carousel is one row; its `PN` lines are its subpages.

**Codec contract.** A page is an ordered list of raw lines, each keeping its
bytes and its terminator (CRLF, LF, or none at end of file). Serialising
concatenates them. Lines before the first `PN` belong to the page; later lines
belong to the preceding subpage. Unknown codes are kept, not rejected. A
malformed known line (bad `PN`, `OL` row outside 0-29, more than 40 cells)
fails with its line number. Cells decode as: byte >= 0x80 gives `b & 0x7F`;
`ESC x` gives `x - 0x40`; any other byte is itself. Every `PN` line in a page
must carry the same `mpp`. An edit rewrites only the lines whose content
changed, and encodes control codes in the form the page first uses (`ESC` if
it has none). Saving identical bytes writes nothing. In JSON a row is a string
whose code points are the cell values 0-127.

**Upload and import.** The uploaded `mpp` must equal the target page; a
mismatch is refused, naming both. The file is never rewritten to match. Import
refuses two files with the same `mpp`.

**Templates and generated pages.** A template is stored apart from pages:
`templates(id, name, tti BLOB, row_rules)`, not transmitted, no `mpp`. A binding
(M3) joins a template, a data source, static values and one target `mpp`. The
rendered result is stored as an ordinary page row with `binding_id` set,
through the same store write as an edit, so the output writer sees only pages.
Rendering rewrites every `PN` to the binding's `mpp`, keeping subpage numbers.
A human or API write to a page with `binding_id` is refused (409). Removing the
binding detaches the page, which keeps its last content.

**Slot syntax.** A slot is `{` + optional alignment (`^` centre, `>` right,
default left) + name `[a-z][a-z0-9_-]*` + zero or more `.` + `}`, matched on
the decoded cells of rows 1-24 only. Its width is its own cell count, braces
included (minimum 3). `{p1name.}` is 9 cells; `{gname` + 21 dots + `}` is 32.
At render:
- every slot name must have a value key, even an empty one; a missing key
  fails the render and keeps the previous page, so a typo, or mosaic bytes that
  happen to look like a slot, fail loudly;
- values from a data source are untrusted: cells below 0x20 and 0x7F are
  stripped, `£` maps to 0x23, other non-ASCII becomes `?`;
- static binding values set by an admin may contain control codes (the
  per-game title colours of `main.go:452-487` become a 3-cell slot);
- the value is clamped to the width, then padded with spaces by alignment
  (centring puts the odd space on the right, as `main.go:640-643` does). No
  cell to the right of a slot ever moves;
- a row containing slots is blanked (its `OL` line written as 40 spaces)
  when every slot value was empty before padding. A template's `row_rules`
  may name one slot per row (`{"17":"p4name"}`); that row is then blanked
  exactly when that slot is empty. This covers the slot-less separator row 17.
  Rows with neither are kept. A blanked row is never omitted: the owner has
  observed that a row absent from the file leaves the previous content on the
  television (D-009), so every row 1 to 24 of a generated page is written;
- the output is encoded in the template's control-code form.

Compared with the old generator: with zero entries, rows 13, 15 and 16 are
blanked (the old generator also sent them blank), rows 17 to 22 are blanked
rather than omitted, and row 7 is held to 40 cells.

**Output writer contract.** One goroutine owns `pagesDir`:
- Ownership: at start, if `pagesDir` is missing or empty, create it with a
  marker file `.ceems-owned`. If it holds files and no marker, refuse to
  start and name the directory. This guards against a mistyped path to the
  owner's real page folder.
- Reconcile: the wanted set is every page with `transmit = 1`, as
  `P<MPP>.tti` (uppercase hex) holding the page BLOB exactly. Write a file
  only when its bytes differ: write `.P<MPP>.tti.tmp` in the same directory,
  then rename over the target. Delete every `*.tti` and `.*.tmp` not in the
  wanted set. shortcut: no fsync, because the tmpfs is rebuilt from SQLite at
  every start.
- Triggers: once at start, then after every committed page change. Changes
  are debounced: reconcile 500 ms after the last change, at most 2 s after
  the first in a burst. Runs never overlap.
- Failure is logged and retried on the next trigger. It never rolls back the
  database write.

**Open verification items** (M2 "vbit2 reload evidence"; not decisions):
- V-1: does vbit2 reload a changed file on disk by itself, and how fast?
  Inference for yes: the old generator overwrites files hourly
  (`main.go:157`), reportedly with effect; `filemonitor.cpp` exists but was
  not read for this ADR. If no, the writer gains a post-reconcile command to
  the TCP control interface (default port 1992). The contract above is
  unchanged.
- V-2: does vbit2 pick up new files and drop deleted ones without a restart?
- V-3: does vbit2 skip `.P<MPP>.tti.tmp`? If it reads every file, temp files
  move to a sibling directory on the same filesystem.
- V-4: on Windows, does a rename over a file vbit2 holds open succeed? If not,
  the writer retries on the next trigger.

## Consequences

- Download is the stored BLOB, so byte identity needs no special code path.
- Search inside row text needs the codec; there is no SQL over rows.
- Frozen: the `mpp` integer encoding and its hex form; the BLOB as truth with
  derived columns rewritten in the same transaction; the slot grammar and
  width rule; generated pages refuse direct writes; the writer owns only a
  directory carrying `.ceems-owned`; `P<MPP>.tti` naming.

## Amendment, 2026-10-24: the writer also writes `vbit.conf`

Under D-061 the ownership marker above is `.ceems-owned`; until then it was
named after the earlier product name.

When the `vbit_conf` setting is on (the default), each reconcile also renders
`vbit.conf` from the settings table and writes it only if its bytes differ from
the file on disk, through the same `.vbit.conf.tmp`-then-rename path as a page.
It is therefore written at start, after every settings save (the save calls the
store's change notification), and whenever it differs on a later reconcile. The
file holds two comment lines and then `header_template`,
`initial_teletext_page`, `row_adaptive_mode`, `lines_per_field`,
`magazine_priority` and `status_display`; the header template is written in
7-bit form (a cell below 0x20 becomes ESC followed by the cell plus 0x40).

When `vbit_conf` is off the writer never writes, creates or deletes
`vbit.conf`. It never reads, writes or deletes `vbit.conf.override`. The
deletion rule is unchanged: a reconcile removes only `*.tti` files and
`.*.tmp` leftovers.
