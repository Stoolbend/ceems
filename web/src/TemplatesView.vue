<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { createTemplate, deleteTemplate, getBinding, getTemplate, listBindings, listTemplates, updateTemplate, type BindingSummary, type Template, type TemplateSummary } from './api'

const templates = ref<TemplateSummary[]>([])
const error = ref('')
const name = ref('')
const file = ref<File | null>(null)
const rules = ref('{}')
const created = ref<Template | null>(null) // the last upload or replace, so its parsed slots can be checked
const editing = ref<Template | null>(null) // set in replace mode: the stored template being replaced
const usedBy = ref<BindingSummary[]>([])
const changes = ref('') // "Added: a. Removed: none." after a replace
const stale = ref<string[]>([]) // per binding, the fixed values the new template no longer uses
const fileKey = ref(0) // changing it clears the file input

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

async function load() {
  try {
    templates.value = await listTemplates()
  } catch (e) {
    error.value = message(e)
  }
}
onMounted(load)

function pick(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] ?? null
}

// readBase64 returns the file's bytes as base64 (the data URL without its prefix).
function readBase64(f: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).replace(/^[^,]*,/, ''))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(f)
  })
}

const pageOf = (b: BindingSummary) => `${b.name} (page ${b.mpp})`

async function replace(t: TemplateSummary) {
  error.value = ''
  created.value = null
  try {
    const [full, all] = await Promise.all([getTemplate(t.id), listBindings()])
    editing.value = full
    usedBy.value = all.bindings.filter((b) => b.templateId === t.id)
    name.value = full.name
    rules.value = JSON.stringify(full.rowRules, null, 2)
    file.value = null
    fileKey.value++
    changes.value = ''
    stale.value = []
  } catch (e) {
    error.value = message(e)
  }
}

function cancel() {
  editing.value = null
  name.value = ''
  rules.value = '{}'
  file.value = null
  fileKey.value++
}

// afterReplace sets the Added/Removed line and lists, per binding, the fixed values the new template ignores.
async function afterReplace(old: Template, now: Template, users: BindingSummary[]) {
  const was = old.slots.map((s) => s.name)
  const is = now.slots.map((s) => s.name)
  const list = (a: string[]) => [...new Set(a)].join(', ') || 'none'
  changes.value = `Added: ${list(is.filter((n) => !was.includes(n)))}. Removed: ${list(was.filter((n) => !is.includes(n)))}.`
  stale.value = []
  for (const u of users) {
    const extra = Object.keys((await getBinding(u.id)).static).filter((k) => !is.includes(k))
    if (extra.length) {
      stale.value.push(`${pageOf(u)} still has fixed values for: ${extra.join(', ')}. They are ignored. Open the binding and save it to drop them.`)
    }
  }
}

async function upload() {
  if (!file.value && !editing.value) return
  error.value = ''
  created.value = null
  changes.value = ''
  stale.value = []
  let rowRules: Record<string, string>
  try {
    rowRules = JSON.parse(rules.value || '{}')
  } catch {
    error.value = 'Row rules must be a JSON object, for example {"17": "p4name"}.'
    return
  }
  try {
    const old = editing.value
    if (old) {
      created.value = await updateTemplate(old.id, name.value, file.value ? await readBase64(file.value) : old.tti, rowRules)
      cancel()
      await load()
      try {
        await afterReplace(old, created.value, usedBy.value)
      } catch (e) {
        error.value = `Template saved, but the fixed-value check failed: ${message(e)}`
      }
    } else if (file.value) {
      created.value = await createTemplate(name.value, await readBase64(file.value), rowRules)
      await load()
    }
  } catch (e) {
    error.value = message(e)
  }
}

async function remove(t: TemplateSummary) {
  if (!window.confirm(`Delete template ${t.name}?`)) return
  error.value = ''
  try {
    await deleteTemplate(t.id)
    await load()
  } catch (e) {
    error.value = message(e)
  }
}
</script>

<template>
  <fieldset class="panel">
  <legend><h1>Templates</h1></legend>
  <details class="help">
    <summary>Help</summary>
    <ul>
      <li>To make a template, create a page, edit its rows in the editor, download it, then upload it here.</li>
      <li><code>{name}</code> marks a slot. The whole token's length, braces included, is its width, so add dots to widen it: <code>{name.....}</code>.</li>
      <li>The editor shows <code>{</code> and <code>}</code> as <code>¼</code> and <code>¾</code>. Tick Show template slots to see the braces.</li>
      <li><code>^</code> centres a value and <code>&gt;</code> right-aligns it: <code>{^name...}</code>, <code>{&gt;score..}</code>.</li>
      <li><code>updated</code> is filled with the time of the run.</li>
      <li>Row rules blank a row when a named slot is empty, for example <code>{"17": "p4name"}</code>.</li>
    </ul>
    <p><RouterLink to="/help">Full guide</RouterLink></p>
  </details>
  <p v-if="error" role="alert">{{ error }}</p>
  <table>
    <thead>
      <tr><th>Name</th><th>Slots</th><th></th><th></th><th></th></tr>
    </thead>
    <tbody>
      <tr v-for="t in templates" :key="t.id">
        <td>{{ t.name }}</td>
        <td>{{ t.slotCount }}</td>
        <td><a :href="`/api/v1/templates/${t.id}/tti`" download>Download</a></td>
        <td><button type="button" @click="replace(t)">Replace</button></td>
        <td><button type="button" @click="remove(t)">Delete</button></td>
      </tr>
    </tbody>
  </table>
  <form class="upload" @submit.prevent="upload">
    <template v-if="editing">
      <h2>Replace {{ editing.name }}</h2>
      <p v-if="usedBy.length" class="used-by">Used by: {{ usedBy.map(pageOf).join(', ') }}. These pages change at the next hourly run, or when you press Run now on the binding.</p>
      <p v-else class="used-by">No binding uses this template.</p>
    </template>
    <p><label>Name <input v-model="name" name="name" required maxlength="100" /></label></p>
    <p><label>Template file <input :key="fileKey" type="file" accept=".tti" @change="pick" /><template v-if="editing"> (optional: leave empty to keep the current file)</template></label></p>
    <p><label>Row rules (JSON) <textarea v-model="rules" name="rules" rows="2" cols="40"></textarea></label></p>
    <button type="submit" :disabled="(!file && !editing) || !name">{{ editing ? 'Replace' : 'Upload' }}</button>
    <button v-if="editing" type="button" @click="cancel">Cancel</button>
  </form>
  <p v-if="changes" class="changes">{{ changes }}</p>
  <p v-for="s in stale" :key="s" class="stale">{{ s }}</p>
  <section v-if="created">
    <h2>Slots in {{ created.name }}</h2>
    <p v-if="!created.slots.length">No slots found.</p>
    <table v-else class="slots">
      <thead>
        <tr><th>Name</th><th>Subpage</th><th>Row</th><th>Column</th><th>Width</th><th>Align</th></tr>
      </thead>
      <tbody>
        <tr v-for="s in created.slots" :key="`${s.sub}-${s.row}-${s.col}`">
          <td>{{ s.name }}</td><td>{{ s.sub + 1 }}</td><td>{{ s.row }}</td><td>{{ s.col }}</td><td>{{ s.width }}</td><td>{{ s.align }}</td>
        </tr>
      </tbody>
    </table>
  </section>
  </fieldset>
</template>
