# Decisions

Owner and coordinator decisions, newest last. A decision recorded here is what
a packet may cite as authority; an answer given only in chat is not. Lasting
design decisions get an ADR under `docs/adr/` as well, linked from the entry.

To reverse a decision, add a new entry that names the one it supersedes, and
mark the old entry `Superseded by D-<nnn>`. Do not edit or delete old entries.

## Open questions

Questions waiting for the owner. Each is numbered, has lettered options and a
recommendation, and says whether it blocks work. Move it to the log once
answered.

<!--
### Q-001 — <question>

Blocking: <yes, which task ids / no>
Raised: <YYYY-MM-DD> by <role>

- a) <option>
- b) <option>

Recommendation: <letter>, because <reason>.
-->

## Log

<!--
### D-001 — <decision in one line>

Date: <YYYY-MM-DD>
Decided by: <owner / coordinator>
Answers: <Q-nnn, or the packet that raised it>
ADR: <docs/adr/nnnn-title.md, or none>

<What was decided and the reason, in a few sentences. Name what it rules out.>
-->

### D-001 — Target hardware is a Raspberry Pi 4 with upstream raspi-teletext

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 1 (chat, 2026-10-11)
ADR: none

Pi 4, Raspberry Pi OS Bookworm Lite 64-bit, composite on the 3.5 mm jack,
legacy graphics driver as vbit2 and raspi-teletext require. Rules out the Pi 5
(unsupported by vbit2 and raspi-teletext upstream; the nutki/raspi5-teletext
fork needs a patched kernel driver and soldered pads) and Raspberry Pi OS
Trixie (the vbit2 install script does not support it). The owner has no Pi 4
yet, so every hardware criterion in `docs/plan.md` stays unticked until one
exists; everything else must be provable on Windows.

### D-002 — Stack is Go, Vue 3 and SQLite in one static binary

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 2 (chat, 2026-10-11)
ADR: docs/adr/0001-stack-and-runtime-shape.md

Backend in Go (the existing high-score generator at
the old high-score generator is Go and its logic is reused).
Web UI in Vue 3, because that is the owner's primary experience. SQLite via a
pure-Go driver so `CGO_ENABLED=0` cross-compiles to `linux/arm64` and
`windows/amd64` from the owner's PC. The built Vue app is embedded in the Go
binary. Web server, REST API, scheduler and page renderer are one process on
the Pi. The owner accepts a performance cost for readability, provided
automated tests cover behaviour. Rules out Rust, Python and Node for the
server and any library that needs cgo.

### D-003 — SQLite is the source of truth; TTI is the import, export and output format

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 3 (chat, 2026-10-11)
ADR: docs/adr/0002-page-storage-and-output-pipeline.md

Pages live in SQLite. Every existing page is a `.tti` file and is imported
once. Each page can be downloaded as TTI and re-uploaded to replace it, so a
page must round-trip through the codec without loss. The app writes the
transmit set as `.tti` files into the directory vbit2 reads (a tmpfs on the
Pi, regenerated at start) and vbit2 continues to run unmodified. Rules out
editing `.tti` files in place as the record.

### D-004 — Pages are grouped by magazine, which the page number dictates

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 4 (chat, 2026-10-11)
ADR: none

The brief's "catalogue" means magazine (the first digit of the page number,
1 to 8). A page belongs to exactly one magazine. There is no separate
grouping entity to model.

### D-005 — First editor is row text editing with preview, plus TTI download and upload

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 5 (chat, 2026-10-11)
ADR: none

The first release edits the 40-column rows as text with a faithful Level 1
preview and no mosaic drawing. Because mosaics cannot be drawn in the app, a
page must be downloadable as TTI and re-uploadable after editing elsewhere.
A visual mosaic editor is a later milestone, not a first-release item.

### D-006 — Named users with per-magazine or per-page grants; the API may face the internet later

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 7 (chat, 2026-10-11)
ADR: docs/adr/0003-users-permissions-and-api-keys.md

The web UI has multiple named users. A user can be granted management of
whole magazines or of individual pages or ranges. API keys carry the same
shape of grant plus read-only or read-write. The REST API may be exposed to
the internet in future for integrations, so it is designed as if
internet-facing from the start. Per `AGENTS.md`, all of this is `critical`
weight.

### D-007 — Cross-compile on the owner's PC; Windows build for local testing

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 8 (chat, 2026-10-11)
ADR: none

Builds are produced on the owner's Windows PC for `linux/arm64` (Pi 4) and
`windows/amd64` (local testing, since no hardware exists yet), then copied to
the Pi over ZeroTier. vbit2 runs on Windows, so the output pipeline can be
exercised locally against a real vbit2 before any Pi exists. Rules out
building on the Pi and containers.

### D-008 — Non-goals for the first release

Date: 2026-10-11
Decided by: owner
Answers: coordinator question 9 (chat, 2026-10-11)
ADR: none

vbit2 and raspi-teletext are not modified or replaced; this project writes
their page directory only. In-vision output (vbit-iv) is out of scope. Fastext
links are the per-page `FL` line and are edited as page metadata, not a
feature of their own.

### D-009 — Answers to Q-001 to Q-008

Date: 2026-10-11
Decided by: owner
Answers: Q-001 to Q-008
ADR: amends the draft of docs/adr/0002-page-storage-and-output-pipeline.md

- Q-001 a: the Google service-account key is a file on the Pi, referenced by
  path in the config; never stored in the database or accepted by the UI.
- Q-002 a: the owner will copy the current `.tti` page set, sanitised, into
  `testdata/pages/`. Until it lands, the M1 import packet and the renderer
  tests cannot be dispatched.
- Q-003 a: the repository is hosted at
  https://github.com/Stoolbend/ceems with one GitHub Actions workflow
  running the Go and Vue tests on push.
- Q-004 a: admins only create API keys.
- Q-005 b: the previous bytes of a page are kept in a `page_versions` table,
  the last 20 per page, on every overwrite and delete.
- Q-006 a: admins only edit templates and bindings.
- Q-007 a: sessions last 14 days with no idle timeout.
- Q-008 a, with a correction to the draft: a row the template logic would
  drop must be written as a blank `OL` line, never omitted. The owner has
  observed that a row absent from the updated file leaves the old content on
  the television. ADR 0002's slot rule now blanks rows instead of omitting
  them. Rules out omitting any row 1 to 24 from a generated page.

### D-010 — ADRs 0001 to 0003 accepted; Go stays the backend language

Date: 2026-10-11
Decided by: owner
Answers: Q-009
ADR: docs/adr/0001-stack-and-runtime-shape.md,
docs/adr/0002-page-storage-and-output-pipeline.md,
docs/adr/0003-users-permissions-and-api-keys.md

All three ADRs are accepted as drafted, with the D-009 blank-row amendment to
ADR 0002. The owner weighed Go against Node with TypeScript (one language with
the Vue UI) and chose Go for single-binary cross-compiled deployment, a
cgo-free SQLite driver and a standard-library-only server. Their frozen
conventions now bind every packet.

### D-011 — A page transmits when any PS line in its file has bit 15 set

Date: 2026-10-11
Decided by: coordinator
Answers: T-003 design record (packet-architect)
ADR: clarifies docs/adr/0002-page-storage-and-output-pipeline.md, no change

ADR 0002 derives `transmit` from "any subpage's PS". `testdata/pages/P701.tti`
has its only `PS,8000` at line 2, before its first `PN` at line 3, so under
the codec contract that line belongs to the page, not a subpage, and a literal
reading would store a live transmitted page as non-transmit. The codec reads
every `PS` line in the file. If vbit2 is found to treat a page-level `PS`
differently, M2 reload evidence revisits this.

### D-012 — Critical M1 packets may be dispatched without a further stop

Date: 2026-10-11
Decided by: owner
Answers: coordinator question in chat, 2026-10-11
ADR: none

The owner authorised dispatching the two `critical` M1 packets (admin
bootstrap and login; create and delete a page) without stopping to ask
first. Each still gets the full `critical` process from `AGENTS.md`:
architect design before dispatch, conformance review, and a mutation check
for every rejection or permission rule it asserts. The coordinator reports
after each integration. This covers M1 only; M3 and M4 critical packets
still need the owner's go-ahead.

### D-013 — Import refuses a file that differs from the stored page

Date: 2026-10-11
Decided by: owner
Answers: T-005 design record (packet-architect), asked in chat 2026-10-11
ADR: none

When `ceems import` meets a `.tti` file whose page number is already
stored with different bytes, it refuses the whole import, naming the file
and the page, and writes nothing. A file equal to the stored page is
skipped as unchanged. A one-off import never overwrites a page edited in the
browser; pages are replaced through upload, or deleted and re-imported.
Consequently `page_versions` (D-009) and the `audit` table arrive with the
first packet that overwrites or deletes a page, not with the import. Rules
out overwrite-on-import and skip-with-warning.

### D-014 — Preview header: fixed template, spaces in cells 0-7, keep the gallery

Date: 2026-10-11
Decided by: owner
Answers: Q-010, Q-011, Q-012 (raised from the T-006 architect record)
ADR: none

- Q-010 a: the preview renders row 0 from the fixed template
  `        CeeMS      mpp        %H:%M:%S` for every page, ignoring the
  page's own `OL,0`. Revisit after M2 vbit2 reload evidence shows what vbit2
  transmits; a `headerTemplate` config key is the likely next step.
- Q-011 a, provisional: header cells 0-7 show the template's spaces. The
  owner wants this checked on a real television once a Pi 4 exists (D-001);
  the M2 deployment criteria are where that check happens.
- Q-012 a: the dev-only `/dev/gallery` route stays after review.

### D-015 — Login details: gallery behind login, case-insensitive usernames, stdlib cross-origin check; audit lands with login

Date: 2026-10-11
Decided by: owner (first three), coordinator (the audit clarification)
Answers: T-007 design record escalations 1-4 (packet-architect), asked in chat 2026-10-11
ADR: amends docs/adr/0003-users-permissions-and-api-keys.md in two details, noted below

- The dev-only `/dev/gallery` route requires a login like every other app
  route; only `/login` is public.
- Usernames are 1-64 characters of letters, digits, `.`, `_` or `-`, and are
  case-insensitive: stored and compared lowercased, so `Conor` and `conor`
  are one user. Rules out case-sensitive usernames.
- The cross-origin check is Go's `http.CrossOriginProtection` (Go floor is
  1.26.2), which checks every unsafe request, not only cookie requests as
  ADR 0003 worded it. This retires ADR 0003's hand-rolled `shortcut:` note.
  M4 decides whether bearer-key requests bypass it.
- Clarification of D-013 (coordinator): D-013's sentence about `audit`
  arriving later concerns page audit rows. ADR 0003 requires `admin create`
  and `reset-password` to be audited, so the `audit` table lands with the
  login packet T-007; page create, update and delete add their rows later.

### D-016 — Page write path: blank page, per-subpage metadata, write grant for create and delete; ADR 0004 accepted

