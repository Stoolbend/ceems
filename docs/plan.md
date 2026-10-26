# Plan

Built from the owner's project brief and decisions D-001 to D-008 in
`docs/decisions.md`. Milestones are in delivery order and each one is a usable
increment. Each item is sized for one packet and names its process weight
(`AGENTS.md`, Process weight). A checkbox is ticked only by the coordinator,
on evidence from integrated `main`, citing the task id. Open product questions
live in `docs/decisions.md`, not here.

Hardware does not exist yet (D-001). Every criterion marked **(Pi)** stays
unticked until a Pi 4 does; everything else must be provable on Windows.
M2 is delivered after M4 (D-018), and M6 holds the features the owner adds
before cut-over (D-033). The milestone order is therefore M0, M1, M3, M4,
M2, M6, M7, M8, M5. M7 (D-047) and M8 (D-048) sit before cut-over because M5 waits for the
hardware.

## M0 — Scaffold, build and the TTI codec

Outcome: one repository that builds one binary for `windows/amd64` and
`linux/arm64`, runs its tests in one command, and reads and writes TTI page
files without loss.

- [x] Repository scaffold `light` (T-001) — Go module, `cmd/ceems`, Vue 3 + Vite +
  TypeScript app under `web/`, built assets embedded in the binary.
  - [x] `go test ./...` and the Vue test command both pass from a clean clone. (T-001)
  - [x] One build script produces both binaries from the owner's Windows PC. (T-001)
  - [x] The Windows binary serves the Vue shell and a health endpoint on the
    configured address. (T-001)
- [x] Configuration and data directory `light` (T-002) — config file (listen address,
  data directory, vbit2 pages directory), SQLite opened at start with
  migrations applied.
  - [x] Starting with an empty data directory creates the database and
    applies every migration; starting again applies none. (T-002)
  - [x] A missing or invalid config fails fast with a message naming the key. (T-002)
- [x] TTI codec `standard` (T-003) — parse and serialise the vbit2 TTI format (PN, SC,
  PS, CT, OL, FL, DE, DS, RE, PF; carousels with several PN lines; rows 1 to
  29; 7-bit ESC and 8-bit control-code forms).
  - [x] Every fixture page (Q-002) round-trips byte-identical. (T-003)
  - [x] A malformed file returns an error naming the line number. (T-003)
  - [x] Page number, magazine, subpage count and description are exposed as
    typed fields. (T-003)
- [x] Continuous integration `light` (T-004) (after Q-003) — tests run on every push.
  - [x] A failing Go or Vue test fails the workflow. (T-004; GitHub runs 38147885329 pass on `main`, 38148378715 fail on a deliberate Go failure)

## M1 — Pages in the browser

Outcome: the owner logs in, sees the current page set imported from the old
system, previews any page faithfully, edits rows, and can download or upload a
page as TTI.

- [x] Page store and one-off import `standard` (T-005) — schema per ADR 0002 and a
  CLI subcommand that imports a directory of `.tti` files.
  - [x] Importing the fixture directory creates one page per file, with
    subpages preserved. (T-005)
  - [x] Importing the same directory again changes nothing. (T-005)
  - [x] Two files claiming the same page number stop the import with both
    paths named. (T-005)
- [x] Admin bootstrap and login `critical` (T-007) — CLI subcommand creates the first
  admin; password login; session cookie; logout.
  - [x] Passwords are stored hashed per ADR 0003; the hash is never returned
    by any route. (T-007)
  - [x] Every route except login and health rejects an anonymous request. (T-007)
  - [x] Mutation check: removing the session check on any page route fails a
    test. (T-007)
  - [x] Repeated failed logins are rate limited. (T-007)
- [x] Magazine and page list `light` (T-008) — magazines 1 to 8 with page counts;
  pages with number, description, subpage count and last update; jump to a
  page number.
  - [x] The list matches the database for the fixture set. (T-008)
