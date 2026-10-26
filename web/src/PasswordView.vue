<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { changePassword } from './api'
import { user } from './session'

const router = useRouter()
const current = ref('')
const next = ref('')
const confirm = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  if (next.value !== confirm.value) {
    error.value = 'The new passwords do not match.'
    return
  }
  busy.value = true
  try {
    await changePassword(current.value, next.value)
    // The server has ended every session of this user, this one included.
    user.value = null
    await router.push({ path: '/login', query: { changed: '1' } })
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form @submit.prevent="submit">
    <fieldset class="dialog">
    <legend><h2>Change password</h2></legend>
    <label>Current password <input v-model="current" name="current" type="password" autocomplete="current-password" required /></label>
    <label>New password <input v-model="next" name="new" type="password" autocomplete="new-password" required minlength="12" maxlength="72" /></label>
    <label>Confirm new password <input v-model="confirm" name="confirm" type="password" autocomplete="new-password" required /></label>
    <button type="submit" :disabled="busy">Change password</button>
    <p v-if="error" role="alert">{{ error }}</p>
    </fieldset>
  </form>
</template>
