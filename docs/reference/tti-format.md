# TTI page file format: reference for agents

- Source: MRG Systems Ltd, "MRG Teletext Page Format: TTI Page file
  format", Version 1.1 (first issue 04/02/2005; v1.0 draft 19/12/2004),
  11 pages. <https://zxnet.co.uk/teletext/documents/ttiformat.pdf>
- Summarised 2026-10-22 by `packet-researcher`, in its own words, and
  checked by the coordinator against `internal/tti/tti.go`. The source
  forbids reproduction without MRG's permission, so read the PDF for exact
  wording.
- Scope: the source defines only PN, CT, DE, PS, SC, OL and FL. It does not
  define other codes seen in real files (DS, SP, RE, PF, RD, MS). Section 4
  says what ceems does with them.
- Marking:
  - **[src]**: stated in the source.
  - **[inferred]**: deduced by the summariser.
  - **[repo]**: observed in this repository.

## 1. File basics [src]

- Extension `.tti`. A text file with one command per line, each line ended
  with CR LF.
- Every line starts with a two-letter command code, then a comma, then its
  parameters.
- The format stores teletext control characters in a text file by moving
  codes below 0x20 up into 0x80-0x9F. It also carries page metadata: the
  page number, magazine and so on.
- It comes from MRG's MiniTED and TED Scheduler. The source's advice to use
  8.3 filenames applies only to MiniTED.

## 2. Line types

**Order.** The source gives no strict ordering rule. Its sample runs DS, DE,
PN, CT, SC, PS, then the OL lines, then FL. [repo] CeeMS treats lines
before the first PN as page-level, and later lines as belonging to the PN
before them (one subpage). Real vbit2 files put each subpage's lines after
its PN.

| Code | Purpose | Syntax | Fields, values, defaults |
|---|---|---|---|
| PN | Page number; starts a (sub)page | `PN,mppss` | m = magazine digit, pp = page, ss = subpage. Five hex digits, e.g. `PN,10003`. The source gives no ranges or default; the magazine is normally 1-8 [inferred]. |
| SC | Subcode | `SC,ssss` | Four hex digits, 0000 to 3F7F. Normally equal to the PN's ss. |
| PS | Page status: control bits and the transmit flag | `PS,ssss` | Four hex digits; see the bit table below. `8000` = normal parallel transmission; `8040` = normal serial. |
| CT | Cycle time for rotating subpages | `CT,n,t` | n = seconds. t = `C` (cycled: n magazine cycles before display) or `T` (timed: queued for transmission after n seconds). |
| DE | Description, informational only | `DE,text` | Up to 24 characters, per the source (MiniTED page list). No effect on transmission. |
| OL | One teletext row | `OL,nn,line` | nn is 1 to 27, per the source. `line` is the row content, encoded as in section 3. The source gives no length limit; a teletext row is 40 cells [inferred]. |
| FL | Fastext links | `FL,red,green,yellow,cyan,link,index` | Six fields: red, green, yellow, cyan, a fifth link, and an index link. Each is a three-digit `mpp` page number, or `0` for no link per the source. The source does not explain the fifth and sixth fields. Sample: `FL,151,200,250,100,0,100`. |
| DS | Destination, e.g. `DS,inserter` | | Appears only in the source's sample, not defined. |

The source does not define SP, RE, RD, MS or PF, so do not infer their
meaning from it. [repo, inferred] In this repository's fixtures, SP holds a
path to the source file, and `RE,0` looks like a region code. Keep
these lines and pass them through.

### PS bits [src], values from the source's table

