# 0009 — Fastext groups and the index setting own parts of a page's FL lines

Status: `Accepted`
Date: `2026-10-24`
Raised by: M10 (D-065)
Decided by: `owner` (D-065), on the architect's design record in `T-061`

## Context

- A page's fastext links are the TTI `FL` line, six links per subpage,
  stored inside the page's TTI bytes (`internal/tti/tti.go:371-404`).
- ADR 0004 says `SavePage` stores what the client sent. ADR 0005 says a
  binding's page is written only by its generator.
- The owner wants shared links kept consistent across many pages, and a
  central index link.

## Decision

- **Rules own slots, not pages.** A rule rewrites only the slots it owns,
  on every subpage. A page's group owns slots 1-4; for a paged group, two
  of those are Previous and Next, computed from the member order with no
  wrap. The index setting, when on, owns slot 6. Slot 5 is never owned.
- **Applied to stored bytes**, in the store's single write path
  (`saveLocked`), so the editor, TTI download and air agree, and every
  writer (editor, TTI upload, generator) follows the rules.
- **Sweeps.** A group change, a membership change, a page create, delete
  or move in a paged group, and an index setting change rewrite the
  affected pages in one transaction, through the same write path.
- Groups, membership and the index setting are admin-only.

## Consequences

- Amends ADR 0004: a save may store different FL slots from those sent.
- Amends ADR 0005: a sweep may write a bound page's FL slots; the next
  generation stores the same slots.
- A sweep changes member ETags, so an editor open on a member must reload
  before saving.
- Each changed page gets a version row and an audit row; enabling the
  index writes one per page.
- The rule runs on the unattended hourly generation path.

## Amendment — row 24 (D-066, 2026-10-25)

- A group also owns row 24 on every subpage of its members: four 10-cell
  fields (colour code plus 9 cells of text) in red, green, yellow, cyan
  order. Prev and Next fields carry fixed text; an `8ff` end is blank.
- Row 24 is applied by the same write path and sweeps as the slots.
- Leaving a group writes an empty row 24, so this rule, unlike the slots,
  changes a page's bytes when it leaves.
- Amends ADR 0004 further: a save of a member page may store a different
  row 24 from the one sent.