- [x] Level 1 preview renderer `standard` (T-006; 4:3 display follow-up T-012) — a Vue component that draws a page
  as a PAL Level 1 decoder would: alphanumeric and mosaic colours, contiguous
  and separated mosaics, held graphics, double height, background colour,
  conceal shown, flash as steady, header row from the vbit2 header template.
  (2026-10-12: D-037 changes this. Flash animates, concealed text is dimmed,
  T-029.)
  - [x] Rendering of each fixture page matches a reviewed snapshot. (T-006; owner review 2026-10-12, D-017)
  - [x] Each control-code case above has its own test page. (T-006)
- [x] Row editor `standard` (T-009) — edit rows 1 to 24 as 40-column text with live
  preview; a palette inserts control codes; metadata form for description,
  cycle time, transmit flag and the six Fastext links; subpages of a carousel
  are each editable, and can be added or removed.
  - [x] A row cannot exceed 40 cells; the UI shows the cell count. (T-009)
  - [x] Saving an unchanged page writes nothing. (T-009)
  - [x] Saving writes the page and the preview re-renders from what was
    stored, not from client state. (T-009)
- [x] TTI download and upload `standard` (T-010) — per page.
  - [x] Download of an imported page is byte-identical to the imported file. (T-010)
  - [x] Upload replaces the page after validation; a page number mismatch is
    refused with the two numbers named. (T-010)
- [x] Create and delete a page `critical` (T-011) — new blank page at a free number;
  delete after confirmation.
  - [x] Creating a page at a taken number or outside magazines 1 to 8 is
    refused. (T-011)
  - [x] Delete removes the page and writes an audit entry naming the user. (T-011)
  - [x] Mutation check: a delete without confirmation token fails a test. (T-011)

## M3 — Templates and scheduled Google Sheet imports

Outcome: the six high-score pages are produced by the app from templates and
Google Sheets, hourly, replacing the old generator.

- [x] Template pages and slots `critical` (T-013; weight raised, D-020) — a page can be a template whose
  rows carry slot tokens per ADR 0002; rendering a template with data
  produces an ordinary transmit page that the editor shows as generated and
  read-only.
  - [x] A slot's value is padded or clamped to the token's width so no cell
    moves. (T-013)
  - [x] A row whose every slot is empty is written as 40 spaces, never
    omitted, so a television replaces old content on that row. (T-013)
  - [x] The rendered page round-trips through the TTI codec. (T-013)
- [x] Google Sheets source `critical` (T-014, T-015) — service-account key from the location
  Q-001 decides; read a configured range with formatted and raw values as the
  old generator does.
  - [x] The key is never logged, returned by a route, or stored in the
    database. (T-014)
  - [x] A sheet that is unreachable, empty, or missing columns reports a
    named error and produces no page. (T-014 named errors; T-015 TestFailedRunKeepsPage proves no page)
- [x] Scheduler and run log `critical` (T-015, T-021) — hourly run per template, manual run
  now, no overlapping runs, each run logged with start, end, outcome and
  error text.
  - [x] A failed run leaves the previous rendered page in place and in the
    output directory. (T-015 proves the page row; T-021 TestFailedRunLeavesFileUntouched proves the file)
  - [x] Mutation check: a run that errors after fetching but before writing
    cannot leave a half-written page. (T-015)
  - [x] The run log is visible in the UI and keeps the last 100 runs. (T-015; 100 per binding, D-020)
- [x] Port the high-score games `standard` — recreate the current template and
  the six game configurations (names, colours, start cells, score units) in
  the app; reproduce number abbreviation, date formatting and the game-name
  colouring from the old generator.
  - [x] A golden test feeds fixed sheet rows through the new renderer and
    the result equals the reviewed expected output derived from the old
    generator for the same rows (ADR 0005; D-032).
  - [x] Sheet identifiers are entered in the UI; no code, test, fixture or
    document contains one (D-031). Done by T-023 (`docs/highscores.md`);
    the owner creates the six bindings from its table.

## M4 — Users, grants, API keys and the REST API

