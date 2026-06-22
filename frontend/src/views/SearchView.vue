<script setup lang="ts">
import { ref } from 'vue'
import { api, type SearchResult, type SearchScope } from '../api/client'
import ScopePicker from '../components/ScopePicker.vue'

const query = ref('')
const loading = ref(false)
const results = ref<SearchResult[]>([])
const scope = ref<SearchScope>({ source_types: [], connection_ids: [], scope_ids: [] })

async function search() {
  if (!query.value.trim()) return
  loading.value = true
  try {
    const resp = await api<{ results: SearchResult[] }>('/api/search', {
      method: 'POST',
      body: JSON.stringify({ query: query.value, scope: scope.value })
    })
    results.value = resp.results
  } finally {
    loading.value = false
  }
}

function formatScore(value: number) {
  return value.toFixed(3)
}
</script>

<template>
  <section class="panel">
    <div class="page-head">
      <div>
        <h2>Search</h2>
        <p>Гибридный поиск по выбранным корпоративным источникам.</p>
      </div>
    </div>
    <ScopePicker v-model="scope" />
    <form class="search-bar" @submit.prevent="search">
      <input v-model="query" placeholder="Например: уведомление руководителя об отпуске" />
      <button :disabled="loading">{{ loading ? 'Ищу...' : 'Найти' }}</button>
    </form>
    <div class="results-list">
      <article v-for="r in results" :key="r.document_id + '-' + r.chunk" :class="['result', `source-${r.source_type}`]">
        <div class="result-head">
          <a :href="r.url" target="_blank" rel="noreferrer">{{ r.title }}</a>
          <span class="score">Score {{ formatScore(r.score) }}</span>
        </div>
        <span :class="['source-badge', r.source_type]">{{ r.source_type }}</span>
        <span class="meta">
          {{ r.source_label }}
          <template v-if="r.source_type === 'confluence'"> · {{ r.space_key }}</template>
          <template v-else> · {{ r.repository }} · {{ r.ref }} · {{ r.file_path }}</template>
        </span>
        <p>{{ r.chunk }}</p>
      </article>
      <p v-if="!results.length && !loading" class="empty-state large">Введите запрос, чтобы увидеть найденные чанки и страницы.</p>
    </div>
  </section>
</template>
