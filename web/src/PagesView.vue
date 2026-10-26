<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { createPage, listPages, putPage, type PageSummary } from './api'
import { decode } from './zxnet'

const router = useRouter()
const pages = ref<PageSummary[]>([])
const error = ref('')
const magazine = ref<number | null>(null) // null = all
const jump = ref('')
const jumpError = ref('')

const magazines = [1, 2, 3, 4, 5, 6, 7, 8]
const count = (m: number) => pages.value.filter((p) => p.mpp[0] === String(m)).length
const shown = computed(() => (magazine.value === null ? pages.value : pages.value.filter((p) => p.mpp[0] === String(magazine.value))))

onMounted(async () => {
  try {
    pages.value = await listPages()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
})

async function go() {
  const mpp = jump.value.trim().toUpperCase()
  if (!/^[1-8][0-9A-F]{2}$/.test(mpp)) {
    jumpError.value = 'Enter three hex digits, magazine 1 to 8 (for example 100 or 8FF).'
    return
  }
  jumpError.value = ''
  await router.push(`/pages/${mpp}`)
}

const newMpp = ref('')
const createError = ref('')

const newUrl = ref('')

async function create() {
  const url = newUrl.value.trim()
  const imported = url === '' ? null : decode(url)
  if (imported instanceof Error) {
    createError.value = imported.message
    return
  }
  const mpp = (newMpp.value.trim() || imported?.pn || '').toUpperCase()
  if (!/^[1-8][0-9A-F]{2}$/.test(mpp)) {
    createError.value = 'Enter three hex digits, magazine 1 to 8 (for example 100 or 8FF).'
    return
  }
  createError.value = ''
  let p
  try {
    p = await createPage(mpp)
  } catch (e) {
    createError.value = e instanceof Error ? e.message : String(e)
    return
  }
  if (imported) {
    try {
      const first = p.subpages[0]
      await putPage({ ...p, subpages: [{ ...first, rows: [first.rows[0], ...imported.rows.slice(1, 25)] }] })
    } catch (e) {
      createError.value = `Page ${mpp} was created blank; the import failed: ${e instanceof Error ? e.message : String(e)}`
      return
    }
  }
  await router.push(`/pages/${mpp}`)
}
</script>

<template>
  <fieldset class="panel">
  <legend><h1>Pages</h1></legend>
  <p v-if="error" role="alert">{{ error }}</p>
  <nav class="magazines">
    <button type="button" :aria-pressed="magazine === null" @click="magazine = null">All ({{ pages.length }})</button>
    <button v-for="m in magazines" :key="m" type="button" :aria-pressed="magazine === m" @click="magazine = m">{{ m }} ({{ count(m) }})</button>
  </nav>
  <form @submit.prevent="go">
    <fieldset role="group">
      <input v-model="jump" class="short" name="jump" maxlength="3" autocomplete="off" placeholder="Go to page" aria-label="Go to page" />
      <button type="submit">Go</button>
    </fieldset>
    <span v-if="jumpError" role="alert">{{ jumpError }}</span>
  </form>
  <form class="create" @submit.prevent="create">
    <fieldset role="group">
      <input v-model="newMpp" class="short" name="create" maxlength="3" autocomplete="off" placeholder="New blank page" aria-label="New blank page" />
      <input v-model="newUrl" name="createUrl" autocomplete="off" placeholder="zxnet URL (optional)" aria-label="zxnet URL (optional)" />
      <button type="submit">Create</button>
    </fieldset>
    <span v-if="createError" role="alert">{{ createError }}</span>
  </form>
  <table>
    <thead>
      <tr><th>Page</th><th>Description</th><th>Subpages</th><th>Updated</th><th>By</th></tr>
    </thead>
    <tbody>
      <tr v-for="p in shown" :key="p.mpp">
        <td><RouterLink :to="`/pages/${p.mpp}`">{{ p.mpp }}</RouterLink></td>
        <td>{{ p.description }}</td>
        <td>{{ p.subpages }}</td>
        <td>{{ p.updatedAt }}</td>
        <td>{{ p.updatedBy }}</td>
      </tr>
    </tbody>
  </table>
  </fieldset>
</template>

<style scoped>
.magazines { display: flex; flex-wrap: wrap; justify-content: flex-start; gap: 0.5rem; margin-bottom: 1rem; }
/* Page numbers are three characters; the URL input takes the remaining width. */
fieldset[role='group'] input.short { flex: 0 0 18ch; width: 18ch; }
</style>