Outcome: other people can be given management of their pages or magazines in
the UI, and integrations can read and update pages with a scoped key. All
items are `critical` (D-006).

- [x] Users and grants `critical` (T-016) — admin creates, disables and resets users;
  grants of magazine, page or page range, each read or write.
  - [x] A non-admin sees only pages within their grants. (T-016)
  - [x] Every write route checks the grant on the server; the UI hiding a
    control is not the check. (T-016)
  - [x] Mutation check: removing the grant check from any write route fails
    a test naming the route. (T-016)
- [x] API keys `critical` (T-017) — create (secret shown once, stored hashed), revoke,
  grant shape plus read-only or read-write, last-used time.
  - [x] A revoked key is refused on the next request. (T-017)
  - [x] The stored form cannot be used as a bearer token. (T-017)
- [x] REST API v1 `critical` (T-017, T-018) — list pages, read a page as JSON rows or as TTI,
  replace a page from JSON rows or a TTI body; bearer-key auth; rate limit;
  audit log of every write naming the key or user.
  - [x] A read-only key cannot write; a key scoped to one page cannot read
    another. (T-018)
  - [x] A write outside the key's range is refused and logged. (T-018)
  - [x] Request and response shapes are documented in one OpenAPI file that
    a test validates against the running server. (T-018)
  - [x] Mutation check: each permission rule above has a test that fails
    when the rule is removed. (T-017, T-018)
- [x] Internet exposure checklist `light` (T-019) — document TLS termination, headers,
  rate limits and what to check before the API faces the internet.
  - [x] The document exists and each item names the code or config that
    satisfies it. (T-019)

## M2 — Output to the pages folder and Pi deployment

Moved after M4 by the owner (D-018): the hardware arrives at the end of the
project. Outcome: every transmit page is written to a configured folder (a
tmpfs on the Pi), and an edit made in the app changes that page's file within
one minute. The owner accepts this as sufficient for vbit2 (D-018); vbit2 is
not run as part of this milestone.

- [x] Output writer `standard` (T-021, run as `critical` by the architect) — on start and after every change, write the
  transmit set as `P<mpp>.tti` into the configured `pagesDir` atomically;
  remove files for deleted or non-transmit pages; coalesce bursts.
  - [x] After import plus start, the directory holds exactly the transmit
    pages and each file equals the page's download. (T-021 live check: 30 of 30 equal)
  - [x] Editing a page through the app replaces only that file, within one
    minute of the save (measured); deleting removes it. (T-021: 0.6 s test, 0.63 s live)
  - [x] A crash between temp write and rename leaves no partial `.tti` file. (T-021: `.tti` only via rename; stale tmp removed)
  - [x] A generated page whose scheduled run failed keeps its previous file
    (the output-directory half of the M3 scheduler criterion). (T-021)
- [x] Deployment kit `light` (T-024; the **(Pi)** criteria wait for hardware, D-029) — `deploy/` with a systemd unit, a tmpfs mount
  for the pages directory, an example config, and an install document for
  Raspberry Pi OS Bookworm Lite alongside the vbit2 install.
  - [ ] The install document is followed from a fresh image to pages on a
    TV. **(Pi)**
  - [ ] After a reboot the pages directory is regenerated and vbit2 shows the
    pages. **(Pi)**
  - [ ] An edit in the browser is visible on the TV within one minute.
    **(Pi)**

## M6 — Further features before cut-over

Added by D-033.

**Outcome:** the features the owner wants in the everyday tool before the
Pi 4 goes live, each usable on its own.

- New items are appended at the end of this milestone. Each one first needs
  a decision entry in `docs/decisions.md` that names it.
- Nothing here is dispatched without the owner's go-ahead (D-033; D-019 stop
  rules).

