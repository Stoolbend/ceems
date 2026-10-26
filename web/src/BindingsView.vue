<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  ApiError, createBinding, dataKeys, deleteBinding, getBinding, getTemplate, listBindings, listRuns, listTemplates,
  runBinding, updateBinding, type BindingSummary, type Run, type Template, type TemplateSummary,
} from './api'
import { PALETTE, fromEditable, toEditable } from './editor/cells'
import PreviewDialog from './PreviewDialog.vue'
import { localTime } from './time'

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// The list. It never loads a single binding, so the sheet id stays out of it.
const preview = ref<InstanceType<typeof PreviewDialog>>()
const bindings = ref<BindingSummary[]>([])
const sheetsConfigured = ref(true)
const error = ref('')
const running = ref<Record<number, boolean>>({})
const runResult = ref<Record<number, string>>({})
const selected = ref<BindingSummary | null>(null)
const runs = ref<Run[]>([])
const templates = ref<TemplateSummary[]>([])

async function load() {
  try {
    const l = await listBindings()
    bindings.value = l.bindings
    sheetsConfigured.value = l.sheetsConfigured
  } catch (e) {
    error.value = message(e)
  }
}

onMounted(async () => {
  await load()
  try {
    templates.value = await listTemplates()
  } catch (e) {
    error.value = message(e)
  }
})

async function select(b: BindingSummary) {
  selected.value = b
  try {
    runs.value = await listRuns(b.id)
  } catch (e) {
    error.value = message(e)
  }
}

async function runNow(b: BindingSummary) {
  running.value[b.id] = true
  runResult.value[b.id] = ''
  try {
    const run = await runBinding(b.id)
    runResult.value[b.id] = run.outcome === 'ok' ? 'ok' : `failed: ${run.error}`
  } catch (e) {
    runResult.value[b.id] = e instanceof ApiError && e.status === 409 ? 'a run is in progress; try again shortly' : message(e)
  } finally {
    running.value[b.id] = false
  }
  await load()
  if (selected.value?.id === b.id) await select(b)
}

async function remove(b: BindingSummary) {
  if (!window.confirm(`Delete binding ${b.name}? Page ${b.mpp} keeps its content and becomes editable.`)) return
  error.value = ''
  try {
    await deleteBinding(b.id)
  } catch (e) {
    error.value = message(e)
    return
  }
  if (selected.value?.id === b.id) selected.value = null
  await load()
}

// The form. editing is the binding id, or null when creating.
const showForm = ref(false)
const editing = ref<number | null>(null)
const formError = ref('')
const name = ref('')
const templateId = ref<number | null>(null)
const template = ref<Template | null>(null)
const mpp = ref('')
const sheet = ref('')
const range = ref('')
const columns = ref('')
const rows = ref(1)
const formats = ref('')
const values = ref<Record<string, string>>({}) // fixed slot name -> editable text
const dropped = ref<string[]>([])
const staticInputs = ref<HTMLInputElement[]>([])
const lastSlot = ref('')

const columnList = computed(() => columns.value.split(',').map((c) => c.trim()).filter((c) => c !== ''))
const keys = computed(() => dataKeys(columnList.value, Number(rows.value)))

// Distinct template slots: those the sheet fills, and the fixed ones with the width of their first occurrence.
const filled = computed(() => [...new Set((template.value?.slots ?? []).map((s) => s.name).filter((n) => keys.value.has(n)))])
const staticSlots = computed(() => {
  const seen = new Map<string, number>()
  for (const s of template.value?.slots ?? []) if (!keys.value.has(s.name) && !seen.has(s.name)) seen.set(s.name, s.width)
  return [...seen].map(([name, width]) => ({ name, width }))
})

// Rebuild the inputs when the fixed-slot set changes: keep values whose slot remains, drop the rest with a warning.
watch(staticSlots, (slots) => {
  if (!template.value) return
  const want = new Set(slots.map((s) => s.name))
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries(values.value)) {
    if (want.has(k)) next[k] = v
    else if (v !== '' &&!dropped.value.includes(k)) dropped.value.push(k) // an empty input is not worth a warning
  }
  for (const s of slots) next[s.name] ??= ''
  values.value = next
})

async function loadTemplate() {
  if (templateId.value === null) return
  try {
    template.value = await getTemplate(templateId.value)
  } catch (e) {
    formError.value = message(e)
  }
}

const isBad = (n: string) => fromEditable(values.value[n] ?? '') instanceof Error
const anyBad = computed(() => staticSlots.value.some((s) => isBad(s.name)))

function reset() {
  editing.value = null
  copyOf.value = ''
  formError.value = ''
  name.value = mpp.value = sheet.value = range.value = columns.value = formats.value = ''
  rows.value = 1
  values.value = {}
  dropped.value = []
  template.value = null
  templateId.value = null
}

