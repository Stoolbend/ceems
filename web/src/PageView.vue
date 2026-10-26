<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError, deletePage, getPage, putPage, uploadTTI, type Page, type Subpage } from './api'
import KeysHelp from './editor/KeysHelp.vue'
import PageEditor from './editor/PageEditor.vue'
import { PALETTE } from './editor/cells'
import TeletextPage from './teletext/TeletextPage.vue'
import { DEFAULT_HEADER, headerRow, vbitHeader } from './teletext/header'
import { user } from './session'
import { decode, encode } from './zxnet'

// One subpage as edited: rows are cell values (rows 1-24 padded to 40), and orig keeps the rows as loaded
// (null for a subpage added in this session) so untouched rows are sent unchanged.
type Edit = {
  cycle: string
  mode: string
  links: string[]
  linksLoaded: boolean
  rows: string[]
  orig: (string | null)[] | null
}

// The six fastext links in FL order; the first four are the coloured keys.
const LINK_NAMES = ['Red', 'Green', 'Yellow', 'Cyan', 'Link 5', 'Index']
const LINK_COLOURS = ['red', 'green', 'yellow', 'cyan', '', '']
const SPACES = ' '.repeat(40)
const mpp = String(useRoute().params.mpp).toUpperCase()
const router = useRouter()
const page = ref<Page | null>(null) // the document as the server last sent it
const edits = ref<Edit[]>([])
const description = ref('')
const transmit = ref(false)
const current = ref(0)
const loadedAt = ref(new Date())
const error = ref('')
const stale = ref(false)
const file = ref<File | null>(null)
const importText = ref('')
const showCodes = ref(false) // view options only: not saved
const showSlots = ref(false)
const grid = ref(false)
const draw = ref(false)
const editor = ref<InstanceType<typeof PageEditor> | null>(null)
const SECTIONS = [['alpha', 'Alpha'], ['mosaic', 'Mosaic'], ['other', 'Other']] as const

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

function toEdit(s: Subpage): Edit {
  return {
    cycle: s.cycleSeconds === null ? '' : String(s.cycleSeconds),
    mode: s.cycleMode ?? 'T',
    links: s.links ?? Array(6).fill(''),
    linksLoaded: s.links !== null,
    rows: s.rows.map((r, i) => (i === 0 ? (r ?? '') : (r ?? '').padEnd(40, ' '))),
    orig: s.rows,
  }
}

// Replaces all local state with a document from the server.
function show(p: Page) {
  page.value = p
  description.value = p.description
  transmit.value = p.transmit
  edits.value = p.subpages.map(toEdit)
  current.value = Math.min(current.value, p.subpages.length - 1)
  loadedAt.value = new Date()
  stale.value = false
}

onMounted(async () => {
  try {
    show(await getPage(mpp))
  } catch (e) {
    error.value = message(e)
  }
})

const sub = computed(() => edits.value[current.value])
const bound = computed(() => page.value?.binding ?? null) // a generated page is read-only here
const locked = computed(() => !!bound.value || page.value?.canWrite === false)

// The editor shows a synthetic header in row 0; only rows 1-24 come back from it.
const headerCells = () => user.value?.headerTemplate ? vbitHeader(user.value.headerTemplate, mpp, loadedAt.value) : headerRow(DEFAULT_HEADER, mpp, loadedAt.value)
const previewRows = computed(() => [headerCells(), ...sub.value.rows.slice(1)])
// A link the server sets (D-065): the four coloured keys from the page's fastext group, the index from the system settings.
const ruled = (k: number) => {
  const g = page.value?.fastextGroup
  if (g && k < 4) return { value: sub.value.links[k], title: `From fastext group ${g.name}` }
  if (k === 5 && user.value?.indexPage) return { value: user.value.indexPage, title: 'Index page from system settings' }
  return null
}
// A grouped page's row 24 is the server's: it is never taken from the editor or an import.
const lastRow = computed(() => (page.value?.fastextGroup ? 23 : 24))
const setRows = (rows: string[]) => (sub.value.rows = [sub.value.rows[0], ...rows.slice(1, lastRow.value + 1), ...sub.value.rows.slice(lastRow.value + 1)])