- [x] zxnet editor URL codec and export `standard` (T-025). A pure TypeScript module
  encodes one frame (rows 0 to 24, 40 cells of values 0 to 127) to the zxnet
  editor's URL fragment, and decodes a fragment back. It has hand-derived
  golden vectors. The page view gets an "Open in zxnet editor" link for the
  current subpage.
  - [x] Encoding a frame of 1000 spaces gives `"QIECBAg"` repeated 166
    times, then `"QIECA"` (1167 characters).
  - [x] Encoding a frame whose only non-zero cell is 0x7F gives:
    - at row 0, column 0: `"_g"`, then 1165 `"A"`;
    - at row 1, column 0: 46 `"A"`, then `"D-"`, then 1119 `"A"`.
  - [x] Each of these encodings decodes back to its frame.
  - [x] For every fixture page (Q-002), each subpage's rows 1 to 24 come
    back unchanged after encode then decode.
  - [x] A fragment that fails any of these checks is refused, with a message
    naming the fault:
    - it is 1120 or 1167 characters once trailing `=` are removed;
    - every character is in `A-Z a-z 0-9 - _`;
    - it has the `<meta>:<data>` shape.
  - [x] The page view link carries the current subpage's rows 1 to 24 as
    edited, row 0 as 40 spaces, and `PN=` set to the page number. It never
    carries `PS`, `SC` or `RE`.
- [x] Import a zxnet URL into a subpage, and create a page from one
  `critical` (D-033; T-026). On the page view, the user pastes a full URL or a bare
  fragment. It replaces rows 1 to 24 of the current subpage in the editor,
  and Save stores it. The page list's create form takes an optional URL and
  creates the page with that URL's rows.
  - [x] After import and Save, rows 1 to 24 of the chosen subpage equal the
    URL's rows. For an 1120-character fragment, row 24 is 40 spaces.
  - [x] In the stored bytes, these are unchanged: row 0, the description,
    the cycle time, the transmit flag, the links and every other subpage.
  - [x] A refused URL changes nothing in the editor and sends no request.
  - [x] A generated page shows no import control, and the existing rule
    refuses a save after import with 409.
  - [x] Create from URL makes the page at the number entered, or at the
    URL's `PN` when the number is blank. The page has the URL's rows and
    transmit off. A refused URL sends no request.
  - [x] Mutation check: removing any refusal in the packet's table makes a
    named test fail.

- [x] UI layout with Pico.css `standard` (D-035). Pico.css 2.x becomes the
  single global stylesheet, and the header navigation gains a Pages link. On
  the page view, the editor sits beside the preview and stacks on narrow
  screens. Inline form fields that Pico would stack are grouped. No other
  framework, plugin or theme work.
  - [x] `npm run build`, `vue-tsc --noEmit` and `vitest run` pass. There is
    one new devDependency and no build or test configuration change. The
    teletext snapshots are unchanged.
  - [x] The header lists Pages for every logged-in user, and the admin links
    for admins (`App.test.ts`).
  - [ ] Dark mode follows the OS. In both modes, the teletext preview in the
    gallery looks unchanged. (Owner browser check, dated.)
  - [ ] The page view shows the editor and preview side by side at 1280px
    and stacked at 800px. (Owner browser check, dated.)

- [x] Compact Pico sizing `light` (D-036). Pin Pico's root font size to 100%,
  tighten form spacing, keep the magazine buttons together, and take label
  text out of the grouped field rows.
  - [x] Build, `vue-tsc` and `vitest` pass with no test change.
  - [ ] At 1920×1080 the pages list and page view look compact. (Owner
    browser check, dated.)

- [x] Palette samples and fastext inputs `standard` (D-037).
  - Each palette button shows a live sample of its code, drawn by
    `TeletextPage`.
  - Flash animates and concealed text is dimmed, in the samples and in the
    page preview.
  - The six link inputs sit under Row 24, labelled by fastext colour.
  - [x] Build, `vue-tsc` and `vitest` pass. The teletext snapshots and
    `render.ts` are unchanged. The sample and link tests are added.
  - [ ] In both colour schemes, the samples, flash, conceal and the coloured
    link borders look right. (Owner browser check, dated.)