Date: 2026-10-12
Decided by: owner
Answers: write-path design record escalations 1-3 and the ADR proposal (packet-architect), asked in chat 2026-10-12
ADR: docs/adr/0004-page-write-path-and-concurrency.md

- A new page is `DE,` empty, `PS,0000` (not transmitted), `CT,8,T`, six
  `8ff` Fastext links, rows 1-24 as 40 spaces, and no `OL,0`. Creating a page
  never changes what is on air. Rules out transmit-on-create and row-less
  pages.
- Cycle time and the six Fastext links are edited per subpage, as the file
  stores them.
- Once grants exist (M4), anyone with a write grant covering a page number may
  create and delete that page; there is no separate admin rule.
- ADR 0004 (page versions, etag concurrency with `If-Match`, etag-confirmed
  delete, the page audit actions) is accepted.
- Coordinator sequencing, not an owner decision: TTI download and upload
  (T-010) is built before the row editor (T-009), because it is the smallest
  exerciser of the shared write path.

### D-017 — Preview renderer accepted on review; preview shows 4:3 with a 16:9 option

Date: 2026-10-12
Decided by: owner
Answers: the T-006 gallery review
ADR: none

The owner reviewed every fixture page in `/dev/gallery` and found the pages
and graphics render as expected. The display is 1:1, where a television is
4:3; the preview must show a 4:3 frame by default, with a widescreen 16:9
option. This is a follow-up packet to T-006, not a defect in the attribute
engine.

### D-018 — M2 moves after M4; a pages folder that updates within a minute is enough for vbit2

Date: 2026-10-12
Decided by: owner
Answers: owner request in chat, 2026-10-12
ADR: affects the open verification items V-1 to V-4 of docs/adr/0002-page-storage-and-output-pipeline.md; no change to its decisions

The owner will not have a Pi 4 until the end of the project, so M2 (output and
Pi deployment) is delivered after M4 and before M5 cut-over. The milestone
keeps its name so existing references hold. The owner is satisfied that the
app will work with vbit2 provided that (a) the transmit pages are written to a
configured folder, which on the Pi becomes a ramdrive, and (b) a page's file
content changes when the page is edited in the app, immediately or within a
short delay of up to one minute. The plan's "vbit2 reload evidence" item is
removed, and the output writer's criteria now measure (b) directly. ADR 0002's
V-1 to V-4 (how vbit2 notices changes, temp-file handling, Windows rename) are
no longer required before cut-over; if vbit2 misbehaves on the Pi, M5 raises
it. Rules out running vbit2 on Windows as an M2 gate. The M3 scheduler
criterion about the output directory is checked when the M2 writer lands.

### D-019 — Standing authorisation through M3, M4 and the M2 output writer; old generator access

Date: 2026-10-12
Decided by: owner
Answers: owner instruction in chat, 2026-10-12, and a coordinator question the same day
ADR: none

- The coordinator may dispatch every packet of M1, M3, M4 and the M2 output
  writer without asking, `critical` ones included, each with the full
  process its weight requires. The M2 deployment kit and M5 are not covered.
  The coordinator stops a packet, and asks, when a `critical` packet raises a
  question or a question blocks the packet. Supersedes the M1-only limit of
  D-012.
- M3 packets may read, without writing, the old generator's code at
  the old high-score generator (`main.go`, the template `.tti`,
  `build.ps1`). No agent opens its `config.json` (it holds Google Sheet URLs)
  or its service-account key file. Before the "Port the high-score games"
  packet, the owner copies a sanitised `config.json`, with every sheet URL or
  id removed, into `testdata/`. Until it lands, that packet waits.

### D-020 — M3 design: ADR 0005 accepted; host local time; 100 runs per binding

Date: 2026-10-12
Decided by: owner
Answers: M3 design record escalations 1 and 2 and the ADR proposal (packet-architect), asked in chat 2026-10-12
ADR: docs/adr/0005-templates-bindings-and-scheduled-rendering.md (to be committed from the record)

