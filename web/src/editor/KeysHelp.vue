<script setup lang="ts">
import { PALETTE } from './cells'
</script>

<template>
  <details class="keys">
    <summary>Keys</summary>
    <table>
      <tr v-for="p in PALETTE" :key="p.code">
        <td>{{ p.name }}</td>
        <td><kbd>Esc</kbd> <kbd>{{ p.key }}</kbd><template v-if="p.code >= 1 && p.code <= 7"> or <kbd>Esc</kbd> <kbd>{{ p.code }}</kbd></template><template v-if="p.code >= 0x11 && p.code <= 0x17"> or <kbd>Esc</kbd> <kbd>Shift+{{ p.code - 0x10 }}</kbd></template></td>
      </tr>
    </table>
    <ul>
      <li>Arrow keys move the cursor; Home and End go to the start and end of the row; Ctrl+Home and Ctrl+End go to the first and last row.</li>
      <li>Enter moves to the next row; Backspace and Delete remove a cell; Insert toggles insert mode.</li>
      <li>In a mosaic cell, q w a s z x toggle one sixel; r inverts, f fills and c clears.</li>
      <li>With Draw on, press and drag over mosaic cells to paint sixels. The first sixel pressed decides whether the drag sets or clears.</li>
      <li>Ctrl+Z undoes; Ctrl+Y or Ctrl+Shift+Z redoes.</li>
      <li>Shift with an arrow key, or a drag with Draw off, selects a block. Esc, a click or any edit clears it.</li>
      <li>Ctrl+C or Ctrl+Insert copies the block; Ctrl+X cuts it, leaving spaces.</li>
      <li>Ctrl+V or Shift+Insert pastes a copied block at the cursor, a zxnet editor URL (the whole subpage) or text (at the cursor).</li>
      <li>Esc, then a key, enters a control code.</li>
    </ul>
  </details>
</template>

<style scoped>
.keys { margin: 0.25rem 0; }
.keys table { width: auto; }
</style>
