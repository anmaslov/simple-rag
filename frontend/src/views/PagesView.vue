<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api, type SourceScope } from '../api/client'

type SourceType = '' | 'confluence' | 'gitlab'
type Freshness = '' | 'updated' | 'indexed' | 'stale'
type Document = {
  id: number
  title: string
  url: string
  source_type: 'confluence' | 'gitlab'
  scope_id: number
  external_id: string
  metadata: Record<string, unknown>
  source_updated_at?: string
  indexed_at?: string
}

const documents = ref<Document[]>([])
const scopes = ref<SourceScope[]>([])
const sourceType = ref<SourceType>('')
const locationFilter = ref('')
const freshness = ref<Freshness>('')
const q = ref('')
const page = ref(1)
const pageSize = ref(25)
const loading = ref(false)
const error = ref('')

const filteredDocuments = computed(() => {
  const location = locationFilter.value
  return documents.value.filter((item) => {
    if (location && documentLocation(item) !== location) return false
    if (freshness.value === 'updated' && !item.source_updated_at) return false
    if (freshness.value === 'indexed' && !item.indexed_at) return false
    if (freshness.value === 'stale' && (!item.source_updated_at || !item.indexed_at || new Date(item.source_updated_at) <= new Date(item.indexed_at))) return false
    return true
  })
})

