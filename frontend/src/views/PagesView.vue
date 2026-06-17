<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

type Page = { id: number; title: string; url: string; space_key: string; version: number; confluence_updated_at: string; indexed_at?: string }
const pages = ref<Page[]>([])
const space = ref('')
const q = ref('')

async function load() {
  const params = new URLSearchParams({ space_key: space.value, q: q.value })
  const resp = await api<{ pages: Page[] }>(`/api/pages?${params}`)
  pages.value = resp.pages
}
onMounted(load)
</script>

<template>
  <section class="panel">
    <div class="page-head">
      <div>
        <h2>Pages</h2>
        <p>Список страниц, которые уже есть в локальном индексе.</p>
      </div>
    </div>
    <div class="toolbar">
      <input v-model="space" placeholder="Space key" @change="load" />
      <input v-model="q" placeholder="Фильтр по заголовку" @input="load" />
    </div>
    <div class="table-card">
      <table>
        <thead><tr><th>Title</th><th>Space</th><th>Version</th><th>Updated</th><th>Indexed</th></tr></thead>
        <tbody>
          <tr v-for="p in pages" :key="p.id">
            <td><a :href="p.url" target="_blank" rel="noreferrer">{{ p.title }}</a></td>
            <td>{{ p.space_key }}</td>
            <td>{{ p.version }}</td>
            <td>{{ p.confluence_updated_at }}</td>
            <td>{{ p.indexed_at || '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
