<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  createFastextGroup, deleteFastextGroup, getFastextGroups, setFastextGroupPages, updateFastextGroup,
  type FastextGroup, type FastextMember,
} from './api'
import CellInput from './editor/CellInput.vue'
import TeletextPage from './teletext/TeletextPage.vue'
import { row24 } from './fastext'

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

// The four coloured keys, as in the page editor.
const KEYS = ['Red', 'Green', 'Yellow', 'Cyan']
const COLOURS = ['red', 'green', 'yellow', 'cyan']

const route = useRoute()
const groups = ref<FastextGroup[]>([])
const error = ref('')

async function load() {
  try {
    groups.value = await getFastextGroups()
  } catch (e) {
    error.value = message(e)
  }
}

// The form. editing is the group id, 'new' when creating, or null when closed.
const editing = ref<number | 'new' | null>(null)
const name = ref('')
const paged = ref(false)
const links = ref(['', '', '', ''])
const texts = ref(['', '', '', ''])
const textBad = ref([false, false, false, false])
const prevSlot = ref(0)
const nextSlot = ref(1)
const isDefault = ref(false)
const members = ref<FastextMember[]>([])
const addMpp = ref('')

function open(g: FastextGroup | null) {
  error.value = ''
  editing.value = g ? g.id : 'new'
  name.value = g?.name ?? ''
  paged.value = g?.paged ?? false
  prevSlot.value = g?.prevSlot ?? 0
  nextSlot.value = g?.nextSlot ?? 1
  // a stored 8ff is not shown: a slot's input says prev or next, any other box is blank
  links.value = (g?.links ?? ['', '', '', '']).map((l) => (l.toLowerCase() === '8ff' ? '' : l))
  texts.value = [...(g?.texts ?? ['', '', '', ''])]
  textBad.value = [false, false, false, false]
  isDefault.value = g?.default ?? false
  members.value = g ? g.pages.map((p) => ({ ...p })) : []
  addMpp.value = ''
}

onMounted(async () => {
  await load()
  const id = Number(route.query.group)
  const g = groups.value.find((x) => x.id === id)
  if (g) open(g)
})

// Exactly one Prev and one Next, on different keys: a press on the other's key swaps them.
function pickPrev(k: number) {
  if (nextSlot.value === k) nextSlot.value = prevSlot.value
  prevSlot.value = k
}
function pickNext(k: number) {
  if (prevSlot.value === k) prevSlot.value = nextSlot.value
  nextSlot.value = k
}
const slotOf = (k: number) => (paged.value ? (k === prevSlot.value ? 'prev' : k === nextSlot.value ? 'next' : '') : '')
const preview = computed(() => row24(texts.value, paged.value ? prevSlot.value : null, paged.value ? nextSlot.value : null))

async function save() {
  error.value = ''
  const body = {
    name: name.value, paged: paged.value, default: isDefault.value,
    links: links.value.map((l, k) => (slotOf(k) || !l.trim() ? '8ff' : l)), texts: texts.value,
    prevSlot: paged.value ? prevSlot.value : null, nextSlot: paged.value ? nextSlot.value : null,
  }
  try {
    const g = editing.value === 'new' ? await createFastextGroup(body) : await updateFastextGroup(editing.value as number, body)
    await load()
    open(g)
  } catch (e) {
    error.value = message(e)
  }
}

async function remove(g: FastextGroup) {
  if (!window.confirm(`Delete group ${g.name}? Its pages keep their current links and lose row 24.`)) return
  error.value = ''
  try {
    await deleteFastextGroup(g.id)
  } catch (e) {
    error.value = message(e)
    return
  }
  if (editing.value === g.id) editing.value = null
  await load()
}

function add() {
  const mpp = addMpp.value.trim().toUpperCase()
  if (members.value.some((m) => m.mpp === mpp)) {
    error.value = `Page ${mpp} is already in the list.`
    return
  }
  error.value = ''
  members.value.push({ mpp, description: '' })
  addMpp.value = ''
}

function move(i: number, by: number) {
  const j = i + by
  if (j < 0 || j >= members.value.length) return
  const m = members.value
  ;[m[i], m[j]] = [m[j], m[i]]
}

async function saveMembers() {
  error.value = ''
  try {
    const g = await setFastextGroupPages(editing.value as number, members.value.map((m) => m.mpp))
    members.value = g.pages.map((p) => ({ ...p }))
    await load()
  } catch (e) {
    error.value = message(e)
  }
}
</script>

