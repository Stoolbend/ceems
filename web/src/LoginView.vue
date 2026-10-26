<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { login } from './api'
import { user } from './session'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)

// localPath accepts only an app path: one leading '/', never '//' or '/\', which a
// browser reads as another host (KI-001). Anything else goes home.
function localPath(redirect: unknown): string {
  return typeof redirect === 'string' && /^\/(?![/\\])/.test(redirect) ? redirect : '/'
}

async function submit() {
  busy.value = true
  error.value = ''
  try {
    user.value = await login(username.value, password.value)
    await router.replace(localPath(route.query.redirect))
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
    <legend><h2>Log in</h2></legend>
    <p v-if="'changed' in route.query" role="status">Password changed; log in again</p>
    <label>Username <input v-model="username" name="username" autocomplete="username" required /></label>
    <label>Password <input v-model="password" name="password" type="password" autocomplete="current-password" required /></label>
    <button type="submit" :disabled="busy">Log in</button>
    <p v-if="error" role="alert">{{ error }}</p>
    </fieldset>
  </form>
</template>
