<script setup lang="ts">
import { user } from './session'
</script>

<template>
  <fieldset class="panel">
    <legend><h1>Help</h1></legend>
    <p>
      Templates and bindings let you keep a page, such as a high-score table, up to date from a Google Sheet. A template is a page with
      named slots in its rows. A binding fills those slots from a range of a sheet every hour and writes the result to a page number.
    </p>
    <ul>
      <li><a href="#overview">How it fits together</a></li>
      <li><a href="#walkthrough">Walkthrough: a top-three scores page</a></li>
      <li><a href="#templates">Templates reference</a></li>
      <li><a href="#bindings">Bindings reference</a></li>
      <li><a href="#errors">When a run fails</a></li>
      <li><a href="#limits">Limits</a></li>
    </ul>

    <h2 id="overview">How it fits together</h2>
    <ul>
      <li>A <strong>template</strong> is a page you draw in the editor, with slot tokens such as <code>{p1name....}</code> where values will go. You download it as a <code>.tti</code> file and upload it on Templates.</li>
      <li>A <strong>binding</strong> joins one template, one range of one Google Sheet and one page number. It says which sheet column fills which slot.</li>
      <li>A <strong>run</strong> reads the range, fills the slots and writes the finished page. A run happens when the server starts, at the top of every hour, and whenever you press Run now.</li>
      <li>A failed run changes nothing: the page keeps its previous content, and the failure is recorded.</li>
      <li>A page with a binding is <strong>read-only</strong> in the editor. It says which binding generates it. To change it, change the template or the binding.</li>
      <li>Preview, on the Pages and Bindings lists, plays a saved page as a TV shows it: the clock runs, subpages rotate and flashing text flashes. Timing for subpages set to cycle is an estimate.</li>
      <li>Templates and Bindings are only for admin accounts.</li>
    </ul>

    <h2 id="walkthrough">Walkthrough: a top-three scores page</h2>
    <p>This builds one working page from scratch. It shows the top three scores from a sheet on page 151.</p>
    <ol>
      <li>
        <strong>Set up Google access, once.</strong>
        <ul>
          <li>Set <code>googleKeyFile</code> in the server config to the path of a Google service-account JSON key, and restart the server.</li>
          <li>Share the sheet with the service account's email address (viewer access is enough).</li>
          <li>Without the key, the Bindings screen shows a banner, no scheduled runs happen, and Run now records a failure.</li>
        </ul>
      </li>
      <li>
        <strong>Make the sheet.</strong> Only the first tab is read. Put a header row in row 1 and the data from row 2: the name in column A and the score in column B. For example:
        <table>
          <thead><tr><th></th><th>A</th><th>B</th></tr></thead>
          <tbody>
            <tr><td>1</td><td>Name</td><td>Score</td></tr>
            <tr><td>2</td><td>Alice</td><td>1200</td></tr>
            <tr><td>3</td><td>Bob</td><td>950</td></tr>
            <tr><td>4</td><td>Cara</td><td>400</td></tr>
          </tbody>
        </table>
      </li>
      <li>
        <strong>Make the template page.</strong> On Pages, type <code>150</code> in New blank page and press Create. In the editor, type these rows:
        <ul>
          <li>row 1: a title, such as <code>TOP THREE</code>;</li>
          <li>row 3: <code>1. {p1name..........} {&gt;p1score.}</code></li>
          <li>row 4: <code>2. {p2name..........} {&gt;p2score.}</code></li>
          <li>row 5: <code>3. {p3name..........} {&gt;p3score.}</code></li>
          <li>row 22: <code>Updated {updated.......}</code></li>
        </ul>
        <p>
          A slot is as wide as its whole token, braces and dots included. <code>{p1name..........}</code> is 18 cells wide, so a name longer than 18
          characters is cut and a shorter one is padded with spaces. <code>{&gt;p1score.}</code> is 11 cells wide, and the <code>&gt;</code> right-aligns the score.
          <code>{updated.......}</code> is 16 cells wide.
        </p>
        <p>
          In the editor's preview the <code>{</code> and <code>}</code> characters show as <code>¼</code> and <code>¾</code>, because that is how British teletext draws those codes.
          The template still works. Tick Show template slots, next to Show control codes, to draw the slots as braces and highlight each one.
        </p>
      </li>
      <li><strong>Download</strong> the page: use the "Download P150.tti" link under the editor.</li>
      <li>
        <strong>Upload it on Templates.</strong>
        <ul>
          <li>Type a name, for example <code>Top three</code>, choose the file, and leave Row rules as <code>{}</code>. Press Upload.</li>
          <li>A Slots table appears. It should list <code>p1name</code> (width 18), <code>p1score</code> (width 11, align right), the same for <code>p2</code> and <code>p3</code>, and <code>updated</code> (width 16), each with its row and column.</li>
        </ul>
      </li>
      <li>
        <strong>Create the binding.</strong> On Bindings choose New binding and fill in:
        <table>
          <tbody>
            <tr><td>Name</td><td><code>Top three</code></td></tr>
            <tr><td>Template</td><td>Top three</td></tr>
            <tr><td>Page</td><td><code>151</code>. The binding creates this page, or takes over an existing page that has no binding.</td></tr>
            <tr><td>Sheet id</td><td>The long id in the sheet's address, between <code>/d/</code> and <code>/edit</code></td></tr>
            <tr><td>Range</td><td><code>A2:B</code></td></tr>
            <tr><td>Columns</td><td><code>p#name, p#score</code></td></tr>
            <tr><td>Rows</td><td><code>3</code></td></tr>
            <tr><td>Formats</td><td>Two lines, press Enter between them:<br /><code>p#score=number unit=sec decimals=3</code><br /><code>p#name=text</code></td></tr>
          </tbody>
        </table>
        <p>
          In Columns, <code>#</code> stands for the entry number. With Rows set to 3, <code>p#name</code> fills <code>p1name</code>, <code>p2name</code> and <code>p3name</code>
          from column A, and <code>p#score</code> fills <code>p1score</code> to <code>p3score</code> from column B. The slot <code>updated</code> fills itself with the time of the run.
          Under Fixed values the form shows "filled from the sheet: ..." and lists those slots. Nothing is left to fill in here, so there are no fixed values to type.
        </p>
      </li>
      <li>
        <strong>Run now.</strong> Press Run now on the binding's row. The result ("ok" or "failed: ...") shows next to the button, and Last run updates.
        Click the binding's name to list its runs. Open page 151 to see the result. It is read-only and says which binding generates it.
      </li>
      <li><strong>From then on,</strong> the page refreshes when the server starts and at the top of every hour. Edit the sheet and the page follows at the next run, or at once if you press Run now.</li>
    </ol>

    <h2 id="templates">Templates reference</h2>
    <h3>Slots</h3>
    <p>A slot is written <code>{name}</code>, <code>{name....}</code>, <code>{^name...}</code> or <code>{&gt;name...}</code>.</p>
    <ul>
      <li>The name starts with a lower-case letter <code>a</code> to <code>z</code>, then lower-case letters, digits, <code>_</code> or <code>-</code>.</li>
      <li>These are slots: <code>{a}</code>, <code>{p1name....}</code>, <code>{best-score}</code>, <code>{&gt;score.}</code>.</li>
      <li>These are not: <code>{1x}</code> (starts with a digit), <code>{A}</code> (upper case), <code>{_x}</code> (starts with an underscore), <code>{}</code> (no name).</li>
      <li>The width is the whole token's length in cells, braces, alignment mark and dots included. The smallest slot is <code>{a}</code>, which is 3 wide. Add dots to make a slot wider.</li>
      <li><code>^</code> centres the value in the slot (an odd spare cell goes on the right). <code>&gt;</code> right-aligns it. With neither, the value is left-aligned.</li>
      <li>Only rows 1 to 24 are scanned. Row 0, the header row, is never a slot.</li>
      <li>The same name may appear more than once, even on other subpages. Each place gets the same value, cut to that place's width.</li>
      <li>Cells outside the slots are never changed.</li>
    </ul>
    <h3>The updated slot</h3>
    <p>
      <code>updated</code> is reserved. It is filled with the time of the run, written like <code>12 Oct 14:00</code> (12 cells), in the server's local time.
      Give it a width of at least 12. You cannot map a sheet column to it or give it a fixed value.
    </p>
    <h3>How values are cleaned, cut and padded</h3>
    <ul>
      <li>Text from the sheet is cleaned first: <code>£</code> becomes <code>#</code>, any other character outside plain ASCII becomes <code>?</code>, and control characters are dropped.</li>
      <li>A value longer than its slot is cut to the slot's width. A shorter one is padded with spaces, on the right, on the left (<code>&gt;</code>) or on both sides (<code>^</code>).</li>
      <li>A slot whose value is empty becomes spaces. If every slot in a row is empty, the whole row is blanked, including any text you typed around the slots.</li>
    </ul>
    <h3>Row rules</h3>
    <p>
      Row rules are a JSON object that maps a row number to a slot name. The row is blanked exactly when that slot is empty, whatever the other slots hold.
      Without a rule, a row is blanked only when all its slots are empty. Use a rule when a row also holds a slot that is never empty, such as a fixed value, or when one slot should decide for the whole row. For example:
    </p>
    <pre><code>{"17": "p4name"}</code></pre>
    <p>This blanks row 17 when <code>p4name</code> is empty, for instance when the sheet has fewer than four entries. Keys are <code>"1"</code> to <code>"24"</code> with no leading zero, and the slot must exist in the template. Leave the box as <code>{}</code> if you need no rules.</p>
    <h3>The upload form</h3>
    <ul>
      <li><strong>Name:</strong> one line of up to 100 characters, unique among templates ("a template with that name already exists").</li>
      <li><strong>Template file:</strong> a <code>.tti</code> file you downloaded from the editor.</li>
      <li><strong>Row rules (JSON):</strong> see above. Invalid JSON gives "Row rules must be a JSON object". A bad key or an unknown slot is refused with a message naming the rule.</li>
      <li>After a good upload the Slots table lists each slot with its subpage, row, column (counted from 0), width and alignment. "No slots found." means the template has no valid slot token, so check the spelling.</li>
    </ul>
    <h3>Changing a template</h3>
    <p>
      Press <strong>Replace</strong> on the template's row. The form fills with its name and row rules, and lists the bindings that use it. Choose a new file, or leave the file empty
      to change only the name or the row rules. Nothing runs by itself: the bound pages change at the next hourly run, or when you press Run now on the binding.
    </p>
    <p>
      The replace is refused (409) when a binding that uses the template would be left with a slot that neither its sheet columns nor its fixed values fill. The message names each
      binding, its page and the slot. Open the binding, add the fixed value, then replace again. Removing a slot is always allowed. After a replace the page shows the slots added and
      removed, and any binding that still holds fixed values for a slot the template no longer has. Those values are ignored; open the binding and save it to drop them.
    </p>

    <h2 id="bindings">Bindings reference</h2>
    <h3>The form</h3>
    <table>
      <thead><tr><th>Field</th><th>Rule</th></tr></thead>
      <tbody>
        <tr><td>Name</td><td>One line of 1 to 100 characters, unique among bindings.</td></tr>
        <tr><td>Template</td><td>One of the uploaded templates. Choosing it loads its slots into the Fixed values section.</td></tr>
        <tr><td>Page</td><td>Three hex digits from 100 to 8FF. A page with no binding is taken over, and its content is replaced at the first run. A missing page is created. A page that already has a binding is refused.</td></tr>
        <tr><td>Sheet id</td><td>The id from the sheet's address. It is shown only in this form, never in the list.</td></tr>
        <tr><td>Range</td><td>Cells in A1 notation, such as <code>A2:B</code> or <code>C5:E30</code>, with no tab name. Only the sheet's first tab is read.</td></tr>
        <tr><td>Columns</td><td>One slot name per range column, separated by commas, in order. <code>#</code> stands for the entry number, 1, 2, 3 and so on. The range must be at least as wide as the list.</td></tr>
        <tr><td>Rows</td><td>How many entries to use, from 1 to 24.</td></tr>
        <tr><td>Formats</td><td>One line per column, as <code>column=formatter</code>. Press Enter between entries: a comma does not separate them, and a line such as <code>p#score=number unit=sec, p#date=date</code> is refused. For example, the two lines <code>p#score=number unit=sec decimals=3</code> and <code>p#date=date</code>. A column with no line uses <code>text</code>.</td></tr>
      </tbody>
    </table>
    <h3>Fixed values</h3>
    <p>
      Every slot in the template that the sheet does not fill needs a fixed value. The form lists them, each with its width, and a counter such as 3/9.
      The line "filled from the sheet: ..." names the slots the columns cover. You cannot give a fixed value to a slot the sheet fills ("not a fixed slot").
    </p>
    <ul>
      <li>The input stops at the slot's width.</li>
      <li>Buttons above the inputs insert teletext control codes, such as a colour change.</li>
      <li>Only teletext characters are allowed. Anything else is marked invalid and Save is disabled.</li>
      <li>If you change Columns or Rows so that a slot becomes sheet-filled, its fixed value is dropped, with a warning.</li>
    </ul>
    <h3>How rows are picked from the range</h3>
    <ul>
      <li>A row counts when every configured column in it has text.</li>
      <li>Incomplete rows at the top are skipped, which lets a header row sit inside the range.</li>
      <li>The first incomplete row after data ends the list. A blank row in the sheet stops it there.</li>
      <li>At most Rows entries are used. Slots with no entry are empty, so their rows blank.</li>
    </ul>
    <h3>Formatters</h3>
    <table>
      <thead><tr><th>Name</th><th>Does</th></tr></thead>
      <tbody>
        <tr><td><code>text</code></td><td>The cell's text as the sheet shows it. This is the default.</td></tr>
        <tr><td><code>number</code></td><td>Marks a numeric column. Characters other than digits, <code>.</code>, <code>,</code> and <code>-</code> are dropped (<code>$1,234</code> becomes <code>1,234</code>). A number too long for its slot is shortened with <code>k</code>, <code>m</code> or <code>b</code>: <code>1234567890</code> becomes <code>1.23b</code>. The width is the real width of each slot in the template. A cell that is not a number is shown as it is.</td></tr>
        <tr><td><code>number unit=sec</code></td><td>As <code>number</code>, then adds the unit after a space: <code>95</code> becomes <code>95 sec</code>. The number is shortened to leave room for the unit, and if the result is still too long it is cut from the right. The unit is up to you, for example <code>unit=pts</code>. A cell that is not a number gets no unit.</td></tr>
        <tr><td><code>number decimals=3</code></td><td>Rounds to exactly that many places (0 to 6): <code>2.9</code> becomes <code>2.900</code>. Thousands commas stay if the sheet had them. If the result is too long for the slot, places are dropped one at a time before <code>k</code>, <code>m</code> or <code>b</code> is used. Without <code>decimals</code>, the sheet's text is kept when it fits, and a number under 1000 that does not fit keeps as many places as fit. Options go in any order, separated by spaces: <code>number unit=sec decimals=3</code>. A unit has no space, comma or <code>=</code>.</td></tr>
        <tr><td><code>seconds</code></td><td>Same as <code>number unit=sec</code>.</td></tr>
        <tr><td><code>date</code></td><td>Writes the date as <code>12 Oct 25</code>. Add a pattern to choose another form, for example <code>date DD/MM/YY</code> gives <code>12/10/25</code>. Pattern tokens: <code>D</code> (9), <code>DD</code> (09), <code>M</code> (1), <code>MM</code> (01), <code>Mth</code> (Jan), <code>YY</code> (25), <code>YYYY</code> (2025); anything else is copied. A pattern longer than its slot is cut from the right. It reads a real date cell, or text such as <code>12/10/2025</code>, <code>2025-10-12</code> or <code>12 Oct 2025</code>. In numeric text the day comes first, unless the second number is over 12, which is then read as the day. Text it cannot read fails the run.</td></tr>
      </tbody>
    </table>
    <h3>Editing, moving and deleting</h3>
    <ul>
      <li><strong>Edit</strong> opens the form with the saved values. Change Page to move the binding and its page to another number. A move onto a page that already exists is refused.</li>
      <li><strong>Copy</strong> opens a new binding form holding every setting of this one except Page, which is empty. The name is the old name plus <code>copy</code>. Type a page and save.</li>
      <li><strong>Delete</strong> removes the binding only. The page keeps its last content and becomes editable.</li>
    </ul>
    <h3>The schedule</h3>
    <p>
      Every binding runs when the server starts and at the top of each hour, one after another in the order they were created. There is one schedule for all of them.
      Run now runs one binding straight away and waits for it to finish. A run that takes more than 60 seconds is stopped and fails.
    </p>
    <h3>The runs table</h3>
    <p>
      Click a binding's name to list its runs, newest first: start, finish, who started it (<code>schedule</code>, or <code>user:</code> and the account's number for Run now), the outcome and any error. The last 100 runs of each binding are kept.
      Run times are in UTC. The <code>updated</code> slot on the page uses the server's local time, so the two can differ by an hour in summer.
    </p>

    <h2 id="errors">When a run fails</h2>
    <p>The error shows beside Run now, in the Last run column and in the runs table. The page keeps its previous content.</p>
    <table>
      <thead><tr><th>Message</th><th>Likely cause</th><th>Fix</th></tr></thead>
      <tbody>
        <tr><td>googleKeyFile is not configured</td><td>The server has no Google key.</td><td>Set <code>googleKeyFile</code> in the server config and restart.</td></tr>
        <tr><td>sheet not found or not shared with the service account</td><td>The sheet id is wrong, or the sheet is not shared with the service account.</td><td>Check the id, and share the sheet with the service account's email address.</td></tr>
        <tr><td>sheet unreachable: ...</td><td>A network problem, a Google error or a timeout.</td><td>Read the rest of the message and try Run now again later.</td></tr>
        <tr><td>range has no values</td><td>The range is empty, or it is not on the sheet's first tab.</td><td>Check the range and that the data is on the first tab.</td></tr>
        <tr><td>no row has every configured column: ...</td><td>Columns lists more columns than the range is wide, or every row has an empty cell in one of them.</td><td>Widen the range or shorten Columns, and fill the cells.</td></tr>
        <tr><td>no value for slot "..." (subpage ... row ...)</td><td>The template has a slot that neither Columns nor Fixed values covers. The form normally prevents this.</td><td>Edit the binding, check the Fixed values, and save.</td></tr>
        <tr><td>a run is in progress; try again shortly</td><td>The hourly run, or another Run now, is still going.</td><td>Wait a few seconds and press Run now again.</td></tr>
        <tr><td>date: unsupported value "..."</td><td>A cell in a <code>date</code> column is not a date, or is not in a form the formatter reads.</td><td>Correct the cell, or format the column as a date in the sheet.</td></tr>
        <tr><td>source: format "...": number: unknown option ...</td><td>A <code>number</code> Formats line has a word that is not <code>unit=</code> or <code>decimals=</code>.</td><td>Write <code>number</code>, <code>number unit=sec</code> or <code>number unit=sec decimals=3</code>.</td></tr>
        <tr><td>source: format "...": number: unit "..." must be printable text with no space, ',' or '='</td><td>Two columns were put on one line, such as <code>p#score=number unit=sec, p#date=date</code>.</td><td>Put each column on its own line.</td></tr>
        <tr><td>source: format "...": number: decimals must be a whole number from 0 to 6</td><td>The <code>decimals=</code> value is not 0 to 6.</td><td>Write for example <code>decimals=3</code>.</td></tr>
        <tr><td>source: format "...": number: option ... is given twice</td><td><code>unit=</code> or <code>decimals=</code> appears twice on the line.</td><td>Give each option once.</td></tr>
        <tr><td>source: format "...": date: pattern has no date token ...</td><td>A <code>date</code> pattern has none of <code>D DD M MM Mth YY YYYY</code>. Tokens are case sensitive.</td><td>Write for example <code>date DD/MM/YY</code>.</td></tr>
      </tbody>
    </table>
    <p>
      The form can also refuse a save. The usual messages are "page 151 already has a binding", "a binding with that name already exists",
      "slot ... needs a value", "slot ... is not a fixed slot" and "Formats line 1 must be column=formatter" and "Formats line 1: put each column on its own line".
    </p>

    <template v-if="user?.admin">
      <h2 id="fastext">Fastext groups</h2>
      <ul>
        <li>A fastext group sets the four coloured links (Red, Green, Yellow, Cyan) of its pages. Make groups on the Fastext screen. A page is in at most one group, and a page in a group shows its links as read-only in the editor, with a notice under them that links to the group. Link 5 is always yours to set.</li>
        <li>A <strong>static</strong> group gives every page the same four links.</li>
        <li>A <strong>paged</strong> group also links each page to its neighbours. Choose which key is Prev and which is Next (never the same key). Those keys point at the previous and next page in the member order; the other two keys are the same for every page.</li>
        <li>The member order is the order on the Fastext screen, set with Up and Down. The first page has no Previous link and the last has no Next link; a missing link is <code>8ff</code>.</li>
        <li>The <strong>default</strong> group takes new pages. Only one group is the default.</li>
        <li>Each coloured link has a text of up to 9 cells, and the group writes row 24 of its pages from the four texts: each is the colour code and then the text. A paged group's Prev and Next keys show <code> [ Prev  </code> and <code>  Next ] </code>, blank on the first and last page. The Fastext screen previews row 24. In a text box, Esc then a key inserts a control code with the page editor's keys (Esc r for red, Esc 1 for alpha red, Esc Shift+1 for mosaic red).</li>
        <li>A page in a group shows row 24 read-only in the editor.</li>
        <li>Taking a page out of a group, or deleting a group, leaves the page's links as they are and blanks its row 24.</li>
        <li>On Settings, <strong>Index link</strong> makes every page's sixth link the Index page. Turning it on or changing the page rewrites every page.</li>
      </ul>

      <h2 id="settings">Settings</h2>
      <ul>
        <li>The pages directory is set in the config file (<code>pagesDir</code>). It is shown on Settings but cannot be changed there; restart CeeMS after editing the file.</li>
        <li>With <strong>Write vbit.conf</strong> on, CeeMS writes a <code>vbit.conf</code> beside the pages. With it off, CeeMS never writes or deletes that file, so the last one written stays until you remove it.</li>
        <li>The file is an override for vbit2, which reads it only when it starts. Restart vbit2 after a change.</li>
        <li>The header template is exactly 32 cells. The page editor's preview row uses it too.</li>
      </ul>
    </template>

    <h2 id="limits">Limits</h2>
    <ul>
      <li>Only a sheet's first tab is read.</li>
      <li>There is no per-binding schedule. Every binding runs at start-up and hourly.</li>
      <li>Replacing a template does not run its bindings. The pages change at the next hourly run or on Run now.</li>
      <li>The Templates and Bindings screens need an admin account.</li>
    </ul>
  </fieldset>
</template>

<style scoped>
/* the theme draws no row rules, so wrapped cells in the error and field tables run together */
tbody tr + tr td { border-top: 1px solid var(--frame); }
</style>