- [x] Palette sections and control-code overlay `standard` (D-038).
  - The palette is in three sections: Alpha, Mosaic and Other.
  - The buttons line up in a grid.
  - A "Show control codes" checkbox marks the code cells on the preview.
  - [x] Build, `vue-tsc` and `vitest` pass. `render.ts` and the snapshots
    are unchanged. The group, section and overlay tests are added.
  - [ ] The sections and the aligned buttons look right, and the overlay is
    readable on every background. (Owner browser check, dated.)

- [x] WYSIWYG editor core `standard` (D-039, T-031).
  - The 24 row inputs are replaced by a cursor on the rendered page, with
    typing in overwrite and insert modes, cursor keys and the mosaic sixel
    keys.
  - Clicking a cell moves the cursor, and a status line reports it.
  - A Grid view option sits beside "Show control codes".
  - [x] Build, `vue-tsc` and `vitest` pass, with the reducer, editor and
    page-view tests. `render.ts`, the snapshots and the API are unchanged.
  - [x] Saving an unchanged page sends every row as loaded.
  - [ ] The cursor lines up at 1280px and 800px, and editing feels right.
    (Owner browser check, dated.)
- [x] Control-code keys and help panel `standard` (D-039, T-032).
  - [x] Esc, then a letter, enters any of the 27 codes. Esc then 1-7, or
    Shift+1-7, enters the alpha or mosaic colours.
  - [x] Each button title names its key, and a Keys panel lists every key.
  - [x] Build, `vue-tsc` and `vitest` pass. `render.ts`, the snapshots and
    the API are unchanged.
  - [ ] The keys work on a UK keyboard, the Keys panel reads well, and the
    browser's Alt shortcuts still work. (Owner browser check, dated.)
- [x] Undo, redo and paste `standard` (D-039, T-033).
  - [x] An undo stack per subpage, on Ctrl+Z, Ctrl+Y and Ctrl+Shift+Z.
  - [x] Pasting a zxnet URL imports it; pasting plain text writes it at the
    cursor. Ctrl+V and Shift+Insert both paste.
  - [x] Build, `vue-tsc` and `vitest` pass.
  - [ ] Undo and paste work in Chrome and Firefox. (Owner browser check,
    dated.)
- [x] Full-height editor layout, and the grid class fix `standard` (D-040,
  T-034).
  - [x] Grid no longer breaks the editor: the class is `show-grid`, not
    Pico's `grid`.
  - [x] The menu bar is pinned. The page editor is full width, with a 4:3
    editor at full height, the fastext strip under it, and a side panel.
  - [x] Build, `vue-tsc` and `vitest` pass.
  - [x] The layout is right at 1920×1080, 1280×800 and about 800px wide.
    The coordinator measured it in the built-in browser on 2026-10-15; see
    the T-034 packet. The owner may still judge the look.
- [x] Agent browser checks: a disposable local server and login `standard`
  (D-041, T-035).
  - [x] `scripts/agent-server.ps1 start` serves on 127.0.0.1 with a
    disposable admin, and never prints the password. `stop` deletes
    everything.
  - [x] AGENTS.md describes when and how agents use it.
- [x] Agent browser checks: a credential-free login `critical` (D-042,
  ADR 0007, T-036).
  - [x] `GET /api/v1/agent-login` exists only with `-tags agentdev`, and
    serves loopback only. A tagged `serve` refuses a non-loopback listen.
  - [x] Both tag states are tested in `scripts/test.ps1`, and the mutation
    checks fail the tests as expected.
  - [x] `agent-server.ps1` prints a login URL and writes no password.
  - [x] The coordinator logs in from the built-in browser after
    integration (2026-10-15).
- [x] Compact control-code palette with key badges `standard` (D-043,
  T-037).
- [x] Mouse painting of mosaic sixels `standard` (D-039, D-044, T-038).
- [x] Block select, copy and cut `standard` (D-039, D-045, T-039).

## M7 — MS-DOS styling

Added by D-047.

