<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Preview } from '../api'
import TeletextPage from './TeletextPage.vue'
import { DEFAULT_HEADER, headerRow, vbitHeader } from './header'

// Plays a preview as a TV shows it. It fetches nothing and has no controls.
const props = defineProps<{ preview: Preview }>()

const index = ref(0)
const now = ref(new Date())
let rotate: ReturnType<typeof setTimeout> | undefined

function schedule() {
  clearTimeout(rotate)
  const n = props.preview.subpages.length
  if (n < 2) return
  rotate = setTimeout(() => { index.value = (index.value + 1) % n; schedule() }, Math.max(1, props.preview.subpages[index.value].seconds) * 1000)
}

watch(() => props.preview, () => { index.value = 0; schedule() }, { immediate: true })
const clock = setInterval(() => { now.value = new Date() }, 1000)
onBeforeUnmount(() => { clearTimeout(rotate); clearInterval(clock) })

const rows = computed(() => {
  const { mpp, headerTemplate, subpages } = props.preview
  const out = (subpages[index.value]?.rows ?? []).map((r) => r ?? '')
  while (out.length < 25) out.push('')
  out[0] = headerTemplate === '' ? headerRow(DEFAULT_HEADER, mpp, now.value) : vbitHeader(headerTemplate, mpp, now.value)
  return out
})
</script>

<template>
  <TeletextPage :rows="rows" />
</template>
