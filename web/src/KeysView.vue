<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { createKey, createKeyGrant, deleteGrant, listKeys, revokeKey, type KeyDoc } from './api'
import { localTime } from './time'

const keys = ref<KeyDoc[]>([])
const error = ref('')
const name = ref('')
const readOnly = ref(false)
const expires = ref('') // datetime-local value, local time; '' for no expiry
// A token the server returned; it is shown once and dropped on the next action.
const shown = ref<{ name: string; token: string } | null>(null)
// The grant form of each row, by key id.
const drafts = reactive<Record<string, { lo: string; hi: string; access: 'read' | 'write' }>>({})

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

async function load() {
  try {
    keys.value = await listKeys()
    for (const k of keys.value) drafts[k.id] ??= { lo: '', hi: '', access: 'read' }
  } catch (e) {
    error.value = message(e)
  }
}
onMounted(load)

// run clears the last error and shown token, runs f, then reloads the list.
async function run(f: () => Promise<void>) {
  error.value = ''
  shown.value = null
  try {
    await f()
    await load()
  } catch (e) {
    error.value = message(e)
  }
}

const create = () =>
  run(async () => {
    const r = await createKey(name.value, readOnly.value, expires.value ? new Date(expires.value).toISOString() : undefined)
    shown.value = { name: r.key.name, token: r.token }
    name.value = ''
    readOnly.value = false
    expires.value = ''
  })

function revoke(k: KeyDoc) {
  if (!window.confirm(`Revoke ${k.name}? It stops working on its next request.`)) return
  return run(() => revokeKey(k.id))
}

const addGrant = (k: KeyDoc) =>
  run(async () => {
    const d = drafts[k.id]
    await createKeyGrant(k.id, d.lo.toUpperCase(), d.hi.toUpperCase(), d.access)
    drafts[k.id] = { lo: '', hi: '', access: 'read' }
  })

const removeGrant = (id: number) => run(() => deleteGrant(id))
</script>

<template>
  <fieldset class="panel">
  <legend><h1>API keys</h1></legend>
  <p v-if="error" role="alert">{{ error }}</p>
  <section v-if="shown" class="token">
    <p>Token for {{ shown.name }}: <code>{{ shown.token }}</code></p>
    <p>Copy it now. It is not shown again.</p>
  </section>
  <table>
    <thead>
      <tr><th>Name</th><th>Read-only</th><th>Expires</th><th>Last used</th><th>Revoked</th><th>Grants</th><th></th></tr>
    </thead>
    <tbody>
      <tr v-for="k in keys" :key="k.id" :data-key="k.id">
        <td>{{ k.name }}</td>
        <td>{{ k.readOnly ? 'yes' : '' }}</td>
        <td>{{ localTime(k.expiresAt, 'never') }}</td>
        <td>{{ localTime(k.lastUsedAt, 'never') }}</td>
        <td>{{ localTime(k.revokedAt) }}</td>
        <td>
          <ul>
            <li v-for="g in k.grants" :key="g.id">
              {{ g.lo }}-{{ g.hi }} {{ g.access }}<template v-if="k.readOnly && g.access === 'write'"> (read only: the key cannot write)</template>
              <button type="button" class="delete-grant" @click="removeGrant(g.id)">Delete</button>
            </li>
          </ul>
          <form v-if="!k.revokedAt" class="grant" @submit.prevent="addGrant(k)">
            <input v-model="drafts[k.id].lo" name="lo" placeholder="100" size="3" maxlength="3" required aria-label="From page" />
            <input v-model="drafts[k.id].hi" name="hi" placeholder="1FF" size="3" maxlength="3" required aria-label="To page" />
            <select v-model="drafts[k.id].access" name="access" aria-label="Access">
              <option value="read">read</option>
              <option value="write">write</option>
            </select>
            <button type="submit">Add grant</button>
          </form>
        </td>
        <td>
          <button v-if="!k.revokedAt" type="button" class="revoke" @click="revoke(k)">Revoke</button>
        </td>
      </tr>
    </tbody>
  </table>
  <form class="create" @submit.prevent="create">
    <h2>Create key</h2>
    <label>Name <input v-model="name" name="name" required maxlength="100" /></label>
    <label><input v-model="readOnly" name="readOnly" type="checkbox" /> Read-only</label>
    <label>Expires <input v-model="expires" name="expiresAt" type="datetime-local" /></label>
    <button type="submit">Create</button>
  </form>
  </fieldset>
</template>
