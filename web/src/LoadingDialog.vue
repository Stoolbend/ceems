<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { pending } from './api'

// A request that finishes inside this time shows nothing.
const DELAY_MS = 300

const el = ref<HTMLDialogElement>()
let timer: ReturnType<typeof setTimeout> | undefined

function stop() { clearTimeout(timer); timer = undefined }

watch(pending, (n) => {
  if (n > 0) {
    timer ??= setTimeout(() => { if (!el.value?.open) el.value?.showModal() }, DELAY_MS)
  } else {
    stop()
    if (el.value?.open) el.value.close()
  }
})
onBeforeUnmount(stop)
</script>

<template>
  <dialog ref="el" class="loading" aria-label="Loading" @cancel.prevent>
    <fieldset class="dialog">
      <p role="status" aria-live="polite">Loading…</p>
      <p class="bar" aria-hidden="true"><span class="block">████</span>░░░░░░░░░░░░░░░░░░░░</p>
    </fieldset>
  </dialog>
</template>

<style scoped>
.loading { padding: 0; border: 0; background: none; overflow: visible; }
.loading::backdrop { background: rgba(0, 0, 0, 0.5); }
.loading fieldset { margin: 0.5em 1ch; }
.loading p { margin: 0; }
/* the track is 20 cells wide; the block steps along it and back */
.bar { position: relative; width: 20ch; white-space: nowrap; }
.block { position: absolute; left: 0; animation: slide 1.6s steps(17, jump-none) infinite alternate; }
@keyframes slide { to { transform: translateX(16ch); } }
@media (prefers-reduced-motion: reduce) { .block { animation: none; } }
</style>
