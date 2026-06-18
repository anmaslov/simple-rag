<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type Connection, type SourceScope } from '../api/client'

type Section = 'connections' | 'sources' | 'jobs'
type SourceType = 'confluence' | 'gitlab'
type Job = {
  id: number
  status: string
  mode: string
  source_type: SourceType
  connection_id?: number
  scope_id?: number
  documents_found: number
  documents_indexed: number
  documents_skipped: number
  error_message?: string
  created_at: string
}
type RemoteSpace = { Key?: string; Name?: string; key?: string; name?: string }
type Project = { id: number; name: string; path_with_namespace: string; default_branch: string; web_url: string }
type GitRef = { name: string }

const activeSection = ref<Section>('connections')
const activeSource = ref<SourceType>('confluence')
const connections = ref<Connection[]>([])
const scopes = ref<SourceScope[]>([])
const jobs = ref<Job[]>([])
const error = ref('')
const notice = ref('')
const busy = ref('')
const loading = ref(true)
const editingId = ref<number | null>(null)
const showConnectionForm = ref(false)

const connectionForm = reactive({
  source_type: 'confluence' as SourceType,
  name: '',
  base_url: '',
  auth_type: 'bearer',
  username: '',
  token: '',
  skip_tls_verify: false
})
const pageForm = reactive({ connection_id: 0, page: '', include_children: true })
const spaceConnection = ref(0)
const remoteSpaces = ref<Array<{ key: string; name: string }>>([])
const selectedSpaces = ref<string[]>([])
const projectConnection = ref(0)
const projectInput = ref('')
const projects = ref<Project[]>([])
const selectedProject = ref<Project | null>(null)
const refs = ref<GitRef[]>([])
const selectedRef = ref('')

const confluenceConnections = computed(() => connections.value.filter((item) => item.source_type === 'confluence'))
const gitlabConnections = computed(() => connections.value.filter((item) => item.source_type === 'gitlab'))
const visibleConnections = computed(() => activeSource.value === 'confluence' ? confluenceConnections.value : gitlabConnections.value)
const visibleScopes = computed(() => scopes.value.filter((item) => item.source_type === activeSource.value))
const visibleJobs = computed(() => jobs.value.filter((item) => item.source_type === activeSource.value))
const connectionName = computed(() => new Map(connections.value.map((item) => [item.id, item.name])))