<template>
  <fieldset class="panel">
    <legend><h1>Fastext</h1></legend>
    <p v-if="error" role="alert">{{ error }}</p>
    <table>
      <thead>
        <tr><th>Name</th><th>Type</th><th>Default</th><th>Pages</th><th></th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="g in groups" :key="g.id">
          <td>{{ g.name }}</td>
          <td>{{ g.paged ? 'Paged' : 'Static' }}</td>
          <td>{{ g.default ? 'Default' : '' }}</td>
          <td>{{ g.pages.length }}</td>
          <td><button type="button" @click="open(g)">Edit</button></td>
          <td><button type="button" @click="remove(g)">Delete</button></td>
        </tr>
      </tbody>
    </table>
    <p><button v-if="editing === null" type="button" @click="open(null)">New</button></p>

    <form v-if="editing !== null" @submit.prevent="save">
      <h2>{{ editing === 'new' ? 'New group' : 'Edit group' }}</h2>
      <p><label>Name <input v-model="name" name="name" maxlength="40" required autocomplete="off" /></label></p>
      <p class="kind">
        <label><input v-model="paged" type="radio" name="kind" :value="false" /> Static</label>
        <label><input v-model="paged" type="radio" name="kind" :value="true" /> Paged</label>
      </p>
      <div class="links">
        <div v-for="(key, k) in KEYS" :key="k">
          <label>{{ key }}
            <input
              v-model="links[k]" :name="`link${k}`" maxlength="3" size="3" class="link" :class="COLOURS[k]"
              pattern="[1-8][0-9A-Fa-f]{2}" :disabled="!!slotOf(k)" :placeholder="slotOf(k)" autocomplete="off"
            />
          </label>
          <CellInput v-model="texts[k]" :maxlength="9" :disabled="!!slotOf(k)" :label="`${key} text`" class="text" @invalid="(b) => (textBad[k] = b)" />
          <p v-if="paged" class="slots">
            <button type="button" :aria-pressed="prevSlot === k" @click="pickPrev(k)">Prev</button>
            <button type="button" :aria-pressed="nextSlot === k" @click="pickNext(k)">Next</button>
          </p>
        </div>
      </div>
      <p>
        <label><input v-model="isDefault" name="default" type="checkbox" /> Default</label>
        <small>New pages join this group.</small>
      </p>
      <TeletextPage :rows="[preview]" :row-count="1" aria-label="Row 24 preview" />
      <p><small>Esc then a key in a text box inserts a control code, as in the page editor. The first page has no Previous text and the last has no Next text.</small></p>
      <p><button type="submit" :disabled="textBad.some(Boolean)">Save</button> <button type="button" @click="editing = null">Close</button></p>
    </form>

    <section v-if="typeof editing === 'number'">
      <h2>Members</h2>
      <p v-if="paged"><small>Previous and Next follow this order. The first page has no Previous link and the last has no Next link.</small></p>
      <p v-if="!members.length">No pages in this group.</p>
      <ol class="members">
        <li v-for="(m, i) in members" :key="m.mpp">
          <span class="mpp">{{ m.mpp }}</span> {{ m.description }}
          <button type="button" :disabled="i === 0" @click="move(i, -1)">Up</button>
          <button type="button" :disabled="i === members.length - 1" @click="move(i, 1)">Down</button>
          <button type="button" @click="members.splice(i, 1)">Remove</button>
        </li>
      </ol>
      <p>
        <label>Add page <input v-model="addMpp" name="addMpp" maxlength="3" size="3" pattern="[1-8][0-9A-Fa-f]{2}" autocomplete="off" @keydown.enter.prevent="/^[1-8][0-9A-Fa-f]{2}$/.test(addMpp.trim()) && add()" /></label>
        <button type="button" :disabled="!/^[1-8][0-9A-Fa-f]{2}$/.test(addMpp.trim())" @click="add">Add</button>
      </p>
      <p><button type="button" @click="saveMembers">Save members</button></p>
    </section>
  </fieldset>
</template>

<style scoped>
.kind { display: flex; gap: 1rem; }
.links { display: flex; flex-wrap: wrap; gap: 0.5rem 1.5rem; }
.links input { width: 6ch; }
.links .text { width: 11ch; display: block; }
.links input.red, .links input.green, .links input.yellow, .links input.cyan { color: #000; box-shadow: none; }
.links input.red { background: #f00; } .links input.green { background: #0f0; }
.links input.yellow { background: #ff0; } .links input.cyan { background: #0ff; }
.links input:disabled { opacity: 0.6; }
.slots { display: flex; gap: 0.25rem; }
.members { padding-left: 2rem; }
.members li { margin-bottom: 0.25rem; }
.mpp { font-family: monospace; }
</style>