// Row 0 is the preview's synthetic header, so it is exported as spaces.
const zxnetUrl = computed(() => encode([SPACES, ...previewRows.value.slice(1)], mpp, sub.value.links))

function addSubpage() {
  const from = sub.value
  edits.value.push({ ...from, links: [...from.links], rows: ['', ...Array(24).fill(SPACES)], orig: null })
  current.value = edits.value.length - 1
}

function removeSubpage() {
  edits.value.splice(current.value, 1)
  current.value = Math.min(current.value, edits.value.length - 1)
}

// A row the user changed is padded to 40 cells (D-009); an untouched row is sent as loaded.
function subpageDoc(e: Edit): Subpage {
  const cycle = String(e.cycle).trim() // v-model on type=number yields a number once edited
  return {
    cycleSeconds: cycle === '' ? null : Number(cycle),
    cycleMode: cycle === '' ? null : e.mode,
    links: e.linksLoaded || e.links.some((l) => l !== '') ? e.links : null,
    rows: Array.from({ length: 25 }, (_, r) => {
      const orig = e.orig?.[r] ?? null
      if (r === 0 || (e.orig && e.rows[r] === (orig ?? '').padEnd(40, ' '))) return orig
      return e.rows[r]
    }),
  }
}

async function save() {
  if (!page.value) return
  error.value = ''
  stale.value = false
  try {
    show(await putPage({ ...page.value, description: description.value, transmit: transmit.value, subpages: edits.value.map(subpageDoc) }))
  } catch (e) {
    stale.value = e instanceof ApiError && e.status === 412
    error.value = message(e)
  }
}

async function remove() {
  if (!page.value || !window.confirm(`Delete page ${mpp}? Its last 20 versions are kept.`)) return
  error.value = ''
  stale.value = false
  try {
    await deletePage(mpp, page.value.etag)
  } catch (e) {
    stale.value = e instanceof ApiError && e.status === 412
    error.value = message(e)
    return
  }
  await router.push('/')
}

// Replaces rows 1 to 24 of the current subpage in the editor only; nothing is sent until Save.
function importZxnet() {
  const d = decode(importText.value)
  if (d instanceof Error) {
    error.value = d.message
    return
  }
  error.value = ''
  for (let r = 1; r <= lastRow.value; r++) edits.value[current.value].rows[r] = d.rows[r]
}

async function reload() {
  error.value = ''
  try {
    show(await getPage(mpp))
  } catch (e) {
    error.value = message(e)
  }
}

function pick(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] ?? null
}

async function upload() {
  if (!file.value) return
  error.value = ''
  try {
    show(await uploadTTI(mpp, file.value))
  } catch (e) {
    error.value = message(e)
  }
}
</script>

