# 0008 — The MS-DOS theme: one hand-rolled stylesheet, a self-hosted VGA font, two schemes

Status: `Accepted` (amended 2026-10-22, D-055; amended 2026-10-22, D-056; amended 2026-10-22, D-057; amended 2026-10-23, D-059)
Date: `2026-10-22`
Raised by: `T-047` (M7-6)
Decided by: `owner` (D-049 to D-052), on the architect's T-043 recommendation and the T-044 to T-046 design records (recorded as D-053)

## Context

- **What the owner wanted.** D-047: Pico.css looked bland; every screen is
  to look like an old MS-DOS program, in the spirit of Norton Commander.
  D-048 left the number of colour schemes to research and moved the
  keyboard-driven UI to M8, so M7 is a restyle only.
- **What D-035 and D-036 fixed.** `@picocss/pico` 2.x was the single global
  stylesheet, with a small `web/src/app.css` pinning its root font size and
  form spacing (`docs/decisions.md:664-713`). D-035 claimed the teletext
  preview "sets every property it depends on"; T-043 found this only partly
  true, because `TeletextPage.vue` inherits `font-weight`, `font-style`,
  `letter-spacing`, `text-transform`, `text-shadow`, `text-rendering` and
  font smoothing from its ancestors
  (T-043).
- **What ADR 0001 fixes.** "No component library"
  (`docs/adr/0001-stack-and-runtime-shape.md:104`). Pico passed that test as
  CSS only; this ADR removes the package altogether.
- **The frozen preview.** `web/src/teletext/TeletextPage.vue` and
  `render.ts` are frozen (D-049). The theme must leave the preview
  pixel-identical in both schemes.
- **The budget.** D-049 keeps the earlier budget: theme CSS plus fonts no
  larger than 83 KB, which T-043 read as Pico alone, `pico.min.css` at
  83,319 bytes (`T-043-style-research.md:169-175,787-789`).
- **Geometry that depends on the header.** The page editor's 4:3 layout
  (D-040) subtracts the header height from the viewport
  (`web/src/PageView.vue:261,266`), so the theme owns one variable the
  editor reads.
- **Research.** T-043 sections 4, 5 and 8
  shortlisted three CSS approaches and three fonts, with licences, sizes
  and bare-element side effects, and recommended option A with the int10h
  font.

## Options

1. **Hand-rolled plain CSS plus a self-hosted VGA font** (T-043 §4A, §5.1):
   one global `web/src/theme.css` using element and attribute selectors,
   replacing Pico and `app.css`; `Web437 IBM VGA 8x16` under CC BY-SA 4.0.
   Cost: about 7 KB of CSS the project owns, a visible credit and a licence
   note, and a test that keeps the stylesheet off the preview.
2. **TuiCss 2.1.2** (T-043 §4B): MIT, 34,470 bytes plus 22 images, 178
   class-driven `.tui-*` rules needing heavy overrides, no release since
   2023, and a bundled font ("Perfect DOS VGA 437") with no clear licence.
3. **WebTUI 0.1.10** (T-043 §4C): MIT, 40,069 bytes, pre-1.0, a modern
   terminal look rather than DOS, and a `*` rule that removes focus outlines.
4. **Keep Pico (do nothing)** (T-043 §8 option c): the package but not the
   VGA look. It fails the D-047 goal.

Font alternatives considered: Departure Mono 1.500 (OFL 1.1, a modern pixel
font, less authentic) and Unscii 2.1 (public domain, 67,300 bytes, about 80%
of the budget) (T-043 §5.2-5.3). BOOTSTRA.386 and terminal.css were rejected
outright (T-043 §4 "Rejected").

## Decision

Option 1 (D-049). No package passed every Q-040 test, and a stylesheet of
this size is cheaper to own than to override. It supersedes D-035's Pico.css
choice and D-036's compact Pico sizing; `@picocss/pico` is uninstalled and
`web/src/app.css` is deleted (T-044).

**The stylesheet.** `web/src/theme.css` is the only stylesheet, imported once
in `web/src/main.ts:2`. Views add their own scoped composition. It has no
media query and its only at-rule is the `@font-face` (`theme.css:4-8`).

**The font.** `web/src/assets/fonts/Web437_IBM_VGA_8x16.woff`, by VileR,
from the int10h Oldschool PC Font Resource v2.2, copied unmodified
(9,788 bytes, SHA-256 recorded in the licence note). Vite hashes it into
`dist/assets/`; the Go binary serves it with `Content-Type: font/woff`
(T-044 handover). The licence is Creative Commons Attribution-ShareAlike 4.0
International. Its obligations are met in two places (D-050, owner option c):