**Outcome:** every screen of the site looks like an old MS-DOS application,
in the spirit of Norton Commander, Volkov Commander, shareware CDs and
late-night BBS file areas, replacing the bland Pico.css look. The owner's
visual reference is <https://lost-builds.byartem.com/>; it is a reference
for the style, not a design to copy.

- Research is the first task: the reference site and the named programs
  are not studied before it.
- Nothing here is dispatched without the owner's go-ahead.
- Carried from T-045: at narrow widths a focused menu item that is already
  partly visible is not scrolled into view; it needs a focus
  `scrollIntoView`.
- The teletext preview's colours, black page and geometry do not change.
  `TeletextPage.vue` is frozen in every M7 packet.

The architect's breakdown (2026-10-21), as amended by D-048. D-049 settles the approach, font and schemes.

- [x] M7-1 Style research `light` (`packet-researcher`, Opus). One
  document, no code: a style brief (palette, font, borders, controls, menu
  bar, focus), a shortlist of two or three approaches (at least one
  hand-rolled plain CSS) with licence, size and maintenance, font
  candidates with licences (self-hosted, no runtime network call), a WCAG
  AA contrast table, a recommendation for Q-040, a recommendation of one
  colour scheme or two (Q-039 c), and notes on the reference programs'
  keyboard conventions as input to M8.
- [x] M7-2 Theme foundation and Pico removal `standard`. The new global
  stylesheet replaces Pico in one step, so every screen is in the DOS
  theme. CSS plus fonts within the agreed bundle budget.
- [x] M7-3 Shell and admin screens `standard`. The header as a DOS menu
  bar, and the tables and forms of every admin screen. Styles only.
- [x] M7-4 Page editor screen `standard`. The side panel, palette,
  fastext strip, status line and Keys panel, keeping the D-040 geometry and
  the D-043 grid. It also removes the editor's "All pages" link (D-051).
- M7-5 (a function-key bar) is dropped by D-048; M8 covers it.
- [x] M7-7 Owner-check follow-up `standard`. Fields as dark wells, a grey
  footer bar with the Dark toggle, and Logout on the right (D-054, D-055).
- [x] M7-6 Close-out `light`. The owner's look-and-feel check in Firefox
  at three widths (D-055 dropped Chrome), ADR 0008, and the D-035
  supersession.

**M7 complete, 2026-10-22** (T-047): every checklist row passed in Firefox.

- [x] M7-9 Inset frame on grey windows and an `[X]` close box on the frame line
  `light`, the owner's findings on the T-049 check (D-057, T-050).
- [x] M7-8 Teletext50 preview font and a credits dialog `standard`, an owner
  request after close-out (D-056, T-049).

Every developer packet keeps `render.ts` and the snapshots unchanged, and
compares the preview inside `.tt-page` before and after in the browser.

## Owner requests after M7

- [x] M7-10 Help page and in-app help for Templates and Bindings
  `standard` (D-058, T-051).
- [x] M7-11 "Show template slots" toggle in the page editor (D-058, T-052).
- [x] M7-12 Replace a template in place, refused when a binding would lose
  a value `critical` (D-060, T-053).
- [x] M7-13 A failed fixed-value check no longer hides a successful template
  replace (D-060, T-054).
- [x] M7-14 Copy a binding into the new-binding form (T-055).

## M8 — Keyboard-driven UI

Added by D-048 (Q-041 c).

**Outcome:** the site can be driven from the keyboard like a DOS program:
a menu bar, function keys and panels, in the spirit of Norton Commander.

- It follows M7. The architect proposes its breakdown after M7-1 reports,
  using M7-1's notes on the reference programs' keyboard conventions.
- Nothing here is dispatched without the owner's go-ahead.
- Carried from T-045: at narrow widths a focused menu item that is already
  partly visible is not scrolled into view; it needs a focus
  `scrollIntoView`.

## M9 — CeeMS: rename and system settings

Added by D-061 and D-062 (owner request, 2026-10-24).

**Outcome:** the product is called CeeMS, the footer shows its version, and
an admin can set the system name and the vbit2 settings that CeeMS writes
to `vbit.conf`.