function newBinding() {
  reset()
  showForm.value = true
}

const copyOf = ref('') // the name copied from, while the form is a copy
const mppInput = ref<HTMLInputElement | null>(null)

// Edit and Copy fill the form the same way. A copy stays a create: it takes a new name and no page.
async function fill(b: BindingSummary, asCopy: boolean) {
  reset()
  try {
    const d = await getBinding(b.id)
    editing.value = asCopy ? null : d.id
    copyOf.value = asCopy ? d.name : ''
    name.value = asCopy ? `${d.name} copy`.slice(0, 100) : d.name
    mpp.value = asCopy ? '' : d.mpp
    sheet.value = d.source.sheet
    range.value = d.source.range
    columns.value = d.source.columns.join(', ')
    rows.value = d.source.rows
    formats.value = Object.entries(d.source.format ?? {}).map(([k, v]) => `${k}=${v}`).join('\n')
    values.value = Object.fromEntries(Object.entries(d.static).map(([k, v]) => [k, toEditable(v)]))
    templateId.value = d.templateId
    showForm.value = true
    await loadTemplate()
    if (asCopy) {
      await nextTick()
      mppInput.value?.focus()
    }
  } catch (e) {
    error.value = message(e)
  }
}
const edit = (b: BindingSummary) => fill(b, false)
const copyBinding = (b: BindingSummary) => fill(b, true)

function insertCode(char: string) {
  const el = staticInputs.value.find((i) => i.getAttribute('aria-label') === lastSlot.value)
  if (!el || (el.value.length >= el.maxLength && el.selectionStart === el.selectionEnd)) return
  el.setRangeText(char, el.selectionStart ?? el.value.length, el.selectionEnd ?? el.value.length, 'end')
  el.dispatchEvent(new Event('input')) // keeps v-model and the counter in step
  el.focus()
}

function parseFormats(): Record<string, string> | Error {
  const out: Record<string, string> = {}
  const lines = formats.value.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim()
    if (line === '') continue
    const eq = line.indexOf('=')
    if (eq < 1) return new Error(`Formats line ${i + 1} must be column=formatter`)
    const spec = line.slice(eq + 1).trim()
    if (/,\s*[^\s=,]+=/.test(spec)) return new Error(`Formats line ${i + 1}: put each column on its own line`)
    out[line.slice(0, eq).trim()] = spec
  }
  return out
}

async function save() {
  if (anyBad.value || templateId.value === null) return
  formError.value = ''
  const format = parseFormats()
  if (format instanceof Error) {
    formError.value = format.message
    return
  }
  const fixed: Record<string, string> = {}
  for (const s of staticSlots.value) fixed[s.name] = fromEditable(values.value[s.name] ?? '') as string
  const body = {
    name: name.value, templateId: templateId.value, mpp: mpp.value.toUpperCase(),
    source: { sheet: sheet.value, range: range.value, columns: columnList.value, rows: Number(rows.value), format },
    static: fixed,
  }
  try {
    if (editing.value === null) await createBinding(body)
    else await updateBinding(editing.value, body)
  } catch (e) {
    formError.value = message(e)
    return
  }
  dropped.value = []
  showForm.value = false
  reset()
  await load()
}
</script>

