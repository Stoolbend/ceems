<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { createGrant, createUser, deleteGrant, listUsers, resetPassword, setDisabled, type UserDoc } from './api'
import { user as me } from './session'

const users = ref<UserDoc[]>([])
const error = ref('')
const username = ref('')
const admin = ref(false)
// A password the server returned; it is shown once and dropped on the next action.
const shown = ref<{ username: string; password: string } | null>(null)
// The grant form of each row, by user id.
const drafts = reactive<Record<number, { lo: string; hi: string; access: 'read' | 'write' }>>({})

const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

async function load() {
  try {
    users.value = await listUsers()
    for (const u of users.value) drafts[u.id] ??= { lo: '', hi: '', access: 'read' }
  } catch (e) {
    error.value = message(e)
  }
}
onMounted(load)

// run clears the last error and shown password, runs f, then reloads the list.
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
    const r = await createUser(username.value, admin.value)
    shown.value = { username: r.user.username, password: r.password }
    username.value = ''
    admin.value = false
  })

function toggle(u: UserDoc) {
  if (!u.disabled && !window.confirm(`Disable ${u.username}? Their sessions end now.`)) return
  return run(async () => {
    await setDisabled(u.id, !u.disabled)
  })
}

function reset(u: UserDoc) {
  if (!window.confirm(`Reset the password of ${u.username}? Their sessions end now.`)) return
  return run(async () => {
    shown.value = { username: u.username, password: await resetPassword(u.id) }
  })
}

const addGrant = (u: UserDoc) =>
  run(async () => {
    const d = drafts[u.id]
    await createGrant(u.id, d.lo.toUpperCase(), d.hi.toUpperCase(), d.access)
    drafts[u.id] = { lo: '', hi: '', access: 'read' }
  })

const removeGrant = (id: number) => run(() => deleteGrant(id))
</script>

<template>
  <fieldset class="panel">
  <legend><h1>Users</h1></legend>
  <p v-if="error" role="alert">{{ error }}</p>
  <section v-if="shown" class="password">
    <p>Password for {{ shown.username }}: <code>{{ shown.password }}</code></p>
    <p>Copy it now. It is not shown again.</p>
  </section>
  <table>
    <thead>
      <tr><th>Username</th><th>Admin</th><th>Disabled</th><th>Grants</th><th></th></tr>
    </thead>
    <tbody>
      <tr v-for="u in users" :key="u.id" :data-user="u.username">
        <td>{{ u.username }}</td>
        <td>{{ u.admin ? 'yes' : '' }}</td>
        <td>{{ u.disabled ? 'yes' : '' }}</td>
        <td>
          <ul>
            <li v-for="g in u.grants" :key="g.id">
              {{ g.lo }}-{{ g.hi }} {{ g.access }}
              <button type="button" class="delete-grant" @click="removeGrant(g.id)">Delete</button>
            </li>
          </ul>
          <form class="grant" @submit.prevent="addGrant(u)">
            <input v-model="drafts[u.id].lo" name="lo" placeholder="100" size="3" maxlength="3" required aria-label="From page" />
            <input v-model="drafts[u.id].hi" name="hi" placeholder="1FF" size="3" maxlength="3" required aria-label="To page" />
            <select v-model="drafts[u.id].access" name="access" aria-label="Access">
              <option value="read">read</option>
              <option value="write">write</option>
            </select>
            <button type="submit">Add grant</button>
          </form>
        </td>
        <td>
          <button v-if="u.id !== me?.id" type="button" class="toggle" @click="toggle(u)">{{ u.disabled ? 'Enable' : 'Disable' }}</button>
          <button type="button" class="reset" @click="reset(u)">Reset password</button>
        </td>
      </tr>
    </tbody>
  </table>
  <form class="create" @submit.prevent="create">
    <h2>Create user</h2>
    <label>Username <input v-model="username" name="username" required maxlength="64" /></label>
    <label><input v-model="admin" name="admin" type="checkbox" /> Admin</label>
    <button type="submit">Create</button>
  </form>
  </fieldset>
</template>