async function load(showSpinner = true) {
  if (showSpinner) loading.value = true
  error.value = ''
  try {
    const [connectionResp, scopeResp, jobResp] = await Promise.all([
      api<{ connections: Connection[] }>('/api/connections'),
      api<{ scopes: SourceScope[] }>('/api/scopes'),
      api<{ jobs: Job[] }>('/api/jobs')
    ])
    connections.value = connectionResp.connections || []
    scopes.value = scopeResp.scopes || []
    jobs.value = jobResp.jobs || []
    setDefaultConnections()
  } catch (e: any) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function setDefaultConnections() {
  if (!confluenceConnections.value.some((item) => item.id === pageForm.connection_id)) {
    pageForm.connection_id = confluenceConnections.value[0]?.id || 0
  }
  if (!confluenceConnections.value.some((item) => item.id === spaceConnection.value)) {
    spaceConnection.value = confluenceConnections.value[0]?.id || 0
  }
  if (!gitlabConnections.value.some((item) => item.id === projectConnection.value)) {
    projectConnection.value = gitlabConnections.value[0]?.id || 0
  }
}

function clearMessages() {
  error.value = ''
  notice.value = ''
}

function openCreateConnection(source: SourceType = activeSource.value) {
  clearMessages()
  editingId.value = null
  activeSection.value = 'connections'
  activeSource.value = source
  Object.assign(connectionForm, {
    source_type: source,
    name: '',
    base_url: '',
    auth_type: source === 'gitlab' ? 'token' : 'bearer',
    username: '',
    token: '',
    skip_tls_verify: false
  })
  showConnectionForm.value = true
}

function editConnection(item: Connection) {
  clearMessages()
  activeSection.value = 'connections'
  activeSource.value = item.source_type
  editingId.value = item.id
  Object.assign(connectionForm, {
    source_type: item.source_type,
    name: item.name,
    base_url: item.base_url,
    auth_type: item.auth_type,
    username: item.username || '',
    token: '',
    skip_tls_verify: item.skip_tls_verify
  })
  showConnectionForm.value = true
}

function closeConnectionForm() {
  editingId.value = null
  showConnectionForm.value = false
}

async function saveConnection() {
  busy.value = 'connection-save'
  clearMessages()
  try {
    const path = editingId.value ? `/api/connections/${editingId.value}` : '/api/connections'
    await api(path, { method: editingId.value ? 'PUT' : 'POST', body: JSON.stringify(connectionForm) })
    notice.value = editingId.value ? 'Подключение обновлено.' : 'Подключение создано.'
    closeConnectionForm()
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function testConnection(item: Connection) {
  busy.value = `connection-test-${item.id}`
  clearMessages()
  try {
    await api(`/api/connections/${item.id}/test`, { method: 'POST' })
    notice.value = `Подключение «${item.name}» работает.`
  } catch (e: any) {
    error.value = `Не удалось подключиться к «${item.name}»: ${e.message}`
  } finally {
    busy.value = ''
  }
}

async function deleteConnection(item: Connection) {
  if (!confirm(`Удалить подключение «${item.name}» вместе со всеми его источниками и документами?`)) return
  busy.value = `connection-delete-${item.id}`
  clearMessages()
  try {
    await api(`/api/connections/${item.id}`, { method: 'DELETE' })
    notice.value = `Подключение «${item.name}» удалено.`
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function addPage() {
  busy.value = 'page-add'
  clearMessages()
  try {
    await api('/api/scopes', {
      method: 'POST',
      body: JSON.stringify({ ...pageForm, source_type: 'confluence', scope_type: 'page', sync: true })
    })
    pageForm.page = ''
    notice.value = 'Страница добавлена. Первичная индексация поставлена в очередь.'
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function loadSpaces() {
  busy.value = 'spaces-load'
  clearMessages()
  remoteSpaces.value = []
  selectedSpaces.value = []
  try {
    const resp = await api<{ spaces: RemoteSpace[] }>(`/api/connections/${spaceConnection.value}/confluence/spaces`)
    remoteSpaces.value = (resp.spaces || [])
      .map((item) => ({ key: item.key || item.Key || '', name: item.name || item.Name || '' }))
      .filter((item) => item.key)
    if (!remoteSpaces.value.length) notice.value = 'Confluence не вернул доступных пространств.'
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function addSpaces() {
  busy.value = 'spaces-add'
  clearMessages()
  try {
    for (const key of selectedSpaces.value) {
      const item = remoteSpaces.value.find((space) => space.key === key)
      await api('/api/scopes', {
        method: 'POST',
        body: JSON.stringify({
          connection_id: spaceConnection.value,
          source_type: 'confluence',
          scope_type: 'space',
          space_key: key,
          name: item?.name || key,
          sync: true
        })
      })
    }
    notice.value = `Добавлено пространств: ${selectedSpaces.value.length}. Индексация запущена.`
    selectedSpaces.value = []
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function searchProjects() {
  busy.value = 'projects-search'
  clearMessages()
  projects.value = []
  selectedProject.value = null
  refs.value = []
  if (!projectConnection.value) {
    error.value = 'Сначала выберите GitLab-подключение.'
    busy.value = ''
    return
  }
  try {
    const resp = await api<{ projects: Project[] }>(
      `/api/connections/${projectConnection.value}/gitlab/projects?q=${encodeURIComponent(projectInput.value.trim())}`
    )
    projects.value = resp.projects || []
    if (!projects.value.length) notice.value = 'Проекты не найдены. Можно добавить проект по полному URL или namespace/project.'
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function chooseProject(project: Project) {
  busy.value = `project-select-${project.id}`
  clearMessages()
  selectedProject.value = project
  selectedRef.value = project.default_branch
  try {
    const [branches, tags] = await Promise.all([
      api<{ refs: GitRef[] }>(`/api/connections/${projectConnection.value}/gitlab/branches?project_id=${project.id}`),
      api<{ refs: GitRef[] }>(`/api/connections/${projectConnection.value}/gitlab/tags?project_id=${project.id}`)
    ])
    const unique = new Map<string, GitRef>()
    for (const item of [...(branches.refs || []), ...(tags.refs || [])]) unique.set(item.name, item)
    refs.value = [...unique.values()]
    if (!selectedRef.value) selectedRef.value = refs.value[0]?.name || ''
  } catch (e: any) {
    selectedProject.value = null
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function addRepository() {
  busy.value = 'repository-add'
  clearMessages()
  try {
    const payload: Record<string, unknown> = {
      connection_id: projectConnection.value,
      source_type: 'gitlab',
      scope_type: 'repository',
      ref: selectedRef.value,
      sync: true
    }
    if (selectedProject.value) payload.project_id = selectedProject.value.id
    else payload.project = projectInput.value.trim()
    await api('/api/scopes', { method: 'POST', body: JSON.stringify(payload) })
    notice.value = 'Репозиторий добавлен. Первичная индексация поставлена в очередь.'
    projectInput.value = ''
    selectedProject.value = null
    selectedRef.value = ''
    projects.value = []
    refs.value = []
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function syncScope(item: SourceScope, force = false) {
  busy.value = `scope-${force ? 'reindex' : 'sync'}-${item.id}`
  clearMessages()
  try {
    await api(`/api/scopes/${item.id}/sync`, {
      method: 'POST',
      body: JSON.stringify({ mode: force ? 'full' : 'incremental', force })
    })
    notice.value = force ? `Переиндексация «${item.name}» запущена.` : `Синхронизация «${item.name}» запущена.`
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function deleteScope(item: SourceScope) {
  if (!confirm(`Удалить источник «${item.name}» и все его документы из индекса?`)) return
  busy.value = `scope-delete-${item.id}`
  clearMessages()
  try {
    await api(`/api/scopes/${item.id}`, { method: 'DELETE' })
    notice.value = `Источник «${item.name}» удалён.`
    await load(false)
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

function switchSection(section: Section) {
  activeSection.value = section
  clearMessages()
}

function switchSource(source: SourceType) {
  activeSource.value = source
  clearMessages()
}

function formatDate(value?: string) {
  return value
    ? new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value))
    : 'Ещё не запускалась'
}

function scopeTypeLabel(type: string) {
  return ({ page: 'Страница', space: 'Пространство', repository: 'Репозиторий' } as Record<string, string>)[type] || type
}

onMounted(load)
</script>

<template>
  <section class="panel sources-page">
    <div class="page-head">
      <div>
        <h2>Источники знаний</h2>
        <p>Сначала настройте подключения, затем добавьте страницы, пространства или репозитории.</p>
      </div>
      <button class="ghost-button" :disabled="loading" @click="load()">
        {{ loading ? 'Обновляю…' : 'Обновить' }}
      </button>
    </div>

    <nav class="section-tabs" aria-label="Разделы источников">
      <button :class="{ active: activeSection === 'connections' }" @click="switchSection('connections')">
        <span>1</span> Подключения
      </button>
      <button :class="{ active: activeSection === 'sources' }" @click="switchSection('sources')">
        <span>2</span> Индексируемые источники
      </button>
      <button :class="{ active: activeSection === 'jobs' }" @click="switchSection('jobs')">
        <span>3</span> Задачи синхронизации
      </button>
    </nav>

    <p v-if="notice" class="alert success">{{ notice }}</p>
    <p v-if="error" class="alert error">{{ error }}</p>

    <div v-if="loading" class="empty-state large">Загружаю настройки источников…</div>

    <template v-else>
      <section v-if="activeSection === 'connections'" class="source-section">
        <div class="section-intro">
          <div>
            <h3>Подключения</h3>
            <p>Здесь хранятся адреса и credentials внешних систем. Токены после сохранения не показываются.</p>
          </div>
          <button @click="openCreateConnection(activeSource)">Добавить подключение</button>
        </div>

        <div class="source-switch" aria-label="Тип подключения">
          <button :class="{ active: activeSource === 'confluence' }" @click="switchSource('confluence')">Confluence</button>
          <button :class="{ active: activeSource === 'gitlab' }" @click="switchSource('gitlab')">GitLab</button>
        </div>

        <div class="connection-grid">
          <article v-for="item in visibleConnections" :key="item.id" class="connection-card">
            <div class="connection-title">
              <span :class="['source-badge', item.source_type]">{{ item.source_type }}</span>
              <strong>{{ item.name }}</strong>
            </div>
            <span class="connection-url">{{ item.base_url || 'Адрес не задан — подключение нужно отредактировать' }}</span>
            <dl class="connection-facts">
              <div><dt>Авторизация</dt><dd>{{ item.auth_type }}</dd></div>
              <div><dt>Секрет</dt><dd>{{ item.has_token ? 'Сохранён' : 'Не задан' }}</dd></div>
              <div><dt>TLS</dt><dd>{{ item.skip_tls_verify ? 'Проверка отключена ⚠' : 'Проверяется' }}</dd></div>
            </dl>
            <p v-if="!item.base_url || !item.has_token" class="inline-warning">
              Подключение неполное. Нажмите «Изменить» и заполните адрес и token.
            </p>
            <div class="card-actions">
              <button
                class="secondary-button"
                :disabled="busy === `connection-test-${item.id}` || !item.base_url || !item.has_token"
                @click="testConnection(item)"
              >
                {{ busy === `connection-test-${item.id}` ? 'Проверяю…' : 'Проверить' }}
              </button>
              <button class="secondary-button" @click="editConnection(item)">Изменить</button>
              <button
                class="danger-button"
                :disabled="busy === `connection-delete-${item.id}`"
                @click="deleteConnection(item)"
              >
                {{ busy === `connection-delete-${item.id}` ? 'Удаляю…' : 'Удалить' }}
              </button>
            </div>
          </article>
          <div v-if="!visibleConnections.length" class="empty-state large">
            <p>Нет подключений {{ activeSource === 'confluence' ? 'Confluence' : 'GitLab' }}.</p>
            <button @click="openCreateConnection(activeSource)">Добавить первое подключение</button>
          </div>
        </div>

        <form v-if="showConnectionForm" class="form-card connection-editor" @submit.prevent="saveConnection">
          <div class="form-heading">
            <div>
              <h4>{{ editingId ? 'Изменить подключение' : 'Новое подключение' }}</h4>
              <p>{{ connectionForm.source_type === 'confluence' ? 'Confluence Server / Data Center' : 'Self-hosted GitLab' }}</p>
            </div>
            <button type="button" class="ghost-button" @click="closeConnectionForm">Закрыть</button>
          </div>
          <div class="form-grid">
            <label><span>Название</span><input v-model="connectionForm.name" required placeholder="Например, Корпоративный Confluence" /></label>
            <label><span>Base URL</span><input v-model="connectionForm.base_url" required placeholder="https://knowledge.company.local" /></label>
            <label v-if="connectionForm.source_type === 'confluence'">
              <span>Тип авторизации</span>
              <select v-model="connectionForm.auth_type"><option value="bearer">Bearer token</option><option value="basic">Basic auth</option></select>
            </label>
            <label v-if="connectionForm.source_type === 'confluence' && connectionForm.auth_type === 'basic'">
              <span>Username</span><input v-model="connectionForm.username" required placeholder="Логин Confluence" />
            </label>
            <label>
              <span>{{ connectionForm.source_type === 'gitlab' ? 'Access token' : 'Token / password' }}</span>
              <input
                v-model="connectionForm.token"
                :required="!editingId"
                type="password"
                autocomplete="new-password"
                :placeholder="editingId ? 'Оставьте пустым, чтобы сохранить текущий' : 'Секрет не будет показан повторно'"
              />
            </label>
          </div>
          <label class="tls-warning">
            <input v-model="connectionForm.skip_tls_verify" type="checkbox" />
            <span><strong>Отключить проверку TLS-сертификата</strong><small>Используйте только для доверенного self-signed сертификата.</small></span>
          </label>
          <div class="card-actions">
            <button :disabled="busy === 'connection-save'">{{ busy === 'connection-save' ? 'Сохраняю…' : 'Сохранить' }}</button>
            <button type="button" class="secondary-button" @click="closeConnectionForm">Отмена</button>
          </div>
        </form>
      </section>

      <section v-else-if="activeSection === 'sources'" class="source-section">
        <div class="section-intro">
          <div>
            <h3>Индексируемые источники</h3>
            <p>Это конкретные страницы, пространства и репозитории, по которым выполняются Search и Chat.</p>
          </div>
        </div>

        <div class="source-switch" aria-label="Тип индексируемого источника">
          <button :class="{ active: activeSource === 'confluence' }" @click="switchSource('confluence')">Confluence</button>
          <button :class="{ active: activeSource === 'gitlab' }" @click="switchSource('gitlab')">GitLab</button>
        </div>

        <template v-if="activeSource === 'confluence'">
          <div v-if="!confluenceConnections.length" class="empty-state large">
            Сначала добавьте подключение Confluence на вкладке «Подключения».
          </div>
          <div v-else class="source-forms">
            <form class="form-card" @submit.prevent="addPage">
              <div><h4>Добавить страницу</h4><p>Можно указать numeric ID или полный URL страницы.</p></div>
              <label><span>Подключение</span><select v-model.number="pageForm.connection_id" required><option v-for="item in confluenceConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
              <label><span>Страница</span><input v-model="pageForm.page" required placeholder="123456 или https://…/pages/123456/…" /></label>
              <label class="check-option"><input v-model="pageForm.include_children" type="checkbox" /><span>Также индексировать все дочерние страницы</span></label>
              <button :disabled="busy === 'page-add'">{{ busy === 'page-add' ? 'Добавляю…' : 'Добавить страницу' }}</button>
            </form>

            <div class="form-card">
              <div><h4>Добавить пространства</h4><p>Загрузите доступные пространства и выберите нужные.</p></div>
              <label><span>Подключение</span><select v-model.number="spaceConnection"><option v-for="item in confluenceConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
              <button class="secondary-button" :disabled="!spaceConnection || busy === 'spaces-load'" @click="loadSpaces">
                {{ busy === 'spaces-load' ? 'Загружаю…' : 'Загрузить пространства' }}
              </button>
              <div v-if="remoteSpaces.length" class="selection-list">
                <label v-for="item in remoteSpaces" :key="item.key" class="check-option">
                  <input v-model="selectedSpaces" type="checkbox" :value="item.key" />
                  <span><strong>{{ item.name }}</strong><small>{{ item.key }}</small></span>
                </label>
              </div>
              <button v-if="remoteSpaces.length" :disabled="!selectedSpaces.length || busy === 'spaces-add'" @click="addSpaces">
                {{ busy === 'spaces-add' ? 'Добавляю…' : `Добавить выбранные (${selectedSpaces.length})` }}
              </button>
            </div>
          </div>
        </template>

        <template v-else>
          <div v-if="!gitlabConnections.length" class="empty-state large">
            Сначала добавьте подключение GitLab на вкладке «Подключения».
          </div>
          <div v-else class="form-card repository-form">
            <div><h4>Добавить репозиторий</h4><p>Найдите проект или сразу укажите полный URL / namespace/project.</p></div>
            <label><span>Подключение</span><select v-model.number="projectConnection"><option v-for="item in gitlabConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
            <label><span>Проект</span><input v-model="projectInput" placeholder="https://gitlab.company/team/app или team/app" /></label>
            <div class="inline-fields">
              <button class="secondary-button" :disabled="!projectConnection || busy === 'projects-search'" @click="searchProjects">
                {{ busy === 'projects-search' ? 'Ищу…' : 'Найти через GitLab API' }}
              </button>
              <button
                :disabled="!projectConnection || !projectInput.trim() || busy === 'repository-add'"
                @click="addRepository"
              >
                {{ busy === 'repository-add' ? 'Добавляю…' : 'Добавить по URL / пути' }}
              </button>
            </div>
            <div v-if="projects.length" class="project-list">
              <button
                v-for="item in projects"
                :key="item.id"
                class="project-option"
                :disabled="busy === `project-select-${item.id}`"
                @click="chooseProject(item)"
              >
                <strong>{{ item.path_with_namespace }}</strong><small>{{ item.default_branch || 'без default branch' }}</small>
              </button>
            </div>
            <div v-if="selectedProject" class="selected-project">
              <p>Проект: <strong>{{ selectedProject.path_with_namespace }}</strong></p>
              <label><span>Branch / tag</span><select v-model="selectedRef"><option v-for="item in refs" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
              <button :disabled="!selectedRef || busy === 'repository-add'" @click="addRepository">
                {{ busy === 'repository-add' ? 'Добавляю…' : 'Добавить выбранный проект' }}
              </button>
            </div>
          </div>
        </template>

        <div class="scope-list">
          <article v-for="item in visibleScopes" :key="item.id" class="scope-card">
            <div class="scope-card-main">
              <span :class="['source-badge', item.source_type]">{{ scopeTypeLabel(item.scope_type) }}</span>
              <div><strong>{{ item.name }}</strong><small>{{ connectionName.get(item.connection_id) || `Connection #${item.connection_id}` }}</small></div>
            </div>
            <div class="scope-sync"><span>Последняя синхронизация</span><strong>{{ formatDate(item.last_synced_at) }}</strong></div>
            <div class="card-actions">
              <button class="secondary-button" :disabled="busy === `scope-sync-${item.id}`" @click="syncScope(item)">
                {{ busy === `scope-sync-${item.id}` ? 'Запускаю…' : 'Синхронизировать' }}
              </button>
              <button class="secondary-button" :disabled="busy === `scope-reindex-${item.id}`" @click="syncScope(item, true)">
                {{ busy === `scope-reindex-${item.id}` ? 'Запускаю…' : 'Переиндексировать' }}
              </button>
              <button class="danger-button" :disabled="busy === `scope-delete-${item.id}`" @click="deleteScope(item)">
                {{ busy === `scope-delete-${item.id}` ? 'Удаляю…' : 'Удалить' }}
              </button>
            </div>
          </article>
          <div v-if="!visibleScopes.length" class="empty-state large">
            Индексируемые источники {{ activeSource === 'confluence' ? 'Confluence' : 'GitLab' }} ещё не добавлены.
          </div>
        </div>
      </section>

      <section v-else class="source-section">
        <div class="section-intro">
          <div><h3>Задачи синхронизации</h3><p>История первичной индексации, обновлений и принудительной переиндексации.</p></div>
        </div>
        <div class="source-switch" aria-label="Фильтр задач">
          <button :class="{ active: activeSource === 'confluence' }" @click="switchSource('confluence')">Confluence</button>
          <button :class="{ active: activeSource === 'gitlab' }" @click="switchSource('gitlab')">GitLab</button>
        </div>
        <div class="table-card">
          <table>
            <thead><tr><th>Job</th><th>Статус</th><th>Режим</th><th>Источник</th><th>Найдено</th><th>Обновлено</th><th>Пропущено</th><th>Создана</th><th>Ошибка</th></tr></thead>
            <tbody>
              <tr v-for="job in visibleJobs" :key="job.id">
                <td class="mono">#{{ job.id }}</td>
                <td><span :class="['status-pill', job.status]">{{ job.status }}</span></td>
                <td>{{ job.mode }}</td>
                <td>{{ job.scope_id ? `Scope #${job.scope_id}` : 'Удалённый источник' }}</td>
                <td>{{ job.documents_found }}</td>
                <td class="good">{{ job.documents_indexed }}</td>
                <td>{{ job.documents_skipped }}</td>
                <td>{{ formatDate(job.created_at) }}</td>
                <td class="error-cell">{{ job.error_message || '—' }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="!visibleJobs.length" class="empty-state large">Задач для этого типа источника пока нет.</p>
        </div>
      </section>
    </template>
  </section>
</template>