| Flag | Hex | Bit | Teletext control bit | Note |
|---|---|---|---|---|
| Transmit page | 8000 | 15 | — | The page is broadcast. CeeMS's "Transmit" is this bit. |
| Erase page | 4000 | 14 | C4 | |
| Substitute page | 0800 | 11 [inferred from hex] | — | The source leaves the bit column blank. |
| Language | 0000-0380 | 7-9 [inferred from hex] | C12-C14 | |
| Serial magazine | 0040 | 6 | C11 | Clear means parallel transmission. |
| Inhibit display | 0020 | 5 | C10 | |
| Interrupted sequence | 0010 | 4 | C9 | |
| Update | 0008 | 3 | C8 | |
| Suppress header | 0004 | 2 | C7 | |
| Subtitle | 0002 | 1 | C6 | |
| Newsflash | 0001 | 0 | C5 | |
| Timed page | — | — | — | Use the CT line instead. |

**Language codes** (3 bits, Western European sets):

| Bits | Language |
|---|---|
| 000 | English |
| 001 | German |
| 010 | Swedish/Finnish |
| 011 | Italian |
| 100 | French |
| 101 | Portuguese/Spanish |
| 110 | Czech/Slovak |
| 111 | undefined |

**Caveats:**

- The source's one-line bit diagram puts some flags at different positions
  from its own table. This summary follows the table, which matches the hex
  values.
- [inferred] The sample's `PS,8100` is transmit plus language bits 010.

## 3. Character encoding [src unless marked]

- **Control codes.** Teletext cell codes below 0x20 cannot sit in a text
  file. In an OL line, a control code `c` is written as byte `c + 0x80`
  (0x80-0x9F).
- **The ESC form**, for seven-bit systems: ESC (0x1B) followed by the code
  plus 0x40. To decode, ESC x gives x − 0x40.
- **Double height** is 0x0D, which can upset text parsers, so files often
  use 0x10 instead. An MRG inserter maps it back on transmission.
- **Other codes**, 0x20 and above, are written as themselves.
- **Row numbers.** The source allows rows 1-27 for OL and gives no cell
  count. [repo] Real files also use row 0 (the header) and row 28
  (enhancement data).
- **Mixed forms.** [repo, ADR 0002] Files in the wild mix the ESC and 0x80
  forms, and CRLF and LF line endings.

## 4. CeeMS support [repo]

Read for this section:

- `internal/tti/tti.go` and `tti_test.go`;
- `internal/httpapi/pages.go`;
- `web/src/teletext/tti.ts` and `web/src/PageView.vue`;
- `docs/adr/0002-page-storage-and-output-pipeline.md`;
- `testdata/sample/pages/*.tti`.

How ceems handles TTI:

- **The codec** keeps a page as an ordered list of raw lines, with their
  terminators. It keeps unknown codes and lines with no `XX,` prefix.
  Serialising a parsed page gives back identical bytes
  (`internal/tti/tti.go:1-8`, `:139-145`).
- **The browser editor** does not edit TTI directly. It sends JSON to the
  API, which builds the document (`internal/httpapi/pages.go:46-81`) and
  applies changes back (`:199-235`).
- **`web/src/teletext/tti.ts`** is used only for tests and the gallery. It
  reads the PN and OL lines.

