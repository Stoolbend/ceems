<script setup lang="ts">
import { ref } from 'vue'

const el = ref<HTMLDialogElement>()
defineExpose({ open: () => el.value?.showModal() })
</script>

<template>
  <dialog ref="el" class="credits" aria-labelledby="credits-title">
    <fieldset class="dialog">
      <legend><h2 id="credits-title">Credits</h2><button type="button" class="close" aria-label="Close" @click="el?.close()">[X]</button></legend>
      <p>CeeMS<br>Copyright © 2026 Conor Morgan and CeeMS contributors</p>
      <p>CeeMS is free software under the GNU Affero General Public License, version 3 or later. You may redistribute it under the terms of that license. It comes with no warranty. <a href="/LICENSE.txt" target="_blank" rel="noopener">License</a></p>
      <p>Source code: <a href="https://github.com/Stoolbend/ceems" target="_blank" rel="noopener">github.com/Stoolbend/ceems</a></p>
      <p>UI font: <a href="https://int10h.org/oldschool-pc-fonts/" target="_blank" rel="noopener">Web437 IBM VGA 8x16</a> by VileR, <a href="https://creativecommons.org/licenses/by-sa/4.0/" target="_blank" rel="noopener">CC BY-SA 4.0</a>.</p>
      <p>Teletext font: <a href="https://galax.xyz/Teletext50/" target="_blank" rel="noopener">Teletext50</a> by Alan Davies, after <a href="http://bjh21.me.uk/bedstead/" target="_blank" rel="noopener">Bedstead</a> by Ben Harris, public domain.</p>
      <p>Built with <a href="https://vuejs.org/" target="_blank" rel="noopener">Vue</a> (MIT), <a href="https://router.vuejs.org/" target="_blank" rel="noopener">vue-router</a> (MIT), <a href="https://go.dev/" target="_blank" rel="noopener">Go</a> (BSD-3-Clause), <a href="https://sqlite.org/" target="_blank" rel="noopener">SQLite</a> via modernc.org/sqlite (public domain) and the <a href="https://github.com/googleapis/google-api-go-client" target="_blank" rel="noopener">Google API client for Go</a> (BSD-3-Clause).</p>
      <p><a href="/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">Full license texts</a></p>
    </fieldset>
  </dialog>
</template>

<style scoped>
.credits { padding: 0; border: 0; background: none; overflow: visible; }
.credits::backdrop { background: rgba(0, 0, 0, 0.5); }
.credits fieldset { margin: 0.5em 1ch; }
.credits .close { position: absolute; top: 0.5em; right: 2ch; }
</style>