- `web/src/assets/fonts/LICENSE.md` beside the font file: name, author,
  source, pack version, download date, SHA-256, "unmodified", the licence
  link, and the statement that the app's code is not a derivative of the
  font;
- a visible credit, the last paragraph of the Keys help `details` in the
  page editor (`web/src/editor/KeysHelp.vue:25`): "Font: Web437 IBM VGA 8x16
  by VileR, CC BY-SA 4.0", linking <https://int10h.org/oldschool-pc-fonts/>
  and <https://creativecommons.org/licenses/by-sa/4.0/>. `KeysHelp.test.ts`
  asserts the text and both hrefs. There is no footer.

The `font-family` stack is `"Web437 IBM VGA 8x16", ui-monospace, Consolas,
"DejaVu Sans Mono", monospace`, at `font-size: 16px` and
`line-height: 16px` on `:root` (`theme.css:22-24`), with
`font-display: block`.

**Tokens and the two schemes** (D-049, D-050). Custom properties on `:root`
(`theme.css:12-29`), in the 16-colour VGA palette:

    --desk --text --bright --accent --panel-text
    --frame --bar --bar-text --hotkey
    --current-bg --current-text --hover-bg
    --dialog --dialog-text --dialog-frame
    --field-bg --field-text --field-placeholder
    --dialog-field-bg --dialog-field-text --dialog-field-placeholder
    --error-bg --error-text --focus
    --shadow --code-bg --code-text --disabled
    --app-header

`--hotkey` is defined but nothing uses it yet.

- **Commander** is the default: blue desk `#0000AA`, grey text `#AAAAAA`,
  yellow accents `#FFFF55`, cyan frames `#55FFFF`, cyan bars `#00AAAA` with
  black text.
- **Dark** overrides four tokens on `:root[data-scheme="dark"]`
  (`theme.css:31`): `--desk:#000000`, `--frame:#AAAAAA`, `--shadow:#555555`,
  `--disabled:#555555`. Bars, fields, dialogs and focus are identical in
  both schemes.
- The OS setting is not followed; there is no `prefers-color-scheme` query.
  A "Dark" button with `aria-pressed`, the last item in the header nav
  (`web/src/App.vue:50`), toggles the scheme on every screen including
  login.
- The choice is kept per browser in `localStorage` under the key
  `ceems.scheme`, value `dark`; anything else means Commander. Every read
  and write is inside `try { } catch`. An inline classic script, the first
  child of `<head>` in `web/index.html:4`, applies `data-scheme="dark"` to
  `<html>` before first paint:

      <script>try{if(localStorage.getItem('ceems.scheme')==='dark')document.documentElement.dataset.scheme='dark'}catch(e){}</script>

- Each scheme has its own WCAG AA contrast table: Commander in T-043 §6,
  Dark in the T-044 design record (Decision 3) and D-050, with the shell
  and editor pairs in the T-045 and T-046 design records. Two Dark pairs
  fail and are accepted: the red invalid-field boundary on black (2.71:1),
  covered by the yellow focus outline on every `aria-invalid` field, and
  disabled text, which SC 1.4.3 exempts. The editor cursor's inverting
  `mix-blend-mode: difference` block is kept at 2.29:1 on green–magenta
  (D-049, D-052).

**Composition primitives** (D-051, D-052).

- **The menu bar**: `header` is a sticky black-on-cyan bar, `height:
  var(--app-header)` where `--app-header: 2rem` (`theme.css:21,62`), holding
  one row of items with room above and below so the offset focus ring lands
  on cyan. Items are `white-space: nowrap; overflow-x: auto` with the
  scrollbar hidden, so the bar scrolls sideways at narrow widths rather than
  wrapping.
- **`fieldset.panel`**: a `3px double var(--frame)` box with the view's
  heading in its `legend`, a hard `2ch 1em` shadow and `overflow-x: auto`
  (`theme.css:88,94`). Every admin screen and the page editor's side column
  sit in one. Wide tables pan inside the panel; there is no wrapper element.
- **`fieldset.dialog`**: the grey variant (`#AAAAAA`, black frame,
  yellow-on-black fields, black focus rings) for Login and Change password,
  centred and `width: fit-content` (`theme.css:96-101`).
- **Checkboxes** keep their native role; `appearance: none` and `::before`
  draw `[ ]` and `[X]` (`theme.css:80-83`). Radios stay native.

**Three interaction states**, each a distinct look:

- **Focus**: `:focus-visible { outline: 2px solid var(--focus);
  outline-offset: 2px }` (`theme.css:127`), yellow on the desk. Where
  yellow would land on cyan or grey (2.69:1 and 2.18:1) the ring is
  recoloured black: `header :focus-visible` and `.dialog :focus-visible`
  (`theme.css:69,101`).
- **Current or pressed**: white on black, via `a[aria-current="page"]` and
  `button[aria-pressed="true"]` (`theme.css:43,58,68`). Hover is black on
  light cyan and loses to current or pressed by rule order.
- **Selected**: `tr[aria-selected="true"]` rows are black on cyan
  (`theme.css:108-113`); the editor's selection overlay is a two-colour
  ring (`web/src/editor/PageEditor.vue:216`).

**The two-colour ring** is the pattern for any ring that cannot pass 3:1 in
one colour on every surface it may touch (WCAG C40): a 2px yellow outline
outside a 2px black `box-shadow` in selected table rows (`theme.css:113`),
and a 1px yellow outline outside a 1px black inset shadow with a 30% yellow
fill on the editor selection (`PageEditor.vue:216`). On every teletext
colour at least one line passes 3:1 (T-046 design record, item 4).

**The editor overlays** (D-049, D-052): the editor focus outline is the
theme's `--focus` ring; the selection is the two-colour ring; the cursor is
unchanged. The status line is a one-row black-on-cyan bar with a 1rem desk
gap above it so the editor's ring lands on the desk; `--strip` is `2rem`
and `--side` is `22rem` (`PageView.vue:260`).

**Keeping the preview frozen.** `web/src/theme.test.ts` reads `theme.css`
and asserts, over every rule outside an at-rule:

1. no selector targets bare `i`, `b` or `span` (`theme.test.ts:17-19`);
2. no selector names a `TeletextPage` class (`tt-page`, `tt-row`,
   `tt-cell`, `g`, `code`, `mosaic`, `sep`, `on`, `flash`, `conceal`,
   `ctl`, `show-grid`, `dh-top`, `dh-bottom`, `aspect-4-3`, `aspect-16-9`,
   `fg-0..7`, `bg-0..7`) and no `--rows`, `--cols` or `--sx` custom property
   (`:21-25`);
3. the inheritable text properties (`text-transform`, `letter-spacing`,
   `word-spacing`, `text-shadow`, `font-weight`, `font-style`,
   `font-variant*`, `font-feature-settings`, `font-synthesis`, font
   smoothing, and `font` other than `font: inherit`) appear only in rules
   whose every selector starts with `h1`–`h6`, `th`, `td`, `legend`, `kbd`,
   `code`, `a`, `small`, `summary`, `input`, `select`, `textarea`,
   `::placeholder` or `::file-selector-button` (`:27-37`);
4. the `*, *::before, *::after { box-sizing: border-box; }` reset is present
   and there is no `prefers-color-scheme` query (`:39-42`);
5. the only at-rule is `@font-face`, so no rule hides inside a block the
   checks above skip (`:44-46`).

`:root` sets `text-rendering: optimizeLegibility`, which Pico set before, so
glyph spacing in the preview does not shift (T-043 §10 item 2). The T-044 to
T-046 before/after computed-style captures of the preview were identical
apart from the geometry that follows the header height.

**Bundle.** A build of `main` at `c3dcec2` (2026-10-22) gives
`dist/assets` CSS 11,083 + WOFF 9,788 = **20,871 bytes**, a quarter of the
83,319-byte limit. T-044 measured 20,190 bytes.

## Consequences

