# Setting up the high-score pages

This guide sets up high-score games in ceems from the web app, using six
invented games as a worked example. Nothing is seeded: you type each sheet id
into the bindings form yourself. Each game is one binding on its own page.

For each game, the sheet id is the segment after `/d/` in that sheet's URL.
Do not paste an id into anything that is committed.

Every sheet must be shared with the service account whose key is set in
`googleKeyFile`; see `deploy/README.md`.

## 1. Upload the template

In **Templates**, upload a high-score template. The sample one is
`testdata/sample/highscores/template.tti`. Use:

| Field | Value |
|---|---|
| Name | `High scores` |
| Row rules (JSON) | `{"17":"p4name"}` |

The row rule blanks the comma rule on row 17 when there is no fourth entry.

## 2. Create one binding per game

In **Bindings**, choose **New binding** once per game. The example games:

| Page | Name | Range | `c` (three palette picks, in this order) | `gname` |
|---|---|---|---|---|
| 401 | Rocket Rally | C5:E | Alpha blue, New background, Alpha yellow | Rocket Rally |
| 402 | Turbo Track | C4:E | space, space, Alpha green | Turbo Track |
| 403 | Maze Muncher | C4:E | Alpha red, New background, Alpha white | Maze Muncher |
| 404 | Haunted Hall | C4:E | Alpha green, New background, Alpha blue | Haunted Hall |
| 405 | Robo Rumble | C4:E | Alpha blue, New background, Alpha white | Robo Rumble |
| 406 | Castle Quest Deluxe | C4:E | Alpha yellow, New background, Alpha red | Castle Quest Deluxe |

For every binding:

- **Template:** `High scores`.
- **Page:** the game's page.
- **Sheet id:** the id from that game's sheet URL.
- **Range:** the cells holding the entries, from the first entry row down.
- **Columns:** `p#name, p#scr, p#date`
- **Rows:** `8`
- **Formats**, one per line:
  - `p#scr=number` and `p#date=date`;
  - for a game timed in seconds (Rocket Rally here) instead `p#scr=seconds`
    and `p#date=date`.
- **Fixed values:** `c` and `gname`. For `c`, click the palette buttons in the
  order shown, or type a space for "space". `gname` is the game's name as text.

The `c` slot is the colour of the name bar on row 7: background colour, then
"New background", then the text colour. A black bar is two spaces and then the
text colour, which is the Turbo Track row.

## 3. What the formats do

- `number` marks a numeric column. Characters other than digits, `.`, `,` and `-`
  are dropped, then a number too long for its slot is shortened to fit that slot's
  real width: `12,345,670` becomes `12.35m` in a nine-cell slot. Text that is not a
  number is shown as it is. `number unit=sec` adds the unit after a space, shortening
  the number to leave room and cutting the result from the right if it is still too
  long. `seconds` is the same as `number unit=sec`; Rocket Rally uses it.
- `date` writes dates as `12 Oct 25`, from a date cell or from text such as
  `12/10/2025` or `2025-10-12`. `date DD/MM/YY` chooses another form from the tokens
  `D DD M MM Mth YY YYYY`; anything else in the pattern is copied, and a pattern
  longer than its slot is cut from the right. Text it cannot read fails the run, and
  the page is left as it was.

## Notes

- Any page from 100 to 8FF may be used for a game.
- A binding's page can be changed later in the bindings form.
- The name bar always runs to cell 36. To end it earlier for one game, type the
  game name, two spaces and "Black background" into that game's `gname`, for
  example `Turbo Track` + space + space + the "Black background" palette button.
  The cells after that code are black.
- Rows 13, 15 and 16 (the top three) and rows 17 to 22 (places 4 to 8) are blank
  when the sheet has fewer entries.
- "Updated" on row 3 is the time of the run in the server's local time.