| Code | Parsed by `internal/tti` | Written by ceems | Used by the editor | Notes |
|---|---|---|---|---|
| PN | Validated: exactly 5 hex digits, magazine 1-8, the same mpp on every PN (`tti.go:93-103`, `:68-77`). A file with no PN is rejected (`:87-89`). | `NewPage` (`tti.go:504`). `AddSubpage` sets the last two digits to the new index (`:438-450`). `SetMPP` rewrites the page part and keeps the suffix (`:159-164`). | Indirectly: the subpage tabs and add/remove subpage, 1-99 (`PageView.vue:87-98`, `pages.go:203-210`). | Lowercase hex is accepted. |
| SC | Passed through, no rule. | `NewPage` writes `SC,0000`. `AddSubpage` writes `%04X` of the new index (`tti.go:447-448`). | No. | Existing SC lines are never checked or kept in step with PN. |
| PS | Validated as 1-4 hex digits (`tti.go:105-111`). | `SetTransmit` sets or clears bit 15 on every PS line. When turning it on in a file with no PS line, it inserts `PS,8000` after the first PN (`tti.go:406-433`). `NewPage` writes `PS,0000`. | The Transmit checkbox (`PageView.vue:203`, `pages.go:233`). | Only bit 15 is understood. The other bits are kept on edit but not shown. Transmit is true if any PS line has the bit set. |
| CT | `ParseCT` accepts `n,T` or `n,C` with n ≥ 0 (`tti.go:473-482`). Not checked when the file is parsed. | `SetField(i,"CT",...)` from `applyCycle` (`pages.go:237-253`). `NewPage` writes `CT,8,T`. | Mode and seconds per subpage. The UI allows 1-999 seconds (`pages.go:245`). | Matches the source's syntax. |
| DE | `Description()` returns the first DE (`tti.go:180-188`). | `SetField(-1,"DE",...)` (`pages.go:230-232`, `jobs/runner.go:156`). `NewPage` writes `DE,`. | The Description input, `maxlength=255` (`PageView.vue:193`). | The source says at most 24 characters. CeeMS allows 255 (`pages.go:227`). |
| OL | Validated: the row is a decimal number 0-29, with at most 40 cells; an ESC and the byte after it count as one cell (`tti.go:113-136`). | `SetRow` replaces or inserts the row in order, in the file's control-code form: ESC, unless the file already uses 0x80-0x9F. Values above 127 are refused (`tti.go:267-344`). | Rows 1-24 are editable (`pages.go:218`). Row 0 is read-only (`pages.go:61,73`). Rows 25-29 are neither shown nor edited. | Wider than the source's 1-27, because real files carry rows 0 and 28. The form detection ignores rows 25-29 (`tti.go:271`). |
| FL | `ParseFL` needs six fields, each exactly three hex digits (`tti.go:484-498`). Not checked when the file is parsed. | `applyLinks` writes six lowercased links (`pages.go:255-274`). `NewPage` writes `FL,8ff,8ff,8ff,8ff,8ff,8ff`. | Six link inputs; the first four are the coloured keys. | CeeMS uses `8ff` for no link, as every fixture in `testdata/sample/pages` does. A file using the source's bare `0` fails `ParseFL`, so the editor would show none of that page's links. |
| DS | Passed through. | Never written. | No. | In 25 files in `testdata/sample/pages`. |
| SP | Passed through. It is not named in the `tti.go` header comment. | Never written. | No. | In 25 files in `testdata/sample/pages`: a file path. |
| RE | Passed through. | Never written. | No. | In 1 fixture file, e.g. `RE,0`. |
| PF | Passed through. It is named in `tti.go:4-6`. | Never written. | No. | No fixture uses it. |
| RD, MS | Treated as unknown codes and kept raw. | No. | No. | No fixture uses them. Their meaning is not in this source. |
| Unknown or prefix-less | Kept raw (`Line.Code()` is empty, `tti.go:25-31`). | No. | No. | Covered by the "unknown and blank" test (`tti_test.go:71`). |

**Other facts:**

- Line endings (CRLF, LF, or none on the last line) are preserved. An added
  line copies the terminator of the line next to it (`tti.go:346-366`).
- Output writes each page to the vbit2 directory atomically, as a hidden
  `.<name>.tmp` file renamed over the target
  (`internal/output/output.go:140-145`).
- An upload is refused if the file's mpp differs from the target page's
  (`pages.go:143`).

## 5. Rules for agents writing TTI

- Keep CRLF unless the file already uses LF. Preserve unknown lines and the
  line order, and edit only the lines that change.
- Write control codes below 0x20 as ESC plus (code + 0x40), or as code +
  0x80 if the file already uses that form. Do not mix the two forms in a
  file you create.
- A row is at most 40 cells, counting an ESC pair as one.
- Every PN in a file has the same magazine and page; subpages differ only
  in the last two digits.
- Per-subpage lines (SC, PS, CT, FL, OL) go after their PN. Page-level lines
  (DE, DS, SP) go before the first PN.
- Write `8ff` for "no link" in FL, not `0`.
