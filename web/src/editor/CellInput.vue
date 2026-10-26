<script setup lang="ts">
import { ref, watch } from 'vue'
import { fromEditable, PALETTE, toEditable } from './cells'

// A one-line box for teletext cells: control codes show as Control Pictures and are typed with Esc and the
// page editor's key (Esc r for red, Esc 1 for alpha red, Esc Shift+1 for mosaic red).
const props = defineProps<{ modelValue: string; maxlength: number; disabled?: boolean; label: string }>()
const emit = defineEmits<{ 'update:modelValue': [cells: string]; invalid: [bad: boolean] }>()

const input = ref<HTMLInputElement>()
const text = ref(toEditable(props.modelValue))
const bad = ref(false)
let pending = false

// follow the parent unless it only echoes what is typed
watch(() => props.modelValue, (v) => {
  if (fromEditable(text.value) !== v) {
    text.value = toEditable(v)
    bad.value = false
    emit('invalid', false)
  }
})

function edited() {
  const cells = fromEditable(text.value)
  bad.value = cells instanceof Error
  emit('invalid', bad.value)
  if (typeof cells === 'string') emit('update:modelValue', cells)
}

function insert(char: string) {
  const el = input.value
  if (!el || (el.value.length >= props.maxlength && el.selectionStart === el.selectionEnd)) return
  el.setRangeText(char, el.selectionStart ?? el.value.length, el.selectionEnd ?? el.value.length, 'end')
  el.dispatchEvent(new Event('input')) // keeps v-model in step
}

// The page editor's pending key (edit.ts): Digit1-7, with Shift for mosaic, or a palette key.
function keydown(e: KeyboardEvent) {
  if (!pending) {
    if (e.key === 'Escape') {
      e.preventDefault()
      pending = true
    }
    return
  }
  if (['Shift', 'Control', 'Alt', 'AltGraph', 'Meta', 'CapsLock'].includes(e.key)) return
  pending = false
  e.preventDefault()
  const digit = /^Digit([1-7])$/.exec(e.code ?? '')
  const code = digit ? Number(digit[1]) + (e.shiftKey ? 0x10 : 0) : PALETTE.find((p) => p.key === e.key)?.code
  if (code !== undefined) insert(toEditable(String.fromCharCode(code)))
}
</script>

<template>
  <input
    ref="input" v-model="text" type="text" :maxlength="maxlength" :disabled="disabled" :aria-label="label" spellcheck="false" autocomplete="off"
    :aria-invalid="bad" @input="edited" @keydown="keydown" @blur="pending = false"
  />
</template>