- **Easier.** One file to read for the whole look; no package to keep
  current; no root font-size steps to pin (D-036's problem is gone); the
  theme is a 7 KB stylesheet and a 10 KB font.
- **Harder or ruled out.**
  - The OS dark setting is ignored (D-049). A third scheme or OS-following
    is a new decision.
  - There are no media queries in `theme.css`; responsive behaviour is
    scoped to views, and the menu bar scrolls rather than wraps. A focused
    menu item can sit partly clipped at 360px (M8).
  - `.dialog` field rules outrank `[aria-invalid]` by order, so an invalid
    field inside a dialog would stay yellow on black. No dialog has one;
    fix it in the packet that adds one.
  - CC BY-SA 4.0 binds any modified copy of the font (a subset or
    conversion) to the same licence. Keep the file unmodified or carry the
    licence with the derivative.
- **Invariants future packets must respect**, changed only by a new ADR:
  - `web/src/teletext/TeletextPage.vue` and `render.ts` stay frozen; the
    preview is identical in both schemes.
  - No inheritable text property sits on any element that can be an
    ancestor of `.tt-page`. `theme.test.ts` enforces the allowed selector
    list above; a new element that needs one goes in that list with a
    reason, not in a bypass.
  - `theme.css` has only the `@font-face` at-rule: no `@media`, `@layer`,
    `@supports` or `@import`, enforced by `theme.test.ts`.
  - The D-040 geometry follows `--app-header`. Change the bar's height
    there and nowhere else; `PageView.vue` reads it.
  - Focus rings are measured where they land, at the 2px offset, on every
    surface they can touch, and need 3:1 there. The yellow ring is default;
    recolour it (black on bars and dialogs) or use the two-colour ring where
    one colour cannot pass.
  - The scheme is `data-scheme="dark"` on `<html>`, stored under
    `ceems.scheme` in `localStorage`, read in `web/index.html` before
    first paint and in `App.vue`. A third reader means a shared module.
  - The font file is served self-hosted from `dist/assets`; no runtime
    network call, no `web/public/`. The credit in `KeysHelp.vue` and
    `web/src/assets/fonts/LICENSE.md` stay as long as the font does.
  - The bundle budget is 83,319 bytes (83 KB) for theme CSS plus fonts.
  - No CSS package, no PostCSS, no Vite CSS plugin. New screens use the
    primitives here (`fieldset.panel`, `fieldset.dialog`, the menu bar, the
    `aria-*` state selectors) before adding rules.

## Amendment 2026-10-22 (D-055): fields as dark wells, the footer bar, Logout on the right

T-048 implements this. It changes the facts below, and every other part of
this ADR stands.

- **Style direction.** The look is inspired by Norton Commander and other
  DOS programs, and is not limited to Commander. Grey windows on the blue
  desk are fine where they make content stand out.
- **Tokens.** `--dialog-field-bg`, `--dialog-field-text` and
  `--dialog-field-placeholder` are removed. `--field-bg:#000000`,
  `--field-text:#FFFF55` and `--field-placeholder:#AAAAAA` take the old
  dialog values. `--field-edge:#AAAAAA` and `--app-footer: 2rem` are new.
  `theme.test.ts` asserts that `--field-bg` differs from `--bar` and
  `--desk`.
- **Fields.** Every text input, select and textarea, on any surface, is a
  black well with yellow text and an inset 1px `--field-edge` line
  (`box-shadow: inset 0 0 0 1px`).
  - Invalid fields drop the line.
  - Disabled fields use `--disabled`.
  - Buttons are unchanged cyan blocks, so a field and a button side by
    side read as two controls.
  - The fastext fields keep their coloured faces, with black text and no
    edge.
- **The footer bar** is a new composition primitive. It is a sticky
  `footer` at `bottom: 0`, `height: var(--app-footer)`, black text on
  `--dialog` grey, and it shares the menu bar's item rules through
  `:is(header, footer)`. It holds an optional text line ("Copyright
  placeholder"), a GitHub link and the Dark toggle, on every screen
  including Login.
  - `#app` is a flex column with `min-height: 100dvh`, and `main` takes
    `flex: 1`. That keeps the footer at the window's bottom on short
    pages.
  - The font credit stays in `KeysHelp.vue`. The Decision's "There is no
    footer" is replaced by this.
- **The Dark toggle** is the last item in the footer nav, not the header
  nav.
- **Header.** Its right-hand group is the username (Change password) link,
  then Logout.
- **Geometry invariant**, replacing the one above: the D-040 geometry
  follows `--app-header` and `--app-footer`. Change the bars' heights there
  and nowhere else; `PageView.vue` reads both. The preview is 1248×936 at
  1920×1080, 1008×756 at 1600×900, and 640×480 at 1024×768.
- **Constraint for M8.** `button:disabled` outranks the footer's item rule,
  and `--disabled` is `#AAAAAA`. So a disabled button in the grey footer
  would show grey on grey (1:1 in Commander). M8's key bar must not disable
  footer buttons without adding a footer-specific disabled colour.
- **Bundle.** T-048 measured 21,210 bytes of CSS plus font, against the
  83,319-byte budget.

## Amendment 2026-10-22 (D-056): Teletext50 for the teletext preview

T-049 implements this. It changes the facts below, and every other part of
this ADR stands.

- **Preview font.** Every `TeletextPage` (the editor, the palette samples
  and the gallery) uses Teletext50 by Alan Davies, a public-domain tweak of
  Bedstead by Ben Harris. It is vendored unmodified as
  `web/src/assets/fonts/Teletext50.otf` (128,692 bytes, SHA-256 in
  `LICENSE.md`, asserted by `TeletextPage.font.test.ts`). The UI keeps
  Web437.
- **Geometry.** The font's em is the SAA5050 6×10 cell, with a 0.6em
  advance. `.tt-cell` font size is `calc(100cqh / var(--rows))`. `--sx` is
  `calc(25 / 18)` in 4:3 and `calc(50 / 27)` in 16:9, so each scaled glyph
  is exactly one cell wide; at 1920×1080 the glyph and cell are both
  31.2×37.44px. Double height is unchanged.
- **`@font-face` placement.** Teletext50's `@font-face` lives in
  `TeletextPage.vue`'s scoped style. `theme.css` keeps Web437 as its only
  at-rule.
- **Frozen preview**, replacing the freeze above: `TeletextPage.vue` and
  `render.ts` change only through an owner-raised packet with an architect
  design record. `render.ts` is unchanged by T-049.
- **Budget.** The CSS-plus-fonts budget is 160,000 bytes, replacing
  83,319. T-049 measured 150,302 bytes: 128,692 (otf), 9,788 (woff) and
  11,822 (css).
- **Credits dialog.** This is a new primitive: a native `<dialog>`
  (`CreditsDialog.vue`) opened with `showModal()`, wrapping a
  `fieldset.dialog`, with a `[■]` close box on the top frame line at the
  right and a 50% black backdrop. The browser handles focus, Esc and focus
  return; there is no focus code, and a backdrop click does not close it.
- **Credits.** Both font credits live in `CreditsDialog.vue`, not
  `KeysHelp.vue`. `CreditsDialog.test.ts` asserts them, and `KeysHelp.test.ts`
  asserts the Keys panel carries none. CC BY-SA 4.0 §3(a)(2) is met by a
  credit one click away on every screen.
- **Footer.** The first list is the text line, GitHub, then Credits; Dark
  stays on the right.

## Amendment 2026-10-22 (D-057): inset frame on grey windows, `[X]` close box

T-050 implements this. It changes the facts below, and every other part of
this ADR stands.

- **Grey windows.** `.dialog` carries `box-shadow: 0 0 0 0.5em
  var(--dialog), 2ch 1em 0 0.5em var(--shadow)`. A grey band surrounds the
  black double frame, and the hard shadow is offset from the band's edge.
  This covers Login, Change password and Credits. `.panel` is unchanged.
- **Close box.** The Credits close box reads `[X]`, not `[■]`. It is
  positioned from the modal `<dialog>` (`top: 0.5em; right: 2ch`, with the
  fieldset at `margin: 0.5em 1ch`), never from the fieldset, because
  Firefox and Chromium disagree on a fieldset's positioning origin. The
  dialog is `overflow: visible`, so the band and the shadow are not
  clipped.

## Amendment 2026-10-23 (D-059): template slot highlight in the preview

T-052 implements this. It changes the facts below, and every other part of
this ADR stands.

- **`TeletextPage.vue`** changed under an owner-raised packet with an
  architect design record, as the D-056 amendment requires:
  - a `showSlots?: boolean` prop, default `false`;
  - a `slot` cell class;
  - one scoped rule,
    `.tt-cell.slot { box-shadow: inset 0 -2px #000, inset 0 -4px #ff0; }`.

  Inside a slot token, cells draw raw ASCII (`String.fromCharCode`), so
  `{ } ^ _` show as typed. Everywhere else the English map still applies.
  `render.ts` and its `ENGLISH` map are unchanged.
- **`web/src/teletext/slots.ts`** mirrors `internal/tmpl/tmpl.go`'s
  `slotRE`. Keep the two in step: a grammar change on either side needs the
  other.
- **Frozen-class list.** `theme.test.ts`'s TeletextPage class list gains
  `slot`.
- **The slot bar** is a third use of the two-colour pattern: a black line
  inside a yellow one. Its contrast on the eight teletext colours was
  judged by eye, not measured.
- **Default and scope.** The default preview is unchanged and identical in
  both schemes. "Show template slots" is a session-only editor view option,
  disabled on bound pages, and never drawn in the palette or gallery.
- **Accepted behaviours:**
  - with Grid on, the bar replaces the grid line on slot cells;
  - a token in the hidden source row under a double-height row is not
    highlighted, though Go fills it.
- **Bundle:** one scoped rule, which is negligible against the
  160,000-byte budget (T-049 measured 150,302).
