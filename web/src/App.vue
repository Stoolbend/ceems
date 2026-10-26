<script setup lang="ts">
import { ref, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { logout } from './api'
import { loadSite, systemName, user } from './session'
import CreditsDialog from './CreditsDialog.vue'
import LoadingDialog from './LoadingDialog.vue'

const VERSION = import.meta.env.VITE_CEEMS_VERSION ?? 'dev'
const REPO_URL = 'https://github.com/Stoolbend/ceems'

void loadSite()
watchEffect(() => { document.title = systemName.value })

const credits = ref<InstanceType<typeof CreditsDialog>>()
const router = useRouter()
const route = useRoute()

async function doLogout() {
  try {
    await logout()
  } finally {
    user.value = null
    await router.push('/login')
  }
}

// shortcut: the scheme is also read in index.html before first paint; share a module if a third reader appears
const dark = ref(document.documentElement.dataset.scheme === 'dark')
function toggleScheme() {
  dark.value = !dark.value
  const root = document.documentElement
  if (dark.value) root.dataset.scheme = 'dark'
  else delete root.dataset.scheme
  try {
    if (dark.value) localStorage.setItem('ceems.scheme', 'dark')
    else localStorage.removeItem('ceems.scheme')
  } catch {
    // storage unavailable: the choice lasts until reload
  }
}
</script>

<template>
  <header>
    <nav class="container">
      <ul><li><h1>{{ systemName }}</h1></li></ul>
      <ul v-if="user">
        <li><RouterLink to="/">Pages</RouterLink></li>
        <template v-if="user.admin">
          <li><RouterLink to="/templates">Templates</RouterLink></li>
          <li><RouterLink to="/bindings">Bindings</RouterLink></li>
          <li><RouterLink to="/fastext">Fastext</RouterLink></li>
          <li><RouterLink to="/users">Users</RouterLink></li>
          <li><RouterLink to="/keys">Keys</RouterLink></li>
          <li><RouterLink to="/settings">Settings</RouterLink></li>
        </template>
      </ul>
      <ul v-if="user">
        <li><RouterLink to="/password" title="Change password">[{{ user.username }}]</RouterLink></li>
        <li><button type="button" @click="doLogout">Logout</button></li>
      </ul>
    </nav>
  </header>
  <main :class="route.meta.wide ? 'container-fluid' : 'container'"><RouterView /></main>
  <footer>
    <nav class="container" aria-label="Footer">
      <ul>
        <li><a :href="REPO_URL" target="_blank" rel="noopener">GitHub</a></li>
        <li><button type="button" @click="credits?.open()">Credits</button></li>
        <li v-if="user"><RouterLink to="/help">Help</RouterLink></li>
      </ul>
      <ul>
        <li>CeeMS {{ VERSION }}</li>
        <li>|</li>
        <li><button type="button" :aria-pressed="dark" @click="toggleScheme">Dark</button></li>
      </ul>
    </nav>
  </footer>
  <CreditsDialog ref="credits" />
  <LoadingDialog />
</template>

<style scoped>
/* the wide page editor fills the window between the header and the footer (PageView), so main has no vertical padding there */
@media (min-width: 1024px) { main.container-fluid { padding-block: 0; } }
</style>
