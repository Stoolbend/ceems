<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { getSettings, putSettings, type Settings } from './api'
import { PALETTE, fromEditable, toEditable } from './editor/cells'
import { systemName as siteName, user } from './session'
import { vbitHeader } from './teletext/header'
import TeletextPage from './teletext/TeletextPage.vue'

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// Magazine 8 comes first in vbit2's list, then 1 to 7.
const MAGAZINES = [8, 1, 2, 3, 4, 5, 6, 7]

const CODES = [
  ['%%#', 'page'], ['%%a', 'day name'], ['%%b', 'month name'], ['%d', 'day'], ['%e', 'day, space padded'],
  ['%m', 'month'], ['%y', 'year'], ['%H', 'hour'], ['%M', 'minute'], ['%S', 'second'],
]

const loaded = ref<Settings | null>(null)
const error = ref('')
const saved = ref(false)
const systemName = ref('')
const vbitConf = ref(false)
const header = ref('')
const initialPage = ref('')
const rowAdaptive = ref(false)
const lines = ref(16)
const mags = ref<number[]>(Array(8).fill(9))
const statusDisplay = ref('')
const indexEnabled = ref(false)
const indexPage = ref('')
const headerInput = ref<HTMLInputElement>()

function fill(s: Settings) {
  loaded.value = s
  systemName.value = s.systemName
  vbitConf.value = s.vbitConf
  header.value = toEditable(s.headerTemplate)
  initialPage.value = s.initialTeletextPage
  rowAdaptive.value = s.rowAdaptiveMode
  lines.value = s.linesPerField
  mags.value = s.magazinePriority.split(',').map(Number)
  statusDisplay.value = s.statusDisplay
  indexEnabled.value = s.indexPageEnabled
  indexPage.value = s.indexPage
}

// The preview shows the live clock, as vbit2 would draw it.
const now = ref(new Date())
let tick: ReturnType<typeof setInterval>
onMounted(async () => {
  tick = setInterval(() => (now.value = new Date()), 1000)
  try {
    fill(await getSettings())
  } catch (e) {
    error.value = message(e)
  }
})
onUnmounted(() => clearInterval(tick))

const cells = computed(() => fromEditable(header.value))
const headerBad = computed(() => cells.value instanceof Error)
const preview = computed(() => (typeof cells.value === 'string' ? vbitHeader(cells.value, '100', now.value) : ''))

function insertCode(char: string) {
  const el = headerInput.value
  if (!el || (el.value.length >= el.maxLength && el.selectionStart === el.selectionEnd)) return
  el.setRangeText(char, el.selectionStart ?? el.value.length, el.selectionEnd ?? el.value.length, 'end')
  el.dispatchEvent(new Event('input')) // keeps v-model and the counter in step
  el.focus()
}

async function save() {
  if (typeof cells.value !== 'string' || !loaded.value) return
  error.value = ''
  saved.value = false
  try {
    const s = await putSettings({
      systemName: systemName.value, pagesDir: loaded.value.pagesDir, vbitConf: vbitConf.value, headerTemplate: cells.value,
      initialTeletextPage: initialPage.value, rowAdaptiveMode: rowAdaptive.value, linesPerField: Number(lines.value),
      magazinePriority: mags.value.join(','), statusDisplay: statusDisplay.value,
      indexPageEnabled: indexEnabled.value, indexPage: indexPage.value,
    })
    fill(s)
    saved.value = true
    siteName.value = s.systemName
    if (user.value) user.value = { ...user.value, headerTemplate: s.headerTemplate, indexPage: s.indexPageEnabled ? s.indexPage : null }
  } catch (e) {
    error.value = message(e)
  }
}
</script>

<template>
  <fieldset class="panel">
    <legend><h1>Settings</h1></legend>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-if="saved" role="status">Saved</p>
    <form v-if="loaded" @submit.prevent="save">
      <p><label>System name <input v-model="systemName" name="systemName" maxlength="40" required autocomplete="off" /></label></p>
      <p>
        <label><input v-model="indexEnabled" name="indexPageEnabled" type="checkbox" /> Index link</label>
      </p>
      <p>
        <label>Index page <input v-model="indexPage" name="indexPage" pattern="[1-8][0-9A-Fa-f]{2}" required size="8" autocomplete="off" /></label>
        <small>When on, every page's index link (the sixth) is this page. Turning it on or changing the page rewrites every page.</small>
      </p>
      <p>
        <label>Pages directory <input :value="loaded.pagesDir" name="pagesDir" disabled /></label>
        <small>Set in the config file (pagesDir). Restart CeeMS to change it.</small>
      </p>
      <p>
        <label><input v-model="vbitConf" name="vbitConf" type="checkbox" /> Write vbit.conf</label>
        <small v-if="!vbitConf">CeeMS will not write or delete vbit.conf; the last file written stays until you remove it.</small>
      </p>
      <fieldset v-if="vbitConf">
        <legend>vbit2 settings</legend>
        <div class="palette">
          <button v-for="p in PALETTE" :key="p.code" type="button" :title="p.name" @click="insertCode(p.char)">{{ p.char }} {{ p.name }}</button>
        </div>
        <p>
          <label>Header template
            <input
              ref="headerInput" v-model="header" name="headerTemplate" type="text" maxlength="32" spellcheck="false"
              :aria-invalid="headerBad" :class="{ bad: headerBad }"
            />
          </label>
          <span class="count">{{ header.length }}/32</span>
        </p>
        <TeletextPage :rows="[preview]" :row-count="1" aria-label="Header preview" />
        <ul class="codes">
          <li v-for="[code, what] in CODES" :key="code"><code>{{ code }}</code> {{ what }}</li>
        </ul>
        <p><label>Initial page <input v-model="initialPage" name="initialTeletextPage" pattern="[1-8][0-9A-Fa-f]{2}(:[0-9A-Fa-f]{4})?" required size="8" /></label></p>
        <p><label><input v-model="rowAdaptive" name="rowAdaptiveMode" type="checkbox" /> Row adaptive mode</label></p>
        <p><label>Lines per field <input v-model.number="lines" name="linesPerField" type="number" min="1" max="32" required /></label></p>
        <p class="mags">
          Magazine priority
          <label v-for="(m, i) in MAGAZINES" :key="m">{{ m }} <input v-model.number="mags[i]" :name="`mag${m}`" type="number" min="1" max="9" required /></label>
        </p>
        <p><label>Status display <input v-model="statusDisplay" name="statusDisplay" maxlength="20" pattern="[!-:<-~]*" required autocomplete="off" /> <small>no spaces or ;</small></label></p>
        <p><small>vbit2 reads vbit.conf when it starts. Restart vbit2 after a change.</small></p>
      </fieldset>
      <p><button type="submit" :disabled="headerBad">Save</button></p>
    </form>
  </fieldset>
</template>

<style scoped>
.bad { border-color: var(--pico-del-color, red); }
.mags input { width: 4rem; }
.codes { columns: 2; }
.palette button { margin: 0 0.25rem 0.25rem 0; }
</style>