<template>
  <!-- the workspace always renders, so the heading, an error and Download show before a page loads, as before -->
  <div class="editor" :class="{ grouped: page?.fastextGroup }">
    <div v-if="page && sub" class="editor-grid rows">
      <PageEditor ref="editor" :key="`${current}/${edits.length}`" :rows="previewRows" :show-codes="showCodes && !locked" :show-slots="showSlots && !locked" :grid="grid && !locked" :draw="draw && !locked" :disabled="locked" :last-row="lastRow" @update:rows="setRows" @error="(m) => (error = m)" />
      <div class="links">
        <label v-for="(name, k) in LINK_NAMES" :key="k">{{ name }} <input :value="ruled(k) ? ruled(k)!.value : sub.links[k]" maxlength="3" size="3" class="link" :class="LINK_COLOURS[k]" :disabled="locked || !!ruled(k)" :title="ruled(k)?.title" @input="sub.links[k] = ($event.target as HTMLInputElement).value" /></label>
        <p v-if="page.fastextGroup" role="note" class="fastext"><span aria-hidden="true">►</span> This page is part of fastext group
          <RouterLink v-if="user?.admin" :to="`/fastext?group=${page.fastextGroup.id}`">{{ page.fastextGroup.name }}</RouterLink><template v-else>{{ page.fastextGroup.name }}</template><template v-if="page.fastextGroup.paged"> (paged)</template>.</p>
      </div>
    </div>
    <div class="side">
      <fieldset class="panel"><legend><h1>Page {{ mpp }}</h1></legend>
      <p v-if="error" role="alert">{{ error }} <button v-if="stale" type="button" @click="reload">Reload</button></p>
      <p v-if="bound" role="status" class="bound warning">Generated by binding {{ bound.name }}. Edit its template or binding instead.</p>
      <p v-if="page?.canWrite === false" role="status" class="readonly warning">Read only: you can view this page but not change it.</p>
      <template v-if="page && sub">
        <p v-if="!bound">
          <button type="button" :disabled="locked" @click="save">Save</button>
          <button type="button" :disabled="locked" @click="remove">Delete</button>
        </p>
        <p>
          <label>Description <input v-model="description" maxlength="255" class="description" :disabled="locked" /></label>
          <small>Updated {{ page.updatedAt }} by {{ page.updatedBy }}</small>
        </p>
        <nav class="tabs" aria-label="Subpages">
          <button v-for="(_, i) in edits" :key="i" type="button" :aria-pressed="i === current" @click="current = i">{{ i + 1 }}</button>
          <button type="button" :disabled="locked" @click="addSubpage">Add subpage</button>
          <button type="button" :disabled="locked || edits.length === 1" @click="removeSubpage">Remove subpage</button>
        </nav>
        <h3>Settings</h3>
        <div class="meta">
          <label><input v-model="transmit" type="checkbox" :disabled="locked" />Transmit</label>
          <label>Mode <select v-model="sub.mode" :disabled="locked"><option value="T">Timed</option><option value="C">Cycles</option></select></label>
          <label>Cycle {{ sub.mode === 'C' ? 'count' : 'seconds' }} <input v-model="sub.cycle" type="number" min="1" max="999" class="cycle" :disabled="locked" /></label>
        </div>
        <div v-if="!locked" class="palette">
          <section v-for="[group, title] in SECTIONS" :key="group">
            <h3>{{ title }}</h3>
            <div class="buttons">
              <button v-for="p in PALETTE.filter((x) => x.group === group)" :key="p.code" type="button" class="ctrlcode-button" :aria-label="p.name" :title="`${p.name} (Esc ${p.key})`" @click="editor?.write(p.code)"><!-- shortcut: a div inside a button is non-conforming HTML, but every browser renders it; upgrade if validation matters --><span class="sample"><TeletextPage :rows="[p.sample]" :row-count="p.rows" :col-count="p.sample.length" aria-hidden="true" /></span><kbd aria-hidden="true">{{ p.key }}</kbd></button>
            </div>
          </section>
        </div>
        <KeysHelp />
        <p class="view">
          <label><input v-model="showCodes" type="checkbox" name="showCodes" :disabled="locked" /> Show control codes</label>
          <label><input v-model="showSlots" type="checkbox" name="showSlots" :disabled="locked" /> Show template slots</label>
          <label><input v-model="grid" type="checkbox" name="grid" :disabled="locked" /> Grid</label>
          <label><input v-model="draw" type="checkbox" name="draw" :disabled="locked" /> Draw</label>
        </p>
      </template>
      <p><a :href="`/api/v1/pages/${mpp}/tti`" download>Download P{{ mpp }}.tti</a></p>
      <p v-if="sub"><a :href="zxnetUrl" target="_blank" rel="noopener">Open subpage {{ current + 1 }} in zxnet editor</a></p>
      <fieldset v-if="!bound" class="import" role="group">
        <input v-model="importText" name="zxnet" :disabled="locked" autocomplete="off" placeholder="zxnet URL" aria-label="zxnet URL" />
        <button type="button" :disabled="locked" @click="importZxnet">Import into subpage {{ current + 1 }}</button>
      </fieldset>
      <form v-if="!bound" class="upload" @submit.prevent="upload">
        <label>Replace from file <input type="file" accept=".tti" :disabled="locked" @change="pick" /></label>
        <button type="submit" :disabled="locked || !file">Upload</button>
      </form>
      </fieldset>
    </div>
  </div>
