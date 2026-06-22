<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'

type Settings = {
  ai?: Record<string, unknown>
  indexing?: Record<string, unknown>
  search?: Record<string, unknown>
  security?: Record<string, unknown>
}

const settings = ref<Settings>({})
const loading = ref(true)
const error = ref('')

const sections = computed(() => [
  {
    key: 'ai',
    title: 'AI-модели',
    description: 'OpenAI-compatible endpoints для генерации embeddings и ответов.',
    values: settings.value.ai || {}
  },
  {
    key: 'indexing',
    title: 'Индексация',
    description: 'Размер чанков и ограничения внешних источников.',
    values: settings.value.indexing || {}
  },
  {
    key: 'search',
    title: 'Поиск',
    description: 'Количество результатов и веса hybrid search.',
    values: settings.value.search || {}
  },
  {
    key: 'security',
    title: 'Безопасность',
    description: 'Как приложение обращается с credentials и TLS.',
    values: settings.value.security || {}
  }
])

const labels: Record<string, string> = {
  llm_base_url: 'LLM endpoint',
  llm_model: 'LLM model',
  embeddings_base_url: 'Embeddings endpoint',
  embeddings_model: 'Embeddings model',
  embeddings_dim: 'Размерность embeddings',
  chunk_size: 'Размер чанка',
  chunk_overlap: 'Перекрытие чанков',
  source_page_limit: 'Документов за запрос к источнику',
  gitlab_max_file_bytes: 'Максимальный размер GitLab-файла',
  top_k: 'Результатов по умолчанию',
  vector_weight: 'Вес vector search',
  keyword_weight: 'Вес keyword search',
  source_credentials: 'Credentials источников',
  tls_verification: 'Проверка TLS'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    settings.value = await api<Settings>('/api/settings')
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function formatValue(key: string, value: unknown) {
  if (key === 'gitlab_max_file_bytes' && typeof value === 'number') {
    return `${(value / 1024 / 1024).toFixed(1)} MB`
  }
  if (Array.isArray(value)) return value.join(', ')
  return String(value ?? '—')
}

onMounted(load)
</script>

<template>
  <section class="panel settings-page">
    <div class="page-head">
      <div>
        <h2>Настройки приложения</h2>
        <p>Технические параметры индексации, поиска и AI-моделей.</p>
      </div>
      <button class="ghost-button" :disabled="loading" @click="load">{{ loading ? 'Обновляю…' : 'Обновить' }}</button>
    </div>

    <div class="settings-note">
      <strong>Подключения Confluence и GitLab находятся на странице Sources.</strong>
      <span>Здесь не отображаются и не редактируются токены, пароли и адреса корпоративных источников.</span>
      <RouterLink to="/sources">Перейти к источникам →</RouterLink>
    </div>

    <p v-if="error" class="alert error">{{ error }}</p>
    <div v-if="loading" class="empty-state large">Загружаю настройки…</div>

    <div v-else class="settings-grid">
      <article v-for="section in sections" :key="section.key" class="settings-card">
        <div class="settings-card-head">
          <h3>{{ section.title }}</h3>
          <p>{{ section.description }}</p>
        </div>
        <dl>
          <template v-for="(value, key) in section.values" :key="key">
            <dt>{{ labels[String(key)] || key }}</dt>
            <dd>{{ formatValue(String(key), value) }}</dd>
          </template>
        </dl>
      </article>
    </div>

    <p class="settings-footer">
      Эти параметры читаются из env при запуске backend. Изменение через UI намеренно недоступно.
    </p>
  </section>
</template>
