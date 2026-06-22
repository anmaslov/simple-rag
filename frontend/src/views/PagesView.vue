<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

type Document = { id: number; title: string; url: string; source_type: string; scope_id: number; external_id: string; metadata: Record<string, unknown>; source_updated_at?: string; indexed_at?: string }
const pages = ref<Document[]>([])
const sourceType = ref('')
const q = ref('')

async function load() {
  const params = new URLSearchParams({ source_type: sourceType.value, q: q.value })
  const resp = await api<{ documents: Document[] }>(`/api/documents?${params}`)
  pages.value = resp.documents
}
onMounted(load)
</script>

<template>
  <section class="panel">
    <div class="page-head">
      <div>
        <h2>Documents</h2>
        <p>Документы Confluence и GitLab, которые уже есть в локальном индексе.</p>
      </div>
    </div>
    <div class="toolbar">
      <select v-model="sourceType" @change="load"><option value="">Все источники</option><option value="confluence">Confluence</option><option value="gitlab">GitLab</option></select>
      <input v-model="q" placeholder="Фильтр по заголовку" @input="load" />
    </div>
    <div class="table-card">
      <table>
        <thead><tr><th>Title</th><th>Source</th><th>Location</th><th>Updated</th><th>Indexed</th></tr></thead>
        <tbody>
          <tr v-for="p in pages" :key="p.id">
            <td><a :href="p.url" target="_blank" rel="noreferrer">{{ p.title }}</a></td>
            <td><span :class="['source-badge', p.source_type]">{{ p.source_type }}</span></td>
            <td>{{ p.source_type === 'confluence' ? p.metadata.space_key : `${p.metadata.project_path} · ${p.metadata.ref} · ${p.metadata.file_path}` }}</td>
            <td>{{ p.source_updated_at || '-' }}</td>
            <td>{{ p.indexed_at || '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