</template>

<style scoped>
.view { display: flex; gap: 1rem; margin-bottom: 0.25rem; }
.palette h3 { margin: 0.5em 0 0.25em; }
.palette .buttons { display: grid; grid-template-columns: repeat(auto-fill, minmax(3rem, 1fr)); gap: 0.5rem; }
.meta { display: flex; flex-wrap: wrap; gap: 1rem; }
.palette button { position: relative; line-height: normal; font-size: 0.8em; height: 3em; width: 3em; margin: 0; padding: 0.2em; display: flex; align-items: center; justify-content: center; }
.sample { flex: none; width: 100%; height: 2.25em; display: flex; align-items: center; justify-content: flex-start; }
.palette kbd { position: absolute; right: 0; bottom: 0; padding: 0 0.3em; font-size: 0.75em; line-height: 1.3; color: #fff; background: #000; }
.palette button :deep(.tt-page) { width: calc(var(--cols) * 0.8em); }
.links { display: flex; flex-wrap: wrap; gap: 0.5rem 1rem; margin-top: 0.5rem; }
.links input { width: 5ch; }
/* the four coloured keys are fields in the preview's own colours, with black text and no edge */
.links input.red, .links input.green, .links input.yellow, .links input.cyan { color: #000; box-shadow: none; }
.links input.red { background: #f00; } .links input.green { background: #0f0; }
.links input.yellow { background: #ff0; } .links input.cyan { background: #0ff; }
.fastext { flex: 0 0 100%; min-width: 0; margin: 0; }
.cycle { width: 5em; }
.description { width: 24em; max-width: 100%; }
/* the global nav spreads its children; the subpage tabs sit left and wrap */
.tabs { flex-wrap: wrap; justify-content: flex-start; gap: 0.5rem; }

/* Wide screens: the editor column is the largest 4:3 box that fits beside a 22rem side panel, above a status
   line and a fastext strip of fixed heights; the side panel takes the rest and scrolls on its own. */
@media (min-width: 1024px) {
  .editor {
    --pad: 0.5rem; --status: 2rem; --strip: 2rem; --side: 22rem; --gap: 1rem;
    height: calc(100dvh - var(--app-header) - var(--app-footer));
    padding-block: var(--pad);
    display: grid;
    grid-template-rows: minmax(0, 1fr);
    /* a grid track percentage is of the workspace's width; the page is 4:3, so its width is 4/3 of the height left */
    grid-template-columns: min(100% - var(--side) - var(--gap), (100dvh - var(--app-header) - var(--app-footer) - 2 * var(--pad) - var(--status) - var(--strip)) * 4 / 3) minmax(var(--side), 1fr);
    column-gap: var(--gap);
  }
  .editor-grid :deep(.status) { height: 1rem; line-height: 1rem; margin: 1rem 0 0; white-space: nowrap; }
  .links { height: var(--strip); margin: 0; flex-wrap: nowrap; align-items: center; gap: 0.75rem; }
  .links label { display: flex; align-items: center; gap: 0.25rem; margin: 0; white-space: nowrap; }
  /* a grouped page's strip has a second line, the notice, cut with an ellipsis if too long */
  .grouped { --strip: 3.5rem; }
  .grouped .links { flex-wrap: wrap; align-content: center; gap: 0.1rem 0.75rem; }
  .grouped .fastext { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .side { min-height: 0; overflow-y: auto; }
}
</style>