- [x] M9-1 Rename to CeeMS, with a version placeholder in the footer
  `light` (D-061, T-056). Dispatched only when no other worktree is open.
- [x] M9-2 Settings store, API and `vbit.conf` writer `critical`
  (D-062, T-057).
  - [x] Follow-up: the status message refuses spaces and `;` `light`
    (D-063, T-059).
- [x] M9-3 Settings page, and the editor preview uses the stored header
  `standard` (D-062, T-058).
  - [x] Follow-up: the menu bar shows the system name on the login page
    and after login `standard` (D-064, T-060).
- [x] M9-4 Version from the build `light`: `scripts/build.ps1` and the
  CI workflow set `VITE_CEEMS_VERSION` to the short commit hash
  (`git describe --always --dirty`) before `npm run build`, so the
  footer shows the commit of the running binary, and `dev` when unset.
- [x] Owner actions after M9-3: rename the GitHub repository, then the local
  folder, and the local config file to `ceems.json`. Done 2026-10-24.
## M10 — Fastext groups

Added by D-065 (owner request, 2026-10-24). ADR 0009.

**Outcome:** an admin keeps fastext links consistent with static and
paged groups and a central index link, and every member page follows a
change.

- [x] M10-1 Store: groups, membership, the FL rules, sweeps and the index
  settings `critical` (D-065, T-061).
- [x] M10-2 API: group routes, the index settings fields, the page and
  `me` documents `standard` (D-065, T-062).
- [x] M10-3 Web: the Fastext page, the editor's read-only links, the
  Settings fields `standard` (D-065, T-063).
- [x] M10-4 Store and API: link text per group and the generated row 24
  `critical` (D-066, T-064).
- [x] M10-5 Web: the 9-cell link text boxes, the row 24 preview and the
  read-only row 24 in the editor `standard` (D-066, T-065).

## M11 — Releases

Outcome: a pushed `vX.Y.Z` tag produces a GitHub release with archives a user
extracts and runs; the footer shows the version (D-069, ADR 0010).

- [x] M11-1 Release workflow, stamped build script, archive docs `standard`
  (D-069, T-073). Closes M9-4.
  - [x] Owner: push `v0.1.0` and check the release and its archives.
- [x] M11-2 A linux/armv6 build and archive for the Raspberry Pi 1 and Pi
  Zero `light` (D-072, T-077).

## M12 — Open-source release

Outcome: the repository and every release carry the AGPL-3.0 licence and the
notices its dependencies require (D-071).

- [x] M12-1 Licence, third-party notices, credits dialog and archives
  `standard` (D-071, T-075).
- [x] M12-2 CodeQL alerts triaged: the backup query fix, a TTI parser fuzz
  test and a disabled pages directory box `light` (T-076).
- [x] M12-3 Generic test fixtures; no station or old product names in the
  tree `standard` (D-074, T-080).
- [x] M12-4 Public history squashed to one commit from the verified tree; the
  GitHub repository recreated by the owner `light` (D-075). The history
  rewrite (T-081) was abandoned.
- [x] M12-5 Agent process files moved to the private `ceems-dev` repository;
  fixture line endings fixed `light` (D-075, T-082).
- [ ] M12-6 Formatter fixes: decimals option, one-per-line Formats guidance
  `standard` (D-076, T-083).

## M5 — Cut-over

Outcome: the Pi 4 is the live service and the old system is retired.

- [ ] Cut-over `standard` — import the live page set on the Pi 4, run both
  machines side by side, switch the TV, stop the old generator.
  - [ ] Every page on the old system renders identically on the new one.
    **(Pi)**
  - [ ] High-score pages update hourly from the new scheduler for one day.
    **(Pi)**
  - [ ] The old generator and its key are removed from the Pi 1B.

## Later, not scheduled

- Visual mosaic editor in the browser.
- Page revision history and undo.
- In-vision output through vbit-iv.
- Page enhancement packets (rows 26 to 29) beyond pass-through.
