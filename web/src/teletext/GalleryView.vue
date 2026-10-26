<script setup lang="ts">
import { onMounted, ref } from 'vue'
import TeletextPage from './TeletextPage.vue'
import { DEFAULT_HEADER, headerRow } from './header'
import { testPages } from './testpages'
import { subpages } from './tti'

const urls = import.meta.glob('../../../testdata/sample/pages/*.tti', { query: '?url', import: 'default', eager: true }) as Record<string, string>

interface Entry { name: string; rows: string[] }
const fixtures = ref<Entry[]>([])
const aspect = ref<'4:3' | '16:9'>('4:3')
const failed = ref<string[]>([])
const now = new Date()

onMounted(async () => {
  const out: Entry[] = []
  for (const [path, url] of Object.entries(urls).sort()) {
    const file = path.split('/').pop()!.replace('.tti', '')
    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error(String(res.status))
      // latin1 so the seven-bit codes in high bytes survive (UTF-8 would merge them)
      const text = new TextDecoder('latin1').decode(await res.arrayBuffer())
      subpages(text).forEach((sp, i) => {
        const rows = sp.rows.map((r) => r ?? '')
        rows[0] = headerRow(DEFAULT_HEADER, sp.mpp, now)
        out.push({ name: `${file}-${i + 1}`, rows })
      })
    } catch {
      failed.value.push(file)
    }
  }
  fixtures.value = out
})
</script>

<template>
  <h1>Teletext gallery</h1>
  <fieldset role="group">
    <label><input v-model="aspect" type="radio" value="4:3" /> 4:3</label>
    <label><input v-model="aspect" type="radio" value="16:9" /> 16:9</label>
  </fieldset>
  <p v-if="failed.length">Failed to load: {{ failed.join(', ') }}</p>
  <h2>Control-code test pages</h2>
  <section v-for="(rows, name) in testPages" :key="name" class="entry">
    <h3>{{ name }}</h3>
    <TeletextPage :rows="rows" :aspect="aspect" />
  </section>
  <h2>Fixture subpages ({{ fixtures.length }})</h2>
  <section v-for="f in fixtures" :key="f.name" class="entry">
    <h3>{{ f.name }}</h3>
    <TeletextPage :rows="f.rows" :aspect="aspect" />
  </section>
</template>

<style scoped>
.entry { display: inline-block; vertical-align: top; margin: 0 16px 16px 0; width: 480px; max-width: 100%; }
.entry:has(.aspect-16-9) { width: 640px; }
</style>