const totalPages = computed(() => Math.max(1, Math.ceil(filteredDocuments.value.length / pageSize.value)))
const pageStart = computed(() => filteredDocuments.value.length ? (page.value - 1) * pageSize.value + 1 : 0)
const pageEnd = computed(() => Math.min(page.value * pageSize.value, filteredDocuments.value.length))
const visibleDocuments = computed(() => filteredDocuments.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const confluenceCount = computed(() => documents.value.filter((item) => item.source_type === 'confluence').length)
const gitlabCount = computed(() => documents.value.filter((item) => item.source_type === 'gitlab').length)
const indexedCount = computed(() => documents.value.filter((item) => item.indexed_at).length)
const scopeByID = computed(() => new Map(scopes.value.map((item) => [item.id, item])))
const locationOptions = computed(() => {
  const values = new Set<string>()
  for (const item of documents.value) {
    if (!sourceType.value || item.source_type === sourceType.value) values.add(documentLocation(item))
  }
  return [...values].filter(Boolean).sort((a, b) => a.localeCompare(b))
})

watch([sourceType, locationFilter, freshness, q, pageSize], () => {
  page.value = 1
})

watch(sourceType, () => {
  locationFilter.value = ''
})

watch(totalPages, (value) => {
  if (page.value > value) page.value = value
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const params = new URLSearchParams()
    if (sourceType.value) params.set('source_type', sourceType.value)
    if (q.value.trim()) params.set('q', q.value.trim())
    const [docResp, scopeResp] = await Promise.all([
      api<{ documents: Document[] }>(`/api/documents?${params}`),
      api<{ scopes: SourceScope[] }>('/api/scopes')
    ])
    documents.value = docResp.documents || []
    scopes.value = scopeResp.scopes || []
  } catch (e: any) {
    error.value = e?.message || 'Не удалось загрузить документы'
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  sourceType.value = ''
  locationFilter.value = ''
  freshness.value = ''
  q.value = ''
  pageSize.value = 25
  page.value = 1
  void load()
}

function goToPage(next: number) {
  page.value = Math.min(Math.max(next, 1), totalPages.value)
}

function documentLocation(item: Document) {
  const scope = scopeByID.value.get(item.scope_id)
  if (item.source_type === 'confluence') {
    const space = stringMeta(item, 'space_key') || scopeConfig(scope, 'space_key')
    if (scope?.scope_type === 'space') return space ? `${scope.name} (${space})` : scope.name
    if (scope?.name) return space ? `${scope.name} · ${space}` : scope.name
    return space || `Confluence scope #${item.scope_id}`
  }
  const repo = stringMeta(item, 'project_path') || stringMeta(item, 'repository') || scopeConfig(scope, 'project_path')
  return repo || scope?.name?.replace(/\s+@\s+.+$/, '') || `GitLab scope #${item.scope_id}`
}

function documentPath(item: Document) {
  if (item.source_type === 'confluence') return item.external_id
  return stringMeta(item, 'file_path') || item.external_id
}

function documentRef(item: Document) {
  if (item.source_type === 'gitlab') return stringMeta(item, 'ref') || scopeConfig(scopeByID.value.get(item.scope_id), 'ref')
  return ''
}

function documentTitle(item: Document) {
  if (item.title?.trim()) return item.title
  const path = documentPath(item)
  if (path) return path.split('/').filter(Boolean).pop() || path
  return item.source_type === 'gitlab' ? documentLocation(item) : item.external_id || documentLocation(item)
}

function documentScopeLabel(item: Document) {
  const scope = scopeByID.value.get(item.scope_id)
  if (!scope) return `#${item.scope_id}`
  return sourceLabelForScope(scope)
}

function stringMeta(item: Document, key: string) {
  const value = item.metadata?.[key]
  return typeof value === 'string' ? value : ''
}

function scopeConfig(item: SourceScope | undefined, key: string) {
  const value = item?.config?.[key]
  return typeof value === 'string' || typeof value === 'number' ? String(value) : ''
}

function sourceLabelForScope(item: SourceScope) {
  if (item.source_type === 'gitlab') {
    const project = scopeConfig(item, 'project_path') || item.name.replace(/\s+@\s+.+$/, '')
    const ref = scopeConfig(item, 'ref')
    return ref ? `${project} @ ${ref}` : project
  }
  const space = scopeConfig(item, 'space_key')
  if (item.scope_type === 'space' && space) return `${item.name} (${space})`
  return space ? `${item.name} · ${space}` : item.name
}

function formatDate(value?: string) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', year: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onMounted(load)
</script>

<template>
  <section class="panel documents-page">
    <div class="page-head">
      <div>
        <h2>Documents</h2>
        <p>Каталог того, что реально попало в локальный индекс: Confluence-страницы и файлы GitLab.</p>
      </div>
      <button type="button" class="secondary-button" :disabled="loading" @click="load">Обновить</button>
    </div>

    <div class="documents-summary">
      <article><span>Всего</span><strong>{{ documents.length }}</strong></article>
      <article><span>Confluence</span><strong>{{ confluenceCount }}</strong></article>
      <article><span>GitLab</span><strong>{{ gitlabCount }}</strong></article>
      <article><span>С индексом</span><strong>{{ indexedCount }}</strong></article>
    </div>

    <section class="documents-filters">
      <label>
        <span>Поиск</span>
        <input v-model="q" placeholder="Название, путь, URL, repository" @keyup.enter="load" />
      </label>
      <label>
        <span>Источник</span>
        <select v-model="sourceType" @change="load">
          <option value="">Все источники</option>
          <option value="confluence">Confluence</option>
          <option value="gitlab">GitLab</option>
        </select>
      </label>
      <label>
        <span>Пространство / репозиторий</span>
        <select v-model="locationFilter">
          <option value="">Все</option>
          <option v-for="item in locationOptions" :key="item" :value="item">{{ item }}</option>
        </select>
      </label>
      <label>
        <span>Состояние</span>
        <select v-model="freshness">
          <option value="">Любое</option>
          <option value="indexed">Есть indexed_at</option>
          <option value="updated">Есть updated_at источника</option>
          <option value="stale">Источник новее индекса</option>
        </select>
      </label>
      <div class="documents-filter-actions">
        <button type="button" :disabled="loading" @click="load">{{ loading ? 'Ищу...' : 'Найти' }}</button>
        <button type="button" class="secondary-button" :disabled="loading" @click="resetFilters">Сбросить</button>
      </div>
    </section>

    <p v-if="error" class="error-state">{{ error }}</p>

    <div class="documents-toolbar">
      <span>
        Показаны {{ pageStart }}-{{ pageEnd }} из {{ filteredDocuments.length }}
        <template v-if="documents.length !== filteredDocuments.length">, загружено {{ documents.length }}</template>
      </span>
      <label>
        <span>На странице</span>
        <select v-model.number="pageSize">
          <option :value="10">10</option>
          <option :value="25">25</option>
          <option :value="50">50</option>
          <option :value="100">100</option>
        </select>
      </label>
    </div>

    <div class="documents-list">
      <article v-for="item in visibleDocuments" :key="item.id" class="document-card">
        <div class="document-card-main">
          <a :href="item.url" target="_blank" rel="noreferrer">{{ documentTitle(item) }}</a>
          <p>
            <span :class="['source-badge', item.source_type]">{{ item.source_type }}</span>
            <strong>{{ documentLocation(item) }}</strong>
            <template v-if="documentRef(item)"> · {{ documentRef(item) }}</template>
            <template v-if="documentPath(item)"> · {{ documentPath(item) }}</template>
          </p>
        </div>
        <dl class="document-card-meta">
          <div><dt>Updated</dt><dd>{{ formatDate(item.source_updated_at) }}</dd></div>
          <div><dt>Indexed</dt><dd>{{ formatDate(item.indexed_at) }}</dd></div>
          <div><dt>Source</dt><dd :title="documentScopeLabel(item)">{{ documentScopeLabel(item) }}</dd></div>
        </dl>
      </article>
      <p v-if="!loading && !visibleDocuments.length" class="empty-state">Документы не найдены. Измените фильтры или синхронизируйте источники.</p>
    </div>

    <nav class="pagination" aria-label="Пагинация документов">
      <button type="button" class="secondary-button" :disabled="page <= 1" @click="goToPage(1)">В начало</button>
      <button type="button" class="secondary-button" :disabled="page <= 1" @click="goToPage(page - 1)">Назад</button>
      <span>Страница {{ page }} из {{ totalPages }}</span>
      <button type="button" class="secondary-button" :disabled="page >= totalPages" @click="goToPage(page + 1)">Вперёд</button>
      <button type="button" class="secondary-button" :disabled="page >= totalPages" @click="goToPage(totalPages)">В конец</button>
    </nav>
  </section>
</template>
