<script setup lang="ts">
import { ref } from 'vue'
import { api, type SearchResult } from '../api/client'

const query = ref('')
const loading = ref(false)
const results = ref<SearchResult[]>([])

async function search() {
  if (!query.value.trim()) return
  loading.value = true
  try {
    const resp = await api<{ results: SearchResult[] }>('/api/search', {
      method: 'POST',
      body: JSON.stringify({ query: query.value })
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
        <p>Быстрый поиск по проиндексированным страницам Confluence.</p>
      </div>
    </div>
    <form class="search-bar" @submit.prevent="search">
      <input v-model="query" placeholder="Например: уведомление руководителя об отпуске" />
      <button :disabled="loading">{{ loading ? 'Ищу...' : 'Найти' }}</button>
    </form>
    <div class="results-list">
      <article v-for="r in results" :key="r.page_id + '-' + r.chunk" class="result">
        <div class="result-head">
          <a :href="r.url" target="_blank" rel="noreferrer">{{ r.title }}</a>
          <span class="score">Score {{ formatScore(r.score) }}</span>
        </div>
        <span class="meta">{{ r.space_key }}</span>
        <p>{{ r.chunk }}</p>
      </article>
      <p v-if="!results.length && !loading" class="empty-state large">Введите запрос, чтобы увидеть найденные чанки и страницы.</p>
    </div>
  </section>
</template>
