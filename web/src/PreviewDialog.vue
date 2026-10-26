<script setup lang="ts">
import { ref } from 'vue'
import { getPreview, type Preview } from './api'
import TeletextPlayer from './teletext/TeletextPlayer.vue'

const el = ref<HTMLDialogElement>()
const mpp = ref('')
const preview = ref<Preview | null>(null)
const error = ref('')

async function open(m: string) {
  mpp.value = m
  preview.value = null
  error.value = ''
  el.value?.showModal()
  try {
    preview.value = await getPreview(m)
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
}
// Closing by Close or Escape drops the preview, which unmounts the player and its timers.
function closed() { preview.value = null }
defineExpose({ open })
</script>

<template>
  <dialog ref="el" class="preview-dialog" aria-labelledby="preview-title" @close="closed">
    <fieldset class="dialog">
      <legend><h2 id="preview-title">Preview {{ mpp }}<template v-if="preview"> {{ preview.description }}</template></h2><button type="button" class="close" aria-label="Close" @click="el?.close()">[X]</button></legend>
      <p v-if="error" role="alert">{{ error }}</p>
      <TeletextPlayer v-if="preview" :preview="preview" />
    </fieldset>
  </dialog>
</template>

<style scoped>
.preview-dialog { padding: 0; border: 0; background: none; overflow: visible; width: min(90vw, 960px); }
.preview-dialog::backdrop { background: rgba(0, 0, 0, 0.5); }
.preview-dialog fieldset { margin: 0.5em 1ch; }
.preview-dialog .close { position: absolute; top: 0.5em; right: 2ch; }
</style>