<template>
  <fieldset class="panel">
  <legend><h1>Bindings</h1></legend>
  <details class="help">
    <summary>Help</summary>
    <ul>
      <li>One binding fills one page from one range of a Google Sheet, using one template.</li>
      <li>Columns map the range's columns to slots, one name per column. <code>#</code> is the entry number, so <code>p#name</code> fills <code>p1name</code>, <code>p2name</code> and so on.</li>
      <li>Fixed values fill the slots the sheet does not.</li>
      <li>Copy opens a new binding with every setting of this one except the page. Type a page and save.</li>
      <li>Runs happen when the server starts and at the top of every hour, or when you press Run now.</li>
      <li>Share the sheet with the service account's email address, or the run fails.</li>
    </ul>
    <p><RouterLink to="/help">Full guide</RouterLink></p>
  </details>
  <p v-if="!sheetsConfigured" role="status" class="off warning">
    Google Sheets not configured: scheduled runs are off. Set googleKeyFile in the server config. Run now still runs and records the failure.
  </p>
  <p v-if="error" role="alert">{{ error }}</p>
  <table class="bindings">
    <thead>
      <tr><th>Name</th><th>Template</th><th>Page</th><th>Last run</th><th></th></tr>
    </thead>
    <tbody>
      <tr v-for="b in bindings" :key="b.id" :aria-selected="selected?.id === b.id">
        <td><button type="button" class="select" @click="select(b)">{{ b.name }}</button></td>
        <td>{{ b.templateName }}</td>
        <td><RouterLink :to="`/pages/${b.mpp}`">{{ b.mpp }}</RouterLink> <button type="button" class="preview" @click="preview?.open(b.mpp)">Preview</button></td>
        <td class="last">
          <template v-if="b.lastRun">{{ localTime(b.lastRun.finishedAt) }} {{ b.lastRun.outcome }}<span v-if="b.lastRun.outcome === 'failed'">: {{ b.lastRun.error }}</span></template>
          <template v-else>never</template>
        </td>
        <td>
          <button type="button" class="run" :disabled="running[b.id]" @click="runNow(b)">Run now</button>
          <button type="button" class="edit" @click="edit(b)">Edit</button>
          <button type="button" class="copy" @click="copyBinding(b)">Copy</button>
          <button type="button" class="delete" @click="remove(b)">Delete</button>
          <span v-if="runResult[b.id]" class="result">{{ runResult[b.id] }}</span>
        </td>
      </tr>
    </tbody>
  </table>
  <p><button type="button" class="new" @click="newBinding">New binding</button></p>
  <PreviewDialog ref="preview" />

  <section v-if="selected" class="runs">
    <h2>Runs of {{ selected.name }}</h2>
    <p v-if="!runs.length">No runs yet.</p>
    <table v-else>
      <thead>
        <tr><th>Started</th><th>Finished</th><th>By</th><th>Outcome</th><th>Error</th></tr>
      </thead>
      <tbody>
        <tr v-for="r in runs" :key="r.id">
          <td>{{ localTime(r.startedAt) }}</td><td>{{ localTime(r.finishedAt) }}</td><td>{{ r.actor }}</td><td>{{ r.outcome }}</td><td>{{ r.error }}</td>
        </tr>
      </tbody>
    </table>
  </section>

  <form v-if="showForm" class="binding" @submit.prevent="save">
    <h2>{{ editing !== null ? 'Edit binding' : copyOf ? `New binding (copy of ${copyOf})` : 'New binding' }}</h2>
    <p v-if="formError" role="alert" class="form-error">{{ formError }}</p>
    <p><label>Name <input v-model="name" name="name" required maxlength="100" /></label></p>
    <p>
      <label>Template
        <select v-model="templateId" name="template" required @change="loadTemplate">
          <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
      </label>
    </p>
    <p><label>Page <input ref="mppInput" v-model="mpp" name="mpp" required pattern="[1-8][0-9A-Fa-f]{2}" maxlength="3" size="3" /></label></p>
    <fieldset>
      <legend>Sheet</legend>
      <p><label>Sheet id <input v-model="sheet" name="sheet" type="text" autocomplete="off" spellcheck="false" required /></label></p>
      <p><label>Range <input v-model="range" name="range" placeholder="C5:E" required /></label></p>
      <p><label>Columns <input v-model="columns" name="columns" placeholder="p#name, p#score, p#date" required /></label></p>
      <p><label>Rows <input v-model.number="rows" name="rows" type="number" min="1" max="24" required /></label></p>
      <p>
        <label>Formats <textarea v-model="formats" name="formats" rows="3" cols="30" placeholder="p#scr=number unit=sec&#10;p#date=date"></textarea></label>
        <small class="hint">One per line: column=formatter</small>
      </p>
    </fieldset>
    <fieldset v-if="template">
      <legend>Fixed values</legend>
      <p v-if="filled.length" class="filled">filled from the sheet: {{ filled.join(', ') }}</p>
      <p v-if="dropped.length" role="status" class="dropped warning">Dropped values for: {{ dropped.join(', ') }} (not fixed slots of the selected template)</p>
      <div v-if="staticSlots.length" class="palette">
        <button v-for="p in PALETTE" :key="p.code" type="button" :title="p.name" @click="insertCode(p.char)">{{ p.char }} {{ p.name }}</button>
      </div>
      <p v-for="s in staticSlots" :key="s.name" class="slot">
        <label>{{ s.name }} (width {{ s.width }})
          <input
            ref="staticInputs"
            v-model="values[s.name]"
            type="text"
            :maxlength="s.width"
            spellcheck="false"
            :aria-label="s.name"
            :aria-invalid="isBad(s.name)"
            :class="{ bad: isBad(s.name) }"
            @focus="lastSlot = s.name"
          />
        </label>
        <span class="count">{{ (values[s.name] ?? '').length }}/{{ s.width }}</span>
      </p>
    </fieldset>
    <p>
      <button type="submit" :disabled="anyBad || templateId === null">Save</button>
      <button type="button" @click="showForm = false">Cancel</button>
    </p>
  </form>
  </fieldset>
</template>