- ADR 0005 is accepted: `SaveGenerated` becomes the fourth page writer, the
  only one allowed to write a page whose `binding_id` is set, audited as
  `page.generate` by actor `job:binding:<id>` (amends ADR 0004's frozen list
  of writers and actions); `internal/httpapi` may import `internal/jobs`
  (amends ADR 0001's import direction); `googleKeyFile` is the config key
  for the service-account key path; no Sheets library error text or sheet id
  is ever logged, stored or returned outside the admin-only binding routes.
- The "last updated" text on generated pages uses the host's local time, as
  the old generator does; no timezone key.
- The run log keeps the newest 100 runs per binding.
- Coordinator weight decision, not an owner one: the M3 "Template pages and
  slots" packet is `critical`, not the plan's `standard`, because it
  introduces `adminRoute`, an authorisation helper (`AGENTS.md`, Process
  weight).
- Open: how fixed binding values containing control codes are entered
  (Q-013); it affects only the scheduler packet.

### D-021 — Sheets error text: network errors keep text only for network failures

Date: 2026-10-12
Decided by: coordinator
Answers: T-014 verification and conformance review
ADR: clarifies docs/adr/0005-templates-bindings-and-scheduled-rendering.md (Sheets source), no change to its decisions

ADR 0005 says a `*url.Error` from the Sheets library becomes `ErrUnreachable`
"with the inner error only". T-014 found that a token-exchange failure
arrives wrapped in a `*url.Error` whose inner text carries the token
endpoint's response body. The rule as built, and binding from now on: the
inner text is kept only when it is a `*net.OpError` or a context
cancellation or deadline, with the sheet id scrubbed; any other inner error
is reported by its Go type name only. `NewSheets` likewise returns a fixed
text with the error's type name, never the library's text. Both are stricter
than ADR 0005's wording and within its intent.

### D-022 — Correction: the 32-cell slot example in ADR 0002 has 25 dots, not 21

Date: 2026-10-12
Decided by: coordinator
Answers: T-013 handover and conformance review
ADR: corrects an example in docs/adr/0002-page-storage-and-output-pipeline.md (Slot syntax); the rule is unchanged

ADR 0002 says `{gname` + 21 dots + `}` is 32 cells. By its own rule (a
slot's width is its cell count, braces included) that token is
1 + 5 + 21 + 1 = 28 cells; a 32-cell `gname` slot has 25 dots. The rule
governs; the code and tests follow it. ADR 0002 is not rewritten; this entry
is the correction a reader of its example should apply.

### D-023 — M4 design: ADR 0006 accepted; self-service password change; constant limits; proxy IP trust in M4

Date: 2026-10-12
Decided by: owner (four answers), coordinator (the two design details marked)
Answers: M4 design record escalations (packet-architect), asked in chat 2026-10-12
ADR: docs/adr/0006-grants-api-keys-and-the-rest-contract.md (to be committed from the record)

- ADR 0006 is accepted (grants and keys in one migration; no rehash on
  login; keys revoked, never deleted; any Authorization header means the
  cookie is ignored; CrossOriginProtection stays on for bearer requests, no
  CORS, which settles D-015's open point; the route registry and the OpenAPI
  contract test).
- Initial and reset passwords stay random and are shown once to the admin.
  Users may also change their own password in the UI. Coordinator detail:
  `PUT /api/v1/me/password` with the current password; the new one is 12-72
  bytes; it ends every session of that user, including the current one,
  exactly as ADR 0003 says, so the user logs in again; it is limited by the
  login per-username limiter and refused for key principals; audit
  `user.change_password`. Built in T-016.
- API rate limits are constants: per client IP 20 requests/second, burst 40;
  per key 5 requests/second, burst 20. Changing them is a decision entry and
  a code change.
- Proxy client-IP trust is built in M4. Coordinator detail: config key
  `trustedProxies` (a list of IPs or CIDRs, default empty). When the
  connection's address is in the list, the client IP is the rightmost
  `X-Forwarded-For` entry that is not itself in the list; otherwise
  `RemoteAddr`, and `X-Forwarded-For` is ignored. Every use of the client IP
  (login and API limits, audit `remote`, logs) goes through that one
  function. Its own `critical` packet, T-020, after T-017.
- Open questions Q-013 and Q-014 (the scheduler) remain open.

### D-024 — Fixed binding values are entered one input per slot, with the editor's palette

Date: 2026-10-12
Decided by: owner
Answers: Q-013 (after the coordinator's pros-and-cons analysis in chat)
ADR: none (storage is unchanged: `bindings.static` stays the JSON object of ADR 0005)

The binding form reads the chosen template's slot list and shows one
labelled input per fixed (static) slot, with its width and a cell counter.
Control codes are typed with the row editor's control-picture characters and
palette (`web/src/editor/cells.ts`), not as `\u00NN` escapes. Slots filled
from the sheet's columns get no input. Changing a binding's template rebuilds
the inputs; values for slots the new template lacks are dropped with a
warning. Rules out the JSON textarea for static values; the `source`
settings remain plain form fields or JSON as the scheduler packet decides.

### D-025 — Without a Google key the scheduler skips scheduled runs

Date: 2026-10-12
Decided by: owner
Answers: Q-014
ADR: none (ADR 0005's runner keeps its shape)

When `googleKeyFile` is not configured (the source is `Unconfigured`), the
scheduler makes no hourly or start-up runs, and the bindings page shows
"Google Sheets not configured". "Run now" still runs and records the failure
with the `googleKeyFile is not configured` error, so an incomplete setup is
visible. Rules out filling each binding's run log with identical hourly
failures.

### D-026 — High-score config: public sheet URLs may be committed; the key stays out of git

Date: 2026-10-12
Decided by: owner
Answers: owner instruction in chat, 2026-10-12
ADR: none

- `testdata/highscores/config.json` is the old generator's config, supplied
  by the owner. It carries the real Google Sheet URLs, and they may be
  committed because the sheets are public. This relaxes the "never copy
  sheet URLs or ids" part of D-019 for these six sheets only. The old
  generator's own `config.json` is still not opened by agents. (Superseded
  by D-074: T-080 deleted this file and the sheet URLs from the tree.)
- The Google key lives in `testdata/secrets/`. That folder is ignored
  (`/testdata/secrets/` in `.gitignore`) and is never committed, printed or
  read by an agent. A test or live check that needs the key takes its path
  from configuration.

### D-027 — Each high-score game has a fixed page number

Date: 2026-10-12
Decided by: owner (the starting numbers beyond 401 are the coordinator's reading)
Answers: owner instructions in chat, 2026-10-12 (two messages; the second
widened "fixed page" to any magazine)
ADR: none (a binding already names its page)

Each game is bound to its own page, which the owner sets for that game
alone. The page can be in any magazine (100–8FF), not only 4xx. Nothing
derives a number from the old `outputPrefix` plus the game's position in the
list, so reordering, adding or moving one game never moves another. The page
is the binding's own page (`bindings.mpp`, set in the bindings UI). The port
adds no prefix, magazine or numbering setting. A binding's page is fixed once
created (`UpdateBinding` refuses a different `mpp`, from ADR 0005). Moving a
game to another page today means deleting its binding and creating a new one
at the new page (superseded by D-028: the page can now be edited in place).
Editing the page in place would be a separate change.

Starting numbers for the port, which keep what is on air today (the old
generator wrote `P4` plus the 1-based list index):

| Page | Game |
|---|---|
| 401–406 | The six games, one per page, in the old config's list order |

(The game names were removed from this record in T-080, D-074.)

The port packet creates one binding per game at these pages. A test must show
that a game bound outside magazine 4 (for example 150) renders at that page.
`outputPrefix` and `outputDir` in the old config are not used.

### D-028 — A binding's page can be edited in place; the page moves with it

Date: 2026-10-12
Decided by: owner
Answers: owner instruction in chat and a coordinator question, 2026-10-12
ADR: amends ADR 0005 ("a binding's page cannot change")

An admin can change a binding's page number in the bindings UI. The bound
page moves to the new number together with its content, its version history
and its transmit flag; the old number no longer exists. Its file leaves the
pages directory at the next reconcile, and the new number's file appears. If a
page already exists at the new number, bound or not, the change is refused
and nothing changes, so no page is ever overwritten. The move is audited.
Supersedes the "delete and recreate" note in D-027.

### D-029 — Standing authorisation extended to the M3 port and the M2 deployment kit

Date: 2026-10-12
Decided by: owner
Answers: coordinator question, 2026-10-12
ADR: none

The coordinator may dispatch, with each packet's full process:
- the binding page move (D-028);
- the M3 "Port the high-score games" packet;
- the M2 deployment kit.

The deployment kit's **(Pi)** criteria wait for the owner's hardware and stay
unticked until then. M5 is not covered. The D-019 stop rules apply.

### D-030 — Page move details

Date: 2026-10-12
Decided by: owner
Answers: Q-015 (a), Q-016 (a), Q-017 (a)
ADR: ADR 0005 amendment written by T-022

- Q-015: a page move writes no `page_versions` row. The `binding.move` audit
  row is the record.
- Q-016: orphaned `page_versions` rows already at the target number are left
  as they are.
- Q-017: the moved page's `updated_at` and `updated_by` become the move's
  time and the admin who made it.

Q-015 to Q-017 asked these three questions, with options and recommendations.

### D-031 — Reading of the M3 sheet-id criterion

Date: 2026-10-12
Decided by: owner
Answers: coordinator question, 2026-10-12, from the T-023 design
ADR: none

"Sheet identifiers are entered in the UI, not committed" means:
- no code, test, fixture, log or document contains a real sheet id;
- the owner types each id into the bindings form;
- `testdata/highscores/config.json` (D-026) is the owner's reference only,
  and no code reads it. (T-080 deleted that file, D-074.)

The plan line is reworded to say this. Rules out stripping the committed URLs
and dropping the criterion.

### D-032 — High-score port: name bar length and port conventions

Date: 2026-10-12
Decided by: owner (bar length); coordinator (conventions, from the architect's T-023 record)
Answers: coordinator question, 2026-10-12
ADR: none (ADR 0005 already provides for the `date` and `number` formatters)

- **Bar length.** The game-name bar on row 7 runs to cell 36 on every game,
  because the name slot is padded to 32 cells and the row is held to 40.
  The old page ended the bar two cells after the name and overran 40 cells.
  To end it earlier for one game, the owner types the name, two spaces and
  "Black background" into that game's name value. No code supports this
  beyond what exists.
- **Slot names:** `p#name`, `p#scr` and `p#date`, plus a three-cell `c`
  colour slot and `gname`.
- **Score units:** a `seconds` formatter, not a unit argument in the
  `format` grammar.
- **Golden test:** the M3 criterion "equals the old generator's output" is
  read as ADR 0005 states it: a reviewed expected output derived from the
  old generator by hand, differing by design where ADR 0002 says so. The
  plan line is reworded to match.

### D-033 — zxnet editor URLs: export, import and create; a new milestone before M5

Date: 2026-10-12
Decided by: owner
Answers: Q-018 (raised 2026-10-12, option c)
ADR: to be drafted by the architect if the design needs one

- **What the feature does.** CeeMS exports one subpage of a page as a URL
  for the zxnet teletext editor. It also imports such a URL into one subpage
  of an existing page, and can create a new page from one.
- **The URL format.** The format and its limits are in
  Q-018: one frame, no subpages, CT or
  DE, no version field, and a blank page for a fragment of the wrong length.
- **What an import changes.** An import replaces rows 1 to 24 of the chosen
  subpage. It keeps CT, DE, the transmit flag and the other subpages. Row 0
  stays out (D-014). The editor's `PS` and `SC` are not taken as ceems's.
  Generated pages refuse the import, as uploads do.
- **Weight.** Import and create overwrite or add user data, so they are
  `critical`. Export is `standard`.
- **Plan.** The owner allows the architect to add a new milestone before M5
  to hold this and other features still to come before completion. The
  architect drafts it and the coordinator writes it into `docs/plan.md`.
  This decision does not authorise dispatching its packets. The stop rules
  of D-019 apply, and a `critical` packet waits for the owner.

### D-034 — zxnet URL feature: design and the owner's answers

Date: 2026-10-12
Decided by: owner (Q-019 to Q-021, dispatch); coordinator (design, from the architect's M6 record and the X270 research)
Answers: Q-019 (a), Q-020 (a), Q-021 (a)
ADR: none. No route, schema, store writer, audit action or page-document
field changes.

- **Where it runs.** The feature is browser-only, in `web/src/zxnet.ts`.
  - Export builds the URL from the editor state.
  - Import fills the editor, and the user's Save stores it through the
    existing `PUT /api/v1/pages/{mpp}`.
  - Create is the existing `POST`, followed by that `PUT`.
  - No REST route for URLs (Q-020).
- **Export mapping.**
  - The meta digit is `0`.
  - Row 0 is 40 spaces. Rows 1 to 24 come as the preview shows them, and the
    data is always 1167 characters.
  - `PN=<MPP>` is sent in upper case. `PS`, `SC` and `RE` are never sent.
  - Links go in `X270=`, mapped one-to-one in ceems `FL` order: red,
    green, yellow, cyan, the fifth link, then the index. Each link is three
    hex digits of page then `3F7F` (any subcode). `8FF` means no link. The
    final digit is `F` (display row 24), so the field is 43 characters. The
    source is the researcher's read of `editor.js:1329-1382` and
    `3384-3418`.
- **Import mapping.**
  - Only 1120 or 1167 characters of base64url after `<meta>:` are accepted;
    anything else is refused with a message.
  - Rows 1 to 24 replace the current subpage's rows in the editor. Row 24 is
    spaces for a 1120-character fragment.
  - Row 0, `PS`, `SC`, `RE`, `X270` and every other field are ignored.
  - The host is not checked. A full URL or a bare fragment is accepted.
  - Create takes the number typed, otherwise a valid `PN`.
- **Stored on Save** (Q-021). The import is written only when the user
  presses Save. Reload discards it.
- **An external test vector** (Q-019). The owner supplies one real editor
  URL as a third golden vector. Packets do not wait for it; it is added when
  it arrives.
- **Dispatch.** The owner allows both M6 zxnet packets: export
  (`standard`) first, then import and create (`critical`) after export
  integrates.

### D-035 — UI layout with Pico.css v2

Superseded by D-053.

Date: 2026-10-12
Decided by: owner (framework, dispatch); coordinator (details, from the architect's Q-022 record)
Answers: Q-022 (b, with Pico.css as the framework)
ADR: none. Pico ships CSS only, with no components and no JS, so ADR 0001's
"no component library" still holds.

- **The stylesheet.** `@picocss/pico` 2.x, version pinned, is the single
  global stylesheet. It is one devDependency and one import in `main.ts`.
  There is no Vite plugin, no PostCSS, and no other package.
- **Rejected alternatives.**
  - Tailwind would need utility classes on every element in ten views, plus
    a Vite plugin that also loads in tests.
  - Bootstrap would need component classes on every control, and its `.row`
    and `.grid` clash with names the page view already uses.
  - water.css and Simple.css lack the grouping and grid helpers the editor
    layout needs.
- **Settled details.**
  - Use the default Pico build with `class="container"`, not the classless
    build. The app mounts inside `#app`.
  - Keep the existing `.bad` row outline for now.
  - Pin Pico v2, because v1's full-width buttons would break the palette.
- **The teletext preview.** It sets every property it depends on. A
  rendering difference is fixed with an explicit property on
  `.tt-page`/`.tt-cell`, never with a global reset.

### D-036 — Compact Pico sizing

Superseded by D-053.

Date: 2026-10-12
Decided by: owner (the problem: "all the page elements look pretty large" at
1920×1080); coordinator (the fix)
Refines: D-035

- **The cause.** Pico 2 raises the root font size at each width step:
  106.25% at 576px, 118.75% at 1024px and 131.25% at 1536px and up. Every
  rem-sized element grows with it. Two layout faults add to this:
  - Pico's `nav` uses `space-between`, which spreads the magazine buttons
    across the page.
  - Each label sits inside its `role="group"` fieldset, so the group's
    button grows to the height of the label and the input together.
- **The fix.**
  - One small global stylesheet, `web/src/app.css`, imported after Pico in
    `main.ts`. It pins `--pico-font-size` to 100% at every width and
    tightens Pico's form spacing variables.
  - The magazine buttons sit together at the left.
  - In each grouped field, the label text sits outside the input-and-button
    row.
- **The teletext preview is unaffected.** It is sized in container units
  (`cqh`), not rem.

### D-037 — Palette samples, flash, conceal and fastext inputs

Date: 2026-10-12
Decided by: owner (the request; Q-023 a, Q-024 a, Q-025 a; dispatch);
coordinator (details, from the architect's T-029 record)
Answers: Q-023, Q-024, Q-025
Refines: D-017 (T-006 preview behaviour), D-035

- **The palette samples.** Each palette button shows a small live sample of
  its code.
  - The sample is drawn by `TeletextPage` itself, which gains optional
    `rowCount` (default 25) and `colCount` (default 40) props.
  - Each `PALETTE` entry carries its own sample cells.
  - `render.ts` and the snapshots stay unchanged.
  - A drawing for each button by hand was rejected; it could drift from the
    real preview.
- **Flash (Q-023 a).** It animates in the page preview and in the samples
  alike: a 1 s cycle, on for 0.75 s and off for 0.25 s. Under
  `prefers-reduced-motion` it stays steady.
- **Conceal (Q-024 a).** Concealed text is shown dimmed to 35% opacity in
  both places.
- **Fastext links (Q-025 a).** The six link inputs move under Row 24.
  - Their labels are Red, Green, Yellow, Cyan, Link 5 and Index, in FL
    order.
  - The first four get a coloured left border in the preview's own colours.
  - The order comes from the zxnet X270 evidence (Q-018). That Link 5 is
    unused and the sixth link is Index is standard knowledge of ETS 300 706
    packet 27/0.

### D-038 — Palette sections and a control-code overlay

Date: 2026-10-13
Decided by: owner (the request: the buttons are not in line; sections for
alpha, mosaic and other; an option to see control codes on the preview);
coordinator (details)
Refines: D-037

- **Sections.** Each `PALETTE` entry gets a `group`, shown as three
  labelled sections in this order:
  - **Alpha:** the eight alpha colours (0x00–0x07).
  - **Mosaic:** the eight mosaic colours (0x10–0x17), plus Contiguous,
    Separated, Hold and Release. All four affect mosaics only.
  - **Other:** Flash, Steady, Normal height, Double height, Conceal, Black
    background and New background.
- **Alignment.** Each section is a CSS grid of equal columns, with
  full-width buttons whose content sits at the left.
  - Every sample sits in a box of fixed size: 6 cells wide (the longest
    sample) and 2 rows tall.
  - A one-row sample is centred vertically in its box.
  - So every button is the same height, and every name starts at the same
    x.
- **The control-code overlay.** The preview gains a "Show control codes"
  checkbox, off by default and not saved.
  - When it is on, every cell whose raw value is below 0x20 is marked. The
    mark is a dotted outline plus the cell's Control Picture
    (U+2400 + value), the same character the row inputs show.
  - The component reads the raw row text it already receives. `render.ts`
    does not change.
  - The bottom half of a double-height row is not marked, because those
    cells show the row above.

### D-039 — WYSIWYG page editor

Date: 2026-10-14
Decided by: owner (the overhaul request; Q-026 a; Q-027 a; Q-028 b;
Q-029 letters plus digits); coordinator (details, from the architect's
design record of 2026-10-14)
Answers: Q-026, Q-027, Q-028, Q-029
Refines: D-009, D-014, D-037, D-038
Evidence: Q-026

- **No copied code.** No code from zxnet, wxTED or edit.tf. Behaviour and
  key schemes are reimplemented in our own code. Developers do not open
  those projects' sources.
- **The architecture.**
  - A pure reducer, `web/src/editor/edit.ts`, takes the rows, the cursor,
    the mode and a key, and returns the new state. Most tests go here.
  - A thin `web/src/editor/PageEditor.vue` mounts `TeletextPage` and owns
    focus, keys, clicks, the cursor overlay and a status line.
  - `TeletextPage` gains only a `grid` prop. `render.ts`, the snapshots,
    `api.ts`, `zxnet.ts` and `BindingsView.vue` stay frozen.
  - Rejected: making `TeletextPage` the editor, and keeping the inputs.
- **The row inputs go (Q-026 a).** The 24 inputs, the character counter
  and the bad-row state are removed. `PageView` edits rows as cell-value
  strings.
  - Row 0 is never editable, and is sent back as loaded.
  - An unchanged row is sent back as loaded; a changed row is padded to 40.
  - `toEditable` and `fromEditable` stay, for `BindingsView`.
- **Editing.**
  - The cursor covers rows 1-24 and columns 0-39. The lower half of a
    double-height row is skipped.
  - Overwrite is the default. Insert toggles insert mode.
  - The other keys: Backspace, Delete, Enter, Home and End, Ctrl+Home and
    Ctrl+End, and the arrows. Tab is left alone.
  - Typing goes through `cellOf`, the inverse of `glyph`. Rows never wrap.
  - In a mosaic cell, q/w/a/s/z/x toggle the six sixels, and r, f and c
    invert, fill and clear.
  - Clicking a cell moves the cursor to it.
- **Control-code keys (Q-029).** Esc starts a one-shot code layer, and the
  next key is looked up in one table on `PALETTE` (`key`).
  - **Letters, matched on `event.key`:**
    - k r g y b m c w: alpha black, red, green, yellow, blue, magenta, cyan
      and white;
    - the same capitals: the mosaic colours;
    - f/F: flash/steady;
    - d/D: double/normal height;
    - n/N: new/black background;
    - h/H: hold/release;
    - s/S: separated/contiguous;
    - o: conceal.
  - **Digits, matched on `event.code`** (physical keys, so they are the
    same on GB and US layouts):
    - Esc then `Digit1` to `Digit7`: alpha red to white;
    - with Shift: mosaic red to white.
  - Esc again, or an unmapped key, cancels. Palette button titles show the
    key. A `<details>` "Keys" panel lists every key.
- **The grid overlay.** `TeletextPage`'s `grid` prop adds an inset
  box-shadow to each cell. Its checkbox sits beside "Show control codes",
  and both stay usable on a generated page. (Superseded by D-044: they are
  disabled and off there.)
- **Accessibility minimum.**
  - The editor is focusable, with `role="application"` and
    `aria-label="Page editor"`.
  - An `aria-live` status line reads the row, column, mode and cell.
  - On a generated page the editor is shown but cannot be focused.
- **Undo and paste (T-033).**
  - Ctrl+Z, Ctrl+Y and Ctrl+Shift+Z work through an in-memory undo stack
    per subpage, capped at 200.
  - Pasting a zxnet URL works like Import. Pasting plain text writes it
    from the cursor, line by line.
- **Later packets.** Mouse painting of sixels (Q-027 a) and rectangular
  block select with copy and cut (Q-028 b).
- **Slicing.** T-031 is the editor core (`packet-developer-big`). T-032
  adds the control-code keys and the help panel. T-033 adds undo, redo and
  paste. The mouse-painting and block-copy packets follow.

D-039 addendum (2026-10-14, from the T-031 conformance review):

- **The sixel keys apply where mosaic mode is in force at the cursor.**
  That is decided by rendering the page with the cursor cell read as a
  space. A blast-through capital, or a control code at the cursor, starts
  from 0x20.
- **The modifier policy is T-032's to settle:** AltGr arrives as Ctrl plus
  Alt, and plain Alt+letter currently types the letter.
- **T-033 takes over Shift+Insert (paste).** Ctrl+Insert is not handled.

### D-040 — Full-height page editor with a side panel, and the grid class clash

Date: 2026-10-15
Authority: owner (the layout request; Q-030 a and Q-031 a); coordinator
(the details).

- **The grid bug.**
  - `TeletextPage` put the class `grid` on the page. That collides with
    Pico.css's `.grid` utility at 768px and above, which laid the 25 rows
    out as columns.
  - The class is renamed `show-grid`.
- **The layout (owner).**
  - On a wide enough screen, the page editor fills the window height at
    4:3.
  - Everything else sits in a side panel on the right. It takes whatever
    width the editor leaves, and scrolls on its own.
  - The menu bar stays pinned at the top, on every screen.
- **The fastext inputs (Q-030 a).**
  - Options: a, a thin strip under the editor; b, in the side panel.
  - Recommended and chosen: a.
  - They stay in a thin strip directly under the editor, with the status
    line. The editor is sized to leave room for both.
- **The width (Q-031 a).**
  - Options: a, full width on the page editor only; b, Pico's 1450px
    container everywhere, which leaves the side panel about 120px at
    1920×1080.
  - Recommended and chosen: a.
  - The page editor screen uses the full window width. Every other screen
    keeps Pico's centred container.
- **The Keys panel** moves from under the editor into the side panel.
- **Narrow screens.** Below the breakpoint, the editor and the side panel
  stack, as they do now.

### D-041 — Agents may log in to a disposable local server

Date: 2026-10-15
Authority: owner ("make a login for agents to use & give them
authorisation to log in for any of their packets"); coordinator (the
safeguards).

- **The tool.**
  - `scripts/agent-server.ps1` starts a throwaway CeeMS server for one
    agent run. It listens on `127.0.0.1` only.
  - Its config, database and pages live in a fresh directory under the
    system temp directory. There is no Google key, so no Sheets job runs.
  - It creates one disposable admin with `ceems admin create` and writes
    the generated password to a file inside that directory. It never
    prints the password.
  - Stopping the server by its recorded PID deletes the directory, and the
    account with it.
- **The authorisation.** Any role may, in any packet, log in from the
  built-in browser, but only:
  - to a server that this tool started;
  - with the account it created.
- **It never extends to:**
  - the Pi;
  - the owner's own `ceems.json`, `data/` or `pages/`;
  - any non-local host;
  - anything under `testdata/secrets/`.
- **The password** may appear only where it is typed into the login form.
  It never appears in a commit, doc, packet, handover or chat.
- **Cleanup.** Every run that starts the server stops it and records that in
  the handover's cleanup section.
- **What this supersedes.** It replaces the binding lesson "no browser
  login" for this case only. Checks against the owner's real data or
  devices remain owner checks.

### D-042 — Credential-free agent login, build-tagged and loopback-only

Date: 2026-10-15
Authority: owner ("dispatch b, and yes to CI running the tagged tests");
architect (the design). ADR:
[0007](adr/0007-credential-free-agent-login.md).

Q-032 asked how agents may get past the login after the classifier refused
the coordinator reading the disposable `login.txt`. The options were:

- a) an owner permission rule for the file;
- b) a credential-free login route, behind a build tag;
- c) no change.

The coordinator and the architect both recommended b. The owner chose b.

- **The route.** `GET /api/v1/agent-login` exists only in a binary built
  with `-tags agentdev` (the `internal/httpapi/agentlogin.go` and
  `agentlogin_off.go` twins).
  - It refuses a non-loopback raw `RemoteAddr`, and ignores
    `X-Forwarded-For`.
  - It issues a session only for an existing, enabled `agent` user.
  - It redirects 303 to `/`.
- **The listen guard.** A tagged `serve` refuses to start on a non-loopback
  listen address.
- **The script.** `scripts/agent-server.ps1` builds with
  `-tags "ui agentdev"`, writes no password file, and prints the login URL.
  This supersedes D-041's password handling. D-041's other limits stand:
  the disposable local server only, never the Pi, the owner's data, or
  another host.
- **CI (owner).** `scripts/test.ps1` runs `go test -tags agentdev ./...` as
  well as `go test ./...`.
- **The invariant.** `agentdev` never appears in `scripts/build.ps1` or in
  `deploy/`. This is the barrier that matters; the loopback checks do not
  help behind a same-host reverse proxy.
- **Weight.** `critical`: the conformance review and mutation checks are
  required.

### D-043 — A compact control-code palette: an icon grid with key badges

Date: 2026-10-15
Decided by: owner (the request; Q-033 a; headings kept); coordinator
(details, from the architect's T-037 record)
Answers: Q-033
Refines: D-037, D-038

Q-033 asked how to make the 27 palette buttons compact, show each key, and
show each meaning without text. The options were a dense icon grid (a), a
two-column list (b), today's list with keys added (c), and a keyboard map
(d).

- **The icon grid (Q-033 a).** Each button shows only its live sample (the
  D-037 sample becomes the icon) and a key badge in the bottom-right
  corner. The name is the button's accessible label and its tooltip.
- **The grid.** `repeat(auto-fill, minmax(4rem, 1fr))`: at least four
  buttons per row in the 18rem panel.
- **The sections.** D-038's Alpha, Mosaic and Other sections stay, with
  small headings (the owner's choice). The sample box stays 6 cells by 2
  rows.
- **Look-alike codes.** Flash/Steady and Hold/Release look alike. They are
  told apart by the key badge (f/F, h/H) and the tooltip. No icons are
  drawn by hand (D-037).
- **The key help.** `KeysHelp.vue` is unchanged; the digit forms
  (Esc 1–7) stay there.

### D-044 — Mouse painting of sixels; view options off on a generated page

Date: 2026-10-17
Decided by: owner (Q-034 a, Q-035 a, Q-036 a, and "the other features
mentioned should also be disabled too"); coordinator (details, from the
architect's T-038 record)
Answers: Q-034, Q-035, Q-036
Supersedes: D-039 in part (its grid bullet: "both stay usable on a
generated page")

The questions, all raised 2026-10-17 for T-038:

- Q-034: mouse only (a), or touch and stylus too (b)?
- Q-035: is Draw disabled on a generated page (a), or enabled but inert
  (b)?
- Q-036: does a drag over an alpha cell do nothing (a), or insert a mosaic
  colour code first (b)?

The coordinator recommended a for each. The owner chose a for each.

- **Mouse only (Q-034 a).** Painting uses mouse events with no pointer
  capture. Touch and stylus painting are out.
- **A Draw view option.** With Draw on, a press and drag over mosaic cells
  sets or clears sixels. The first sixel pressed decides which, for the
  whole drag. One drag is one undo step. With Draw off, the mouse only
  moves the cursor.
- **Mosaic cells only (Q-036 a).** A drag paints only where mosaic is in
  force, as the sixel keys do. It never changes a control-code cell, an
  alpha cell or row 0.
- **Generated pages (Q-035 a, extended by the owner).** Draw, Show control
  codes and Grid are all disabled on a generated page, and are off there.
- **The packet.** T-038, `standard`, `packet-developer`.

### D-045 — Block select, copy and cut: an anchor in State and native clipboard events

Date: 2026-10-21
Decided by: owner (Q-028 b in D-039; Q-038 a); coordinator (details, from
the architect's T-039 record)
Answers: Q-038
Refines: D-039

Q-038 asked whether a generated page allows selecting and copying a block:
no (a), or yes with a focusable read-only editor (b). The coordinator
recommended a. The owner chose a.

- **Selecting.** Shift and an arrow key, or a drag with Draw off, select a
  rectangle between an anchor and the cursor. Esc, a click, undo or redo,
  and any edit clear it. Esc with a selection only clears it; the next Esc
  opens the code layer.
- **Clipboard.** Copy and cut use the browser's native events, so the
  block reaches the OS clipboard over plain HTTP. `text/plain` holds the
  block as glyphs with Control Pictures for codes.
  `application/x-ceems-block` holds the same text and marks it as a
  rectangle.
- **Paste.** A block overwrites a rectangle at the cursor, codes intact,
  clipped at column 39 and row 24, ignoring insert mode. A text or zxnet
  URL paste is unchanged, and the text path still drops codes.
- **Cut** leaves spaces, as one undo step.
- **Rows.** Row 0 is never in a block. A hidden lower-half row is copied,
  cut and pasted as stored.
- **Generated pages (Q-038 a).** No select or copy; the editor stays
  unfocusable.
- **The packet.** T-039, `standard`, `packet-developer`.

### D-046 — Follow-ups: session expiry, mapped proxy addresses, the output test

Date: 2026-10-21
Decided by: owner ("continue on with the follow ups"; Q-037 a);
coordinator (details, from the architect's T-040 and T-041 records)
Answers: Q-037

Q-037 asked whether the forced return to login may discard unsaved
page-editor edits, as a reload does today: accept (a), or add a leave guard
later (b). The coordinator recommended a. The owner chose a.

- **T-040 (KI-002), `standard`.** `api.ts` exposes `onUnauthorized`, and
  `router.ts` registers a handler that clears the user and goes to
  `/login` with a `redirect`. Login and `/me` are exempt. Unsaved edits
  are lost (Q-037 a).
- **T-041 (KI-003), `critical`.** IPv4-mapped addresses and mapped-form
  prefixes are normalised in `inPrefixes`, and the reported client is the
  plain IPv4 form. A mapped-form config entry keeps working. The
  `httpapi.New` comment names the agentdev route.
- **T-042 (KI-004, the test half), `light`.** The output test polls with
  `os.Stat` and reads only after a change. The writer retry stays open.
- **Order.** The follow-ups are dispatched after T-039 is integrated.

### D-047 — Milestone M7: an MS-DOS look for the whole site

Date: 2026-10-21
Decided by: owner (the request); coordinator (the placement)
Refines: D-035 (Pico.css as the global stylesheet)

- **The goal (owner).** Pico.css looks bland. Research and adopt a new
  styling approach across every area of the site, so it looks like an old
  MS-DOS application.
- **The inspiration (owner).**
  - <https://lost-builds.byartem.com/> shows the visual style wanted. It is
    a reference, not a site to copy.
  - Norton Commander, Volkov Commander, shareware CDs and late-night BBS
    file areas.
- **Research first (owner).** Studying those sources is one of M7's own
  tasks. Nothing is researched before that task is dispatched.
- **The placement (coordinator).** M7 sits after M6 and before M5, since
  M5 waits for the Pi 4. The owner may move it.
- **The breakdown.** The architect proposes the subtasks; the coordinator
  records them here once the owner agrees.
- Each M7 packet still needs the owner's go-ahead (as in D-033).

### D-048 — M7 colour schemes left to research; a keyboard-driven UI becomes M8

Date: 2026-10-21
Decided by: owner (Q-039 c, Q-041 c); coordinator (the placement)
Answers: Q-039, Q-041
Refines: D-047

The questions, both raised 2026-10-21 from the architect's M7 breakdown:

- Q-039: one DOS colour scheme (a), two chosen by the OS setting (b), or
  let the M7-1 research recommend (c)? The coordinator recommended a. The
  owner chose c.
- Q-041: look only (a), look plus a function-key bar (b), or a full
  keyboard-driven UI with menus and panels (c)? The coordinator
  recommended a. The owner chose c.

- **Colour schemes (Q-039 c).** M7-1 recommends one scheme or two, with
  its reasons. The owner confirms before M7-2, together with Q-040.
- **Keyboard UI (Q-041 c).** A full keyboard-driven UI is a new feature
  set, so it is its own milestone, M8, after M7. M7 stays a restyle.
- **M7-5 goes.** The function-key bar is dropped from M7; M8 covers it.
- **M7-1 also notes the keyboard conventions** (menu bar, function keys,
  panels) of the reference programs, as input to M8's breakdown. It
  designs nothing for M8.
- **M8's breakdown** comes from the architect after M7-1 reports.

### D-049 — M7 style: plain CSS, the VGA font, two schemes by toggle

Date: 2026-10-21
Decided by: owner
Answers: Q-040; confirms Q-039 (D-048)
Refines: D-047, D-048
Evidence: T-043 (M7-1)

- **Approach and font (Q-040 a).**
  - One hand-rolled global stylesheet replaces Pico and `app.css`; no CSS
    package is used.
  - The font is int10h **Web437 IBM VGA 8x16**, self-hosted. Its licence is
    CC BY-SA 4.0, so the app shows a visible credit to "VileR" with a link to
    <https://int10h.org/oldschool-pc-fonts/>, and a licence note sits beside
    the font file.
  - The budget is unchanged: theme CSS plus fonts no larger than 83 KB.
- **Colour schemes (Q-039).** The owner chose neither of the offered
  options.
  - There are **two schemes, not switched by the OS setting**.
  - The default is the blue "Commander" scheme from the T-043 brief.
  - A darker scheme is the alternative, chosen by an easily reached toggle in
    the app.
  - Both schemes share one set of tokens, and each needs its own WCAG AA
    contrast table.
  - Open for M7-2's design record: the dark palette, where the toggle sits,
    and where the choice is kept.
- **Editor overlays.** M7 restyles all three overlays to the theme:
  - the editor's focus outline;
  - the selection overlay;
  - the cursor, which keeps its inverting (`mix-blend-mode: difference`)
    block, so it shows on every teletext colour.

  `TeletextPage.vue` and `render.ts` stay frozen.
- **The preview** is otherwise unchanged in both schemes. The T-043 report's
  section 10 lists the constraints that keep it so, and they bind M7-2 to
  M7-4.

### D-050 — M7-2 design: dark palette, toggle, credit, checkboxes, font download

Date: 2026-10-21
Decided by: owner (credit placement, checkboxes, the download); architect
(the dark palette and toggle mechanics, in the M7-2 design record)
Refines: D-049
Packet: T-044

- **The dark scheme (architect).**
  - The colours are a black desk (`#000000`), grey frames (`#AAAAAA`), a
    dark-grey shadow and dark-grey disabled text (`#555555`).
  - Every other token matches Commander: bars, fields, dialogs and focus are
    identical in both schemes.
  - Contrast: every text pair passes WCAG AA. Two pairs fail, and both are
    accepted:
    - the red invalid-field boundary on black (2.71:1). It is covered by
      the yellow focus outline every invalid field carries (19.69:1);
    - disabled text, which SC 1.4.3 exempts.
  - The full table is in the M7-2 design record (packet T-044,
    Decision 3).
- **Toggle (architect).**
  - A "Dark" button with `aria-pressed`, as the last item in the header
    nav, so it shows on every screen, including login.
  - The choice is kept in `localStorage` (`ceems.scheme`), per browser.
  - An inline script in `index.html` applies it before first paint.
- **Font credit (owner, option c).**
  - The credit to VileR, with links to int10h and to CC BY-SA 4.0, sits in
    the page editor's Keys help panel.
  - A licence note sits beside the font file.
  - There is no footer.
- **Checkboxes (owner, option b).** M7-2 builds the DOS `[ ]` / `[X]`
  glyph style. Radios stay native.
- **Font download (owner).** The developer may download int10h's
  `oldschool_pc_font_pack_v2.2_web.zip` (about 5 MB) into a scratch folder,
  and commit only the unmodified `Web437_IBM_VGA_8x16.woff` and a licence
  note.

### D-051 — M7-3 design: menu bar, panels, dialogs, and the "All pages" links

Date: 2026-10-21
Decided by: owner (dialogs, create forms, the links); architect (the rest, in
the M7-3 design record)
Refines: D-049, D-050
Packet: T-045

- **Menu bar (architect).** The header becomes a black-on-cyan bar 2rem
  high (`--app-header: 2rem`). Focus rings inside the bar are black, because
  yellow on cyan is 2.69:1. The editor grows to fill the space and stays
  4:3; `PageView.vue` is unchanged.
- **Panels (architect).** Each admin screen sits in a `fieldset.panel` with
  its heading in the `legend`, a double frame and a hard shadow. Wide tables
  scroll sideways inside the panel. There are no media queries, so the
  smaller shadow below 1024px from the T-043 brief is dropped.
- **Dialogs (owner, a).** Login and Change password become centred grey
  `fieldset.dialog` boxes, with yellow-on-black fields and black focus
  rings.
- **Create forms (owner, a).** The create forms stay headed sections inside
  their panel, with no nested frames.
- **"All pages" links (owner, b).** The duplicate "All pages" link is
  removed from the five admin views that have it (Templates, Bindings,
  Users, Keys and Password), because the menu bar's Pages item covers it.
  The page editor's link (`PageView.vue:184`) is left to M7-4.

### D-052 — M7-4 design: framed side panel, 22rem side column, fastext faces, two-colour selection ring

Date: 2026-10-22
Decided by: owner (fastext faces, the side-panel frame); architect (the rest,
in the M7-4 design record)
Refines: D-037, D-040, D-043, D-049
Packet: T-046

- **Side panel (owner, a).** The editor's side column holds one
  `fieldset.panel`, with "Page nnn" in its legend. `--side` grows from
  18rem to 22rem, so the palette keeps four buttons per row inside the
  frame.
  - The preview at a 1024px-wide window becomes 640×480, limited by width.
  - At 1600×900 and 1920×1080 it becomes 1072×804 and 1312×984.
- **Fastext fields (owner, a).** The four coloured link fields get red,
  green, yellow and cyan faces with black text, replacing D-037's coloured
  left border, which was 1.40–2.67:1 against the cyan field.
- **Status line (architect).** A one-row, black-on-cyan bar, with a 1rem
  desk gap above it, so the editor's yellow focus ring lands on the desk.
  `--strip` becomes 2rem.
- **Selection overlay (architect).** A two-colour ring, a 1px yellow outline
  outside a 1px black inset, with a 30% yellow fill. On every teletext
  colour at least one line passes 3:1.
  - The cursor's `difference` block is unchanged. Its green–magenta pair is
    2.29:1, accepted under D-049.
- **Palette (architect).** The colour samples keep their em-sized growth.
  The grid gap becomes 0.5rem, so focus rings land on the desk. Badges get
  square corners, and there are no font-size steps.
- The editor's "All pages" link is removed (D-051).

### D-053 — M7 theme recorded as ADR 0008

Date: 2026-10-22
Decided by: coordinator (the record); owner to accept ADR 0008
Supersedes: D-035, D-036
Packet: T-047
ADR: `docs/adr/0008-ms-dos-theme.md` (Proposed)

- **The record.** ADR 0008 gathers D-047 to D-052 and the T-044 to T-046
  design records into one place. It covers the hand-rolled `theme.css`, the
  self-hosted Web437 IBM VGA 8x16 font and its CC BY-SA 4.0 credit, the
  Commander and Dark schemes, the composition primitives, and the
  invariants later packets must respect.
- **What it supersedes.** D-035's Pico.css stylesheet and D-036's Pico
  sizing. Pico is uninstalled and `app.css` is deleted (T-044).
- **A correction to D-052.** D-052's preview sizes at 1600×900 and
  1920×1080 (1072×804 and 1312×984) left out the editor's 16px block
  padding. The CSS formula is the decision, and it gives 1050.7×788 and
  1290.7×968, as measured in T-046. The 640×480 size at a 1024px-wide
  window stands.
- **Bundle.** A build of `main` at `c3dcec2` gives 20,871 bytes of CSS plus
  font, against the 83,319-byte budget.

### D-054 — ADR 0008 accepted; owner check findings

Date: 2026-10-22
Decided by: owner
Refines: D-053
Packet: T-047

- **ADR 0008 accepted.** The owner reviewed the theme: "It looks good & I
  accept the ADR."
- **Findings, filed as one follow-up packet (T-048):**
  - Fields and buttons do not read as different controls. On the Pages
    list, the "Go to page" field and its "Go" button form one cyan strip,
    and you cannot tell which is the button until you hover over it. The
    fix applies across the app.
  - Add a footer bar holding the Dark toggle, with room for a copyright
    line and a GitHub link.
  - Move Logout to the right-hand end of the header, where the Dark toggle
    is now.

### D-055 — M7-7 design: fields as dark wells, a grey footer bar, Logout on the right

Date: 2026-10-22
Decided by: owner (the footer's colour and text, the GitHub link, the
username move, the ADR form, Firefox as the check browser); architect (the
rest, in the T-048 design record)
Refines: D-049, D-051, D-052, D-054
Packet: T-048
ADR: `docs/adr/0008-ms-dos-theme.md`, amended when T-048 is integrated

- **Style direction (owner).** The look is "inspired by Commander and other
  DOS programs", not limited to Commander. Grey windows on the blue desk are
  fine, and sometimes preferred, where they make content stand out.
- **Fields (architect).** Every text input, select and textarea is a black
  well with yellow text and a 1px grey inset edge, the old dialog field
  style. Buttons stay cyan blocks. The separate dialog field tokens are
  removed.
- **Footer (owner, grey with black text).** A sticky bottom bar,
  `--app-footer: 2rem`, on every screen. It holds "Copyright placeholder",
  a "GitHub" link to `https://github.com/Stoolbend/ceems`, and the
  Dark toggle. M8's key bar will use this row.
- **Header (owner, a).** The username link and Logout move together to
  the right-hand end. The Dark toggle leaves the header.
- **Geometry.** The D-040 formula subtracts `--app-footer` as well. The
  preview becomes 1248×936 at 1920×1080 and 1008×756 at 1600×900, and stays
  640×480 at 1024×768.
- **ADR form (owner, a).** A dated amendment section in ADR 0008. It updates
  the token list and the composition primitives. It extends the geometry
  invariant to "follows `--app-header` and `--app-footer`".
- **Browser checks (owner).** The owner uses only Firefox, so the owner's
  check is Firefox only. Chrome is no longer required.

### D-056 — Teletext50 for the teletext preview; credits move to a dialog

Date: 2026-10-22
Decided by: owner (the font, the budget, the credits dialog, the scope);
architect (the rest, in the T-049 design record)
Refines: D-049, D-050, D-053, D-055
Packet: T-049
ADR: `docs/adr/0008-ms-dos-theme.md`, amended when T-049 is integrated

- **Preview font (owner).** Every `TeletextPage` (the editor, the palette
  samples and the gallery) uses Teletext50 by Alan Davies, a
  public-domain tweak of Bedstead by Ben Harris with SAA5050-style glyphs.
  The menus, panels and buttons keep Web437. The owner's request lifts the
  `TeletextPage.vue` freeze for the font only; `render.ts` stays frozen.
- **Geometry (architect).** The cell's font size is the full cell height,
  because the font's em is the SAA5050 6×10 cell. `--sx` is 25/18 in 4:3
  and 50/27 in 16:9. One unmodified `.otf` serves both aspects and double
  height. Its `@font-face` lives in the component, so `theme.css` keeps its
  single `@font-face`.
- **Budget (owner, 1a).** The `.otf` ships unmodified (128,692 bytes). The
  CSS-plus-fonts budget rises from 83,319 to 160,000 bytes.
- **Credits (owner, 2a amended).** Both font credits move from the Keys
  help to a grey "Credits" dialog, opened from the footer after GitHub,
  with a `[■]` close box at the top right. It is a native `<dialog>`, so the
  browser handles focus and Esc. This supersedes D-050's placement of the
  credit, not its obligation: CC BY-SA 4.0 §3(a)(2) is met by a credit one
  click away on every screen.

### D-057 — Grey windows get an inset frame; the close box is `[X]` on the frame line

Date: 2026-10-22
Decided by: owner (1a, 2a, 3a on the T-049 Firefox check); coordinator (the
CSS, in the T-050 packet)
Refines: D-055, D-056
Packet: T-050

- **Inset frame (owner, 1a).** Every grey window (Login, Change password,
  Credits) shows a 0.5em grey band outside its black double frame, so the
  frame stands out against the desk. It is drawn as a spread box-shadow on
  `.dialog`, and the hard shadow is offset from the grey edge.
- **Close glyph (owner, 2a).** The Credits close box reads `[X]`. This
  replaces D-056's `[■]`.
- **Close position (owner, 3a).** The close box sits on the top frame line
  at the right in Firefox too. It is positioned from the modal `<dialog>`,
  not the fieldset, because the browsers disagree on a fieldset's
  positioning origin.

### D-058 — Guides for Templates and Bindings; a "Show template slots" editor toggle

Date: 2026-10-23
Decided by: owner
Packets: T-051; T-052 (to be
written once its design record is in)

- **Guides (1a, 2a).** A Help page at `/help`, linked from the footer when
  signed in, gives a walkthrough that builds one working example, then a
  reference for every field and rule. Templates and Bindings each get a
  collapsed Help panel, in the editor's Keys pattern, linking to it.
- **Slot braces (b).** In the English teletext set, `{` and `}` (0x7B, 0x7D)
  are drawn as `¼` and `¾`, so a slot typed as `{p1name.}` shows in the
  editor as `¼p1name.¾`. The guides explain this. The editor gains a "Show
  template slots" toggle beside "Show control codes", which draws slot
  tokens with braces and highlights them. The default preview stays
  TV-accurate. Its design is an architect record in T-052's packet.

### D-059 — "Show template slots": session-only, off on bound pages, raw ASCII inside a slot

Date: 2026-10-23
Decided by: owner (1a, 2a, 3a); architect (the design, in the T-052 packet)
Refines: D-058
Packet: T-052

- **Not saved (1a).** Like Show control codes, Grid and Draw, the toggle is
  a view option for the session, and is off each time a page opens.
- **Disabled on bound pages (2a).** A generated page's slots are already
  filled.
- **Raw ASCII inside a slot (3a).** With the toggle on, every cell of a
  slot token is drawn as typed, so `^` and `_` show as typed as well as
  `{` and `}`. The English map otherwise draws them as `↑` and `#`.
- **The design (architect).** `web/src/teletext/slots.ts` mirrors Go's
  slot regex. `TeletextPage` gains a `showSlots` boolean prop and a `slot`
  cell class with a yellow and black underline bar. `render.ts` is
  unchanged. This is the owner-raised change to the frozen
  `TeletextPage.vue` that ADR 0008 (D-056) requires.

### D-060 — Templates can be replaced in place, guarded by the bindings that use them

Date: 2026-10-23
Decided by: owner (1a, 2b, 3a, 4a); architect (the rule and design, in the T-053 packet)
Amends: ADR 0005 (the consequence "upload as a new template")
Packet: T-053

- **The rule.** `PUT /api/v1/templates/{id}` is refused with 409 `in_use`
  when a binding using the template would be left with a slot it fills from
  neither its sheet columns nor its fixed values. The message names every
  such binding, its page and its missing slots. Removing a slot, and
  changing a slot's width, alignment or position, are never refused. The
  owner's first idea, "every old slot must still be present", was dropped:
  it misses the only dangerous case, a new slot, and blocks harmless
  removals.
- **No automatic run (1a).** Bound pages change at the next hourly run or
  on Run now. The form lists the pages that use the template.
- **Stale fixed values (2b).** A dropped fixed slot is allowed. After the
  save, the Templates view lists each binding's fixed values the template
  no longer uses. They are ignored at render, and dropped when the binding
  is next saved (D-024).
- **Name (3a).** It can change in the same edit; a duplicate is refused.
- **Weight (4a).** `critical`, with four mutation checks.
- **Accepted race.** A binding saved at the same moment as its template's
  update can slip past the check. The worst outcome is one failed run that
  keeps the previous page. Recorded as a `shortcut:` comment.

### D-061 — The product is renamed CeeMS, with a version placeholder in the footer

Date: 2026-10-24
Decided by: owner (request, 4a, 5b); architect (scope, in the T-056 packet)
Amends: ADR 0002 (the marker name)
Packet: T-056

- **Name.** The product is CeeMS, a portmanteau of Ceefax and CMS. The
  station's on-air name is not the product name. At the time it stayed in
  test pages, fixtures and the default header (all since replaced, D-074).
- **No migration.** Nothing is installed anywhere except the owner's
  development environment, so everything is renamed outright with no
  upgrade path: the database file is `ceems.db`, and the config file, the
  binary, the deploy kit, the cookie and the browser storage keys are
  renamed too.
- **Go module (4a).** The Go module is renamed `ceems`.
- **Marker (5b).** The pages-directory marker becomes `.ceems-owned`. This
  amends ADR 0002's frozen rule that the writer owns only a directory
  carrying the marker under its earlier name. The owner's local `pages/` must be re-marked or
  emptied once.
- **Version.** The footer shows `CeeMS dev`, read from the build-time
  variable `VITE_CEEMS_VERSION`. A later plan item sets that variable from
  the commit in `scripts/build.ps1` and CI.
- **Owner actions after the milestone:**
  - rename the GitHub repository;
  - rename the local folder once no task worktree is open;
  - rename the local config file to `ceems.json`.

### D-062 — System settings: admin only, stored in the database, vbit.conf written by CeeMS

Date: 2026-10-24
Decided by: owner (request, 1a, 2b, 3b); architect (the design, in the T-057
packet)
Amends: ADR 0002 (the writer also writes `vbit.conf`); adds the audit
action `settings.update`
Packets: T-057,
T-058

- **Settings.** They are kept in the database, in a `settings` table.
  - **System name:** shown in the menu bar and the browser title.
  - **vbit.conf writing:** on or off, on by default.
  - **Six vbit2 keys:** `header_template`, `initial_teletext_page`
    (default `100`), `row_adaptive_mode` (default off), `lines_per_field`,
    `magazine_priority` and `status_display`.
  - **Admin only.** API keys are refused, as on every admin route.
- **Output directory (1a).** `pagesDir` stays a config-file key. The
  settings page shows it read-only. An editable path would let a web
  session make the server write anywhere.
- **`lines_per_field` (2b).** The default is `16`, vbit2's documented
  default.
- **vbit.conf.** When the option is on, CeeMS writes `vbit.conf` in the
  pages directory, holding only the six keys:
  - at start-up, after every settings save, and on each reconcile when it
    differs;
  - through the same temp-then-rename path as pages.

  Other vbit2 keys go in `vbit.conf.override`, which CeeMS never reads,
  writes or deletes. When the option is off, CeeMS leaves any existing
  `vbit.conf` exactly as it is. vbit2 reads the file at start, so the page
  says to restart vbit2 after a change.
- **System name and on-air text are not coupled.** The system name is not
  substituted into the header or status message.
- **Editor header (3b).** The page editor's preview header row uses the
  stored `header_template`, delivered to signed-in users in the `me`
  document, instead of the fixed `DEFAULT_HEADER`.

### D-063 — The status message refuses spaces and `;`; the default header keeps its cyan cell

Date: 2026-10-24
Decided by: owner (1c, 2a)
Refines: D-062
Packets: T-059,
T-058

- **Status message (1c).** `statusDisplay` takes 0 to 20 characters, each
  from 0x21 to 0x7E except `;`. Spaces and `;` are refused, following the
  sample `vbit.conf`'s "there must be no whitespace" and its `;` comment
  marker. This narrows T-057's rule, which allowed 0x20 to 0x7E.
- **Default header (2a).** The default `header_template` keeps cell 0x03
  (cyan) before the clock, written to `vbit.conf` as ESC C, as in the
  owner's sample.

### D-064 — The system name is public, and the menu bar reads it from one place

Date: 2026-10-24
Decided by: owner (the request), coordinator (the design)
Refines: D-062
Packets: T-060

- **Owner request.** The menu bar should show the system name on the login
  page as well as after login. The owner reports that it still shows
  `CeeMS` after logging in.
- **Public endpoint.** `GET /api/v1/site` needs no session and returns
  `{"systemName": "..."}`. The system name is the station's on-air name,
  so making it public exposes nothing new. No other setting is public.
- **One source.** The web app loads the name from `/api/v1/site` when it
  starts. A settings save updates it. The `h1` and `document.title` read
  only this value, and fall back to `CeeMS` until it loads or if the call
  fails. The client stops reading `systemName` from `me` and login. The
  server keeps returning it there, so the API does not change shape.

### D-065 — Fastext groups and a central index link

Date: 2026-10-24
Decided by: owner (the request; answers 1, 2b, 3a, 4a, 5a, 6a, 7b, then
1a, 2a, 3a, 4a), architect (the design)
ADR: `docs/adr/0009-fastext-groups.md`
Packets: `T-061` (store), then the API, web and docs packets of M10

- **Groups.** An admin creates named fastext groups on a Fastext page. A
  group is static or paged and holds the four coloured links (red, green,
  yellow, cyan). One group may be the default.
- **Paged groups (owner 1).** The admin picks which coloured key is
  Previous and which is Next, with toggle buttons under the four link
  boxes. A chosen key's box is disabled. The two keys must differ.
- **No wrap (2b).** The first member's Previous key and the last member's
  Next key get `8ff`, no link.
- **Membership.** A page belongs to at most one group. An admin sets a
  group's members and their order on the Fastext page (4a). The page
  editor shows the group and links to the Fastext page.
- **Ownership of the links (3a).** While a page is in a group, every save
  stores the group's four coloured links, whatever was sent. A page taken
  out of a group keeps its last links, which are manual again.
- **Group changes** rewrite every member page in one transaction. Adding,
  deleting or renumbering a page in a paged group re-links its neighbours.
- **Default group.** A page created in the editor, from zxnet, or by a new
  binding joins the default group, last in order. `ceems import` does not
  apply a group (5a).
- **Generated pages (6a).** A binding's page may join a static or paged
  group at any time. The group's keys override the template's for that
  page.
- **Link 5** is never touched by a rule.
- **Index link (7b).** Two system settings: `indexPageEnabled` (default
  off, 1a) and `indexPage` (default `100`). When on, every page's sixth
  link is the index page, including the index page itself (4a). Turning
  it on or changing the page rewrites every page. Turning it off rewrites
  nothing; the links become manual again. `ceems import` applies it, and
  compares a file to a stored page after applying it, so a re-import
  stays unchanged (2a). The editor shows the sixth link read-only while
  it is on (3a).
- **Audit.** Each page a rule changes gets its own `page.fastext` audit
  row and version row, plus the triggering `fastext.*` or
  `settings.update` row (coordinator).

### D-066 — A fastext group writes row 24 with its link text

Date: 2026-10-25
Decided by: owner (the request; answers 1b, 2a, 3b, 4a, 5a), architect
(the design)
Refines: D-065
ADR: `docs/adr/0009-fastext-groups.md` (amendment)
Packets: `T-064` (store and API), `T-065` (web)

- **Owner request.** A group sets each coloured link's text as well as its
  page, and generates row 24 on every member page.
- **Layout.** Row 24 is four 10-cell fields in the order red, green,
  yellow, cyan. Each field is the slot's alphanumeric colour code (red,
  green, yellow, cyan) followed by the link text padded with spaces to 9
  cells. Example: `OL,24,ALink1    BLink2    CLink3    FLink4    `, where
  `A`, `B`, `C` and `F` stand for the colour codes.
- **Link text.** Up to 9 cells per link, control codes included; each
  control code is one cell.
- **Prev and Next text (1b).** Fixed, not set per group: ` [ Prev  ` and
  `  Next ] `, in the colour of the slot that holds them. Example:
  `OL,24,AIndex    B [ Prev  C  Next ] FArt index`.
- **Ends of a paged group (2a).** Where Previous or Next is `8ff`, that
  field is the colour code followed by 9 spaces.
- **Scope.** Row 24 is written on every subpage of every member page, by
  the same rule and sweeps as the links (D-065), for static and paged
  groups alike. The index link does not appear on row 24.
- **Leaving a group (3b).** A page taken out of a group, or left by a
  deleted group, keeps an empty row 24 line (`OL,24,` with no cells) on
  every subpage, so the inserter sends a blank row.
- **Editor (4a).** The page editor shows row 24 read-only on a member page.
- **Entering text (5a).** The Fastext page edits each link text in a
  one-row, 9-cell box that takes control codes with the page editor's own
  keys, and previews the row 24 the group will write.

### D-067 — A blank fastext link box means no link

Date: 2026-10-25
Decided by: owner
Refines: D-065
Packets: `T-066`

- **Owner report.** Saving a new group with the green link left blank
  failed. The quoted error was the pre-T-064 server's (the server had not
  been restarted, and it refused the new client's `texts` field); on a
  current server a blank link is refused as an invalid link.
- **Decision.** On the Fastext page a blank link box saves as `8ff`, and a
  stored `8ff` shows as an empty box when the group is opened. The link
  boxes are no longer required. The API and store do not change: `8ff`
  stays the stored form of "no link". The slot's row 24 text is unaffected.

### D-068 — The page document says whether the caller can write it

Date: 2026-10-25
Decided by: owner (the request), coordinator (the design)
Refines: D-006, D-016
Packets: `T-068`

- **Owner report.** A user with a read-only grant on a page sees the editor
  as if they could change it; only a save fails.
- **Server says.** The page document gains `canWrite`: whether the
  caller's principal may write that page (`auth.Principal.Can(mpp, true)`),
  so the client never reimplements the grant rule. It is ignored on PUT.
  Enforcement does not change.
- **Editor.** When `canWrite` is false, the side panel shows a read-only
  warning, Save and Delete show disabled, and every editing control is
  disabled as on a bound page. A bound page keeps its own message.

### D-069 — Releases: a pushed semver tag, a stamped footer, per-platform archives

Date: 2026-10-25
Decided by: owner (the requirements and the answers below), architect (the
release control method and design)
Refines: D-001 (the build)
Packets: `T-073`
ADR: `0010`

- **Owner request.** GitHub Actions; a release only when wanted, numbered
  in semantic versioning; an archive the user extracts and uses; the footer
  shows the version on a release build and the commit hash otherwise; the
  architect decides the release control method first.
- **Release control.** The owner pushes a tag `vMAJOR.MINOR.PATCH`
  (optional `-pre`, no `+build`). `.github/workflows/release.yml` checks
  the tag against semver, runs the test suite on the tagged commit, builds,
  and publishes a GitHub release with auto-generated notes; a `-pre` tag is
  marked pre-release. A manual run (`workflow_dispatch`) builds any commit
  and uploads the archives to the run, with no release. `ci.yml` is
  unchanged. No develop branch.
- **Version string.** A release shows the tag as typed (`v1.2.0`) in the
  footer, the release name and the archive names. Any other build shows the
  first 7 characters of the commit hash (owner: 3a). `scripts/build.ps1`
  takes `-Version`, defaulting to `git describe --tags --always --dirty`,
  and sets `VITE_CEEMS_VERSION`; this closes M9-4. The Go binary does not
  change.
- **Archives.** `ceems-<version>-windows-amd64.zip` holds `ceems.exe` and
  `ceems.example.json`. `ceems-<version>-linux-arm64.tar.gz` and
  `ceems-<version>-linux-amd64.tar.gz` (owner: 5b adds Linux x64) hold
  `ceems` (mode 0755), `ceems.example.json` and `deploy/`. Only the example
  config ships; the user copies it to `ceems.json`, so extracting an
  upgrade never overwrites their settings (owner: 1a). `ceems admin create`
  stays the first step.
- **Publication.** Immediately, not as a draft (owner: 4a).

### D-070 — One commit per task on `main`, and the history rewritten to match

Date: 2026-10-25
Decided by: owner
Refines: the coordinator protocol (`agent-coordinator.md` in the private process repository)

- **Owner report.** The log was hard to read: 261 of 341 commits were
  `docs:`, three bookkeeping commits (packet, dispatch, integration) around
  each change.
- **Going forward.** The packet is committed on the task branch, the ledger
  stays uncommitted until integration, and the squash commit carries the
  code, the packet with its handover, the ledger entry, the coordinator notes
  and the plan tick. One task is one commit on `main`.
- **History.** The existing history is rewritten so each contiguous run of
  commits for one task becomes one commit, with the tree of the run's last
  commit and the code commit's message. Every rewritten commit's tree equals
  a tree from the old history; the final tree is unchanged. The owner deleted
  the remote `v0.1.0` tag; it is re-created on the rewritten commit and the
  release re-run. Old commit hashes quoted in packets, the ledger and this
  log refer to the pre-rewrite history; a bundle of it is kept by the owner.

### D-071 — CeeMS is AGPL-3.0, with its notices in the credits dialog

Date: 2026-10-26
Decided by: owner
Packets: `T-075`

- **Licence.** AGPL-3.0-or-later (owner: 1c over MIT, Apache-2.0 and
  GPL-3.0). Every linked dependency is BSD, MIT, Apache-2.0 or public
  domain, so all are compatible; GPL-2.0-only was ruled out by the
  Apache-2.0 dependencies.
- **Copyright.** "Copyright © 2026 Conor Morgan and CeeMS contributors"
  (2c). The footer's placeholder copyright line is removed; the line lives
  in the credits dialog, which serves as the AGPL section 5(d) "Appropriate
  Legal Notices" (copyright, no warranty, the licence and how to view it)
  and carries the section 13 source link.
- **Notices.** The dialog summarises; a generated, committed
  `THIRD_PARTY_NOTICES.txt` holds every linked component's full licence and
  notice texts, is served by the binary, ships in every release archive, and
  is checked for staleness by `scripts/test.ps1` (3a).
- **Contributors.** Claude's `Co-Authored-By` trailers are attribution, not
  a copyright claim; re-licensing needs only human contributors' consent.
  A CLA is not adopted now; decide before the first outside contribution if
  re-licensing should stay possible.

### D-072 — A linux/armv6 build for the Raspberry Pi 1 and Pi Zero

Date: 2026-10-26
Decided by: owner (the request), coordinator (the name and scope)
Refines: D-069 (the release targets)
Packets: `T-077`

- **Target.** `GOOS=linux GOARCH=arm GOARM=6`, built with the others by
  `scripts/build.ps1` and shipped in every release as
  `ceems-<version>-linux-armv6.tar.gz`, with the same contents as the other
  Linux archives. No code change is needed: the SQLite driver is pure Go.
- **Support.** The Pi 4 on 64-bit Raspberry Pi OS stays the documented and
  supported board. The armv6 build is offered untested on hardware; whether
  vbit2 and raspi-teletext run on a Pi 1 is for their own documentation.

### D-073 — Binding format values carry a unit or date pattern; the slot width reaches the formatter

Date: 2026-10-26
Decided by: owner (the request and answers Q1-Q4, E1-E4), architect (the design)
Refines: D-032 (supersedes its "a `seconds` formatter, not a unit argument" line)
Packets: `T-079`

- **Grammar.** A Formats value is `text`, `number [unit=<text>]`, `seconds`
  (an alias for `number unit=sec`) or `date [<pattern>]`. Only the server
  validates it. The storage shape and API are unchanged.
- **Numbers.** Non-numeric characters are stripped; the number keeps the
  sheet's text when it fits, otherwise shrinks with k/m/b to the width the
  unit leaves; if it still does not fit, the text is cut from the right. A
  cell that is not a number shows unchanged, without the unit.
- **Dates.** Tokens `D DD M MM Mth YY YYYY`; the default stays
  `DD Mth YY`. A pattern longer than the slot is cut from the right.
- **Width.** The runner passes each slot's width from the template (the
  smallest where a name appears in several slots), replacing the fixed
  nine-cell assumption.

### D-074 — No page, config or name from the owner's service in the repository

Date: 2026-10-26. Owner request; supersedes the fixture provenance in D-009 (Q-002),
D-026 and the D-061 note that the station name stays in test data.

- The fixture pages, the high-score template, goldens and config, and the
  vbit2 sample under `testdata/` came from the owner's own teletext service
  before the project was open source. They are replaced by generic pages
  written for the project, and the old generator config is dropped (T-080).
- The station name and the old product name do not appear in the tree; the
  old product name becomes `ceems` / CeeMS wherever it is referred to.
- History is rewritten to remove the old files and names, and the GitHub
  repository is recreated from the rewritten history by the owner (T-081).
  Commit hashes recorded before the rewrite are mapped to the new ones.

### D-075 — One squashed public history; agent process files live in a private repository

Date: 2026-10-26. Owner decisions; supersedes D-074's history rewrite and
D-070's rule that a packet is committed on its task branch.

- The public repository starts again from one commit made from the verified
  final tree. The history rewrite (T-081) is abandoned. The old history is
  kept privately in the companion repository as `archive/ceems-main`.
- Task packets, the run ledger, evidence, the coordinator protocol, the
  packet template and the project brief live in the private companion
  repository `ceems-dev` (checked out at `C:\Code\ceems-dev`). The public
  tree keeps the code, user docs, ADRs, these decisions, the plan,
  `AGENTS.md` and the agent definitions.
- `T-nnn` ids in code and public docs stay as plain task ids; they are not
  links.
- Fixture files under `testdata/` are stored byte-exact (`-text`), so the
  tests see the same line endings on every platform (T-082).

### D-076 — Number formats take a decimals option; Formats stay one entry per line

Date: 2026-10-26. Owner request; extends D-073.

- `number` takes space-separated options in any order: `unit=<text>` and
  `decimals=<0-6>`. A unit has no space, comma or equals sign.
- With `decimals=d` the number is rounded to exactly d places, keeping the
  sheet's thousands commas. When it does not fit its slot, decimals drop one
  at a time, then k/m/b shrinking applies, then the value is cut from the
  right.
- Without `decimals`, a value under 1000 that does not fit keeps as many
  decimals as fit instead of rounding to a whole number.
- The Formats box takes one `column=formatter` entry per line. A comma does
  not separate entries; the form says so and the Help page shows a
  two-entry example (T-083).

### D-077 — Page preview: a dedicated preview document and a TV-style player

Date: 2026-10-26. Owner request; research Q-027.

- A Preview button on the page list and on the Bindings list opens a dialog
  that plays the saved page as a TV would: the live header row, subpages
  rotating on their own timing, flashing text. Concealed text is shown
  dimmed, as in the editor. Fastext links are drawn but do nothing, and there
  are no playback controls (owner, for now).
- The preview has its own read-only endpoint, `GET /api/v1/preview/{mpp}`,
  not the editor's page document or the admin settings. It returns only what
  a viewer needs: the page number, description, header template and, per
  subpage, the rows and how many seconds to show it. A later anonymous viewer
  serves the same document for transmitting pages (owner). For now it needs
  a signed-in caller who may read the page.
- Subpage seconds come from the CT line. `CT,n,T` is n seconds. `CT,n,C`
  is n times the estimated cycle time of the page's magazine. A subpage with
  no CT line gets 8 seconds, the default CeeMS writes for a new page.
- The server estimates a magazine's cycle time from the transmitting pages
  and the vbit2 settings with the formula from Q-027 (vbit2 at 4948857). It
  is computed on each request; one pass over at most 800 pages is cheap
  enough for a Pi 1. It is exact with one busy magazine and typically within
  about 15 to 50 per cent with several, which is close enough for a preview.
- The player component takes the preview document as input and fetches
  nothing itself, so the later viewer can reuse it.

### D-078 — CeeMS writes every live vbit2 key

Date: 2026-10-28. Owner request; research of vbit2 `configure.cpp` at
4948857 (same as master).

- vbit2 reads 11 keys from `vbit.conf`. CeeMS writes all of them except
  `full_field`, which vbit2 reads and ignores. The four added are
  `network_identification_code` and `country_network_identification`
  (4 hex digits, default `0000`), `datacast_lines` (0 up to
  `lines_per_field`, default 0) and `magazine_serial` (default false), all
  vbit2's own defaults. Existing databases get them by a migration.
- The settings page links to the vbit2 wiki's manual configuration page
  instead of explaining each key. `datacast_lines` and `magazine_serial`
  are not on the wiki, so each gets a one-line hint.
- `lines_per_field` is limited to 1 to 16, because vbit2's raspi-teletext
  launcher refuses more (owner). The old limit of 32 was a guess.
- A new install's `magazine_priority` default becomes the wiki's
  `9,3,3,6,3,3,5,6` (owner). Existing databases keep their value.
- The initial page is checked as vbit2 checks it: the page's low byte is not
  `FF`, and a subcode is at most `3F7F` with no bits of `C080` set. vbit2
  corrupts the magazine priorities when that line is malformed, so CeeMS
  never writes a value vbit2 would reject.
- This supersedes the key list and the `lines_per_field` limit in D-062
  (vbit.conf writer); the override file still loads after `vbit.conf` and
  still wins.
