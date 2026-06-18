<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { api, type Connection, type SourceScope } from '../api/client'

type Job = {
  id: number; status: string; mode: string; source_type: string; connection_id?: number; scope_id?: number;
  documents_found: number; documents_indexed: number; documents_skipped: number; error_message?: string; created_at: string
}
type RemoteSpace = { Key?: string; Name?: string; key?: string; name?: string }
type Project = { id: number; name: string; path_with_namespace: string; default_branch: string; web_url: string }
type GitRef = { name: string }

const connections = ref<Connection[]>([])
const scopes = ref<SourceScope[]>([])
const jobs = ref<Job[]>([])
const error = ref('')
const notice = ref('')
const busy = ref('')
const editingId = ref<number | null>(null)
const connectionForm = reactive({
  source_type: 'confluence' as 'confluence' | 'gitlab', name: '', base_url: '', auth_type: 'bearer',
  username: '', token: '', skip_tls_verify: false
})
const pageForm = reactive({ connection_id: 0, page: '', include_children: true })
const spaceConnection = ref(0)
const remoteSpaces = ref<Array<{ key: string; name: string }>>([])
const selectedSpaces = ref<string[]>([])
const projectConnection = ref(0)
const projectQuery = ref('')
const projects = ref<Project[]>([])
const selectedProject = ref<Project | null>(null)
const refs = ref<GitRef[]>([])
const selectedRef = ref('')

const confluenceConnections = computed(() => connections.value.filter((item) => item.source_type === 'confluence'))
const gitlabConnections = computed(() => connections.value.filter((item) => item.source_type === 'gitlab'))

async function load() {
  error.value = ''
  const [c, s, j] = await Promise.all([
    api<{ connections: Connection[] }>('/api/connections'),
    api<{ scopes: SourceScope[] }>('/api/scopes'),
    api<{ jobs: Job[] }>('/api/sync/status')
  ])
  connections.value = c.connections || []
  scopes.value = s.scopes || []
  jobs.value = j.jobs || []
  if (!pageForm.connection_id && confluenceConnections.value[0]) pageForm.connection_id = confluenceConnections.value[0].id
  if (!spaceConnection.value && confluenceConnections.value[0]) spaceConnection.value = confluenceConnections.value[0].id
  if (!projectConnection.value && gitlabConnections.value[0]) projectConnection.value = gitlabConnections.value[0].id
}

function resetConnectionForm(source: 'confluence' | 'gitlab' = connectionForm.source_type) {
  editingId.value = null
  Object.assign(connectionForm, {
    source_type: source, name: '', base_url: '', auth_type: source === 'gitlab' ? 'token' : 'bearer',
    username: '', token: '', skip_tls_verify: false
  })
}

function editConnection(item: Connection) {
  editingId.value = item.id
  Object.assign(connectionForm, {
    source_type: item.source_type, name: item.name, base_url: item.base_url, auth_type: item.auth_type,
    username: item.username || '', token: '', skip_tls_verify: item.skip_tls_verify
  })
}

async function saveConnection() {
  busy.value = 'connection'
  error.value = ''
  try {
    const path = editingId.value ? `/api/connections/${editingId.value}` : '/api/connections'
    await api(path, { method: editingId.value ? 'PUT' : 'POST', body: JSON.stringify(connectionForm) })
    notice.value = editingId.value ? 'Подключение обновлено' : 'Подключение создано'
    resetConnectionForm()
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function testConnection(item: Connection) {
  busy.value = `test-${item.id}`
  try {
    await api(`/api/connections/${item.id}/test`, { method: 'POST' })
    notice.value = `${item.name}: соединение успешно`
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function deleteConnection(item: Connection) {
  if (!confirm(`Удалить подключение «${item.name}» вместе со scopes и индексом?`)) return
  await api(`/api/connections/${item.id}`, { method: 'DELETE' })
  await load()
}

async function addPage() {
  busy.value = 'page'
  try {
    await api('/api/scopes', {
      method: 'POST',
      body: JSON.stringify({ ...pageForm, source_type: 'confluence', scope_type: 'page', sync: true })
    })
    pageForm.page = ''
    notice.value = 'Страница добавлена, индексация поставлена в очередь'
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function loadSpaces() {
  busy.value = 'spaces'
  try {
    const resp = await api<{ spaces: RemoteSpace[] }>(`/api/connections/${spaceConnection.value}/confluence/spaces`)
    remoteSpaces.value = (resp.spaces || []).map((item) => ({ key: item.key || item.Key || '', name: item.name || item.Name || '' }))
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function addSpaces() {
  busy.value = 'add-spaces'
  try {
    for (const key of selectedSpaces.value) {
      const item = remoteSpaces.value.find((space) => space.key === key)
      await api('/api/scopes', {
        method: 'POST',
        body: JSON.stringify({ connection_id: spaceConnection.value, source_type: 'confluence', scope_type: 'space', space_key: key, name: item?.name || key, sync: true })
      })
    }
    selectedSpaces.value = []
    notice.value = 'Пространства добавлены, первичная индексация запущена'
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function searchProjects() {
  busy.value = 'projects'
  try {
    const resp = await api<{ projects: Project[] }>(`/api/connections/${projectConnection.value}/gitlab/projects?q=${encodeURIComponent(projectQuery.value)}`)
    projects.value = resp.projects || []
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function chooseProject(project: Project) {
  selectedProject.value = project
  selectedRef.value = project.default_branch
  const [branches, tags] = await Promise.all([
    api<{ refs: GitRef[] }>(`/api/connections/${projectConnection.value}/gitlab/branches?project_id=${project.id}`),
    api<{ refs: GitRef[] }>(`/api/connections/${projectConnection.value}/gitlab/tags?project_id=${project.id}`)
  ])
  refs.value = [...(branches.refs || []), ...(tags.refs || [])]
}

async function addRepository() {
  if (!selectedProject.value) return
  busy.value = 'repository'
  try {
    await api('/api/scopes', {
      method: 'POST',
      body: JSON.stringify({
        connection_id: projectConnection.value, source_type: 'gitlab', scope_type: 'repository',
        project_id: selectedProject.value.id, ref: selectedRef.value, sync: true
      })
    })
    notice.value = 'Репозиторий добавлен, индексация поставлена в очередь'
    selectedProject.value = null
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function syncScope(item: SourceScope, force = false) {
  busy.value = `sync-${item.id}`
  try {
    await api(`/api/scopes/${item.id}/sync`, { method: 'POST', body: JSON.stringify({ mode: force ? 'full' : 'incremental', force }) })
    await load()
  } catch (e: any) {
    error.value = e.message
  } finally {
    busy.value = ''
  }
}

async function deleteScope(item: SourceScope) {
  if (!confirm(`Удалить scope «${item.name}» и все его документы из индекса?`)) return
  await api(`/api/scopes/${item.id}`, { method: 'DELETE' })
  await load()
}

function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) : '—'
}

onMounted(load)
</script>

<template>
  <section class="panel sources-page">
    <div class="page-head">
      <div><h2>Sources</h2><p>Управление подключениями, областями индексации и фоновыми задачами.</p></div>
      <button class="ghost-button" @click="load">Обновить</button>
    </div>
    <p v-if="notice" class="alert success">{{ notice }}</p>
    <p v-if="error" class="alert error">{{ error }}</p>

    <section class="source-section">
      <div class="section-head"><div><span class="source-badge confluence">Confluence</span><h3>Подключения и scopes</h3></div></div>
      <div class="connection-grid">
        <article v-for="item in confluenceConnections" :key="item.id" class="connection-card">
          <strong>{{ item.name }}</strong><span>{{ item.base_url }}</span>
          <small>{{ item.auth_type }} · token {{ item.has_token ? 'сохранён' : 'не задан' }} · TLS {{ item.skip_tls_verify ? 'без проверки ⚠' : 'проверяется' }}</small>
          <div class="card-actions"><button class="secondary-button" @click="testConnection(item)">Проверить</button><button class="secondary-button" @click="editConnection(item)">Изменить</button><button class="danger-button" @click="deleteConnection(item)">Удалить</button></div>
        </article>
        <p v-if="!confluenceConnections.length" class="empty-state large">Подключения Confluence ещё не добавлены.</p>
      </div>

      <div class="source-forms">
        <form class="form-card" @submit.prevent="addPage">
          <h4>Добавить страницу</h4>
          <select v-model.number="pageForm.connection_id" required><option :value="0" disabled>Подключение</option><option v-for="item in confluenceConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select>
          <input v-model="pageForm.page" required placeholder="ID или полный URL страницы" />
          <label class="check-option"><input v-model="pageForm.include_children" type="checkbox" /><span>Индексировать дочерние страницы</span></label>
          <button :disabled="busy === 'page'">Добавить и индексировать</button>
        </form>
        <div class="form-card">
          <h4>Добавить пространства</h4>
          <select v-model.number="spaceConnection"><option :value="0" disabled>Подключение</option><option v-for="item in confluenceConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select>
          <button class="secondary-button" :disabled="!spaceConnection || busy === 'spaces'" @click="loadSpaces">Загрузить пространства</button>
          <div class="space-checks">
            <label v-for="item in remoteSpaces" :key="item.key" class="check-option"><input v-model="selectedSpaces" type="checkbox" :value="item.key" /><span>{{ item.name }} <small>{{ item.key }}</small></span></label>
          </div>
          <button :disabled="!selectedSpaces.length || busy === 'add-spaces'" @click="addSpaces">Добавить выбранные</button>
        </div>
      </div>
    </section>

    <section class="source-section">
      <div class="section-head"><div><span class="source-badge gitlab">GitLab</span><h3>Подключения и репозитории</h3></div></div>
      <div class="connection-grid">
        <article v-for="item in gitlabConnections" :key="item.id" class="connection-card">
          <strong>{{ item.name }}</strong><span>{{ item.base_url }}</span>
          <small>access token {{ item.has_token ? 'сохранён' : 'не задан' }} · TLS {{ item.skip_tls_verify ? 'без проверки ⚠' : 'проверяется' }}</small>
          <div class="card-actions"><button class="secondary-button" @click="testConnection(item)">Проверить</button><button class="secondary-button" @click="editConnection(item)">Изменить</button><button class="danger-button" @click="deleteConnection(item)">Удалить</button></div>
        </article>
        <p v-if="!gitlabConnections.length" class="empty-state large">Подключения GitLab ещё не добавлены.</p>
      </div>
      <div class="form-card repository-form">
        <h4>Добавить репозиторий</h4>
        <select v-model.number="projectConnection"><option :value="0" disabled>GitLab-подключение</option><option v-for="item in gitlabConnections" :key="item.id" :value="item.id">{{ item.name }}</option></select>
        <div class="inline-fields"><input v-model="projectQuery" placeholder="Название или namespace/project" /><button class="secondary-button" @click="searchProjects">Найти</button></div>
        <div class="project-list"><button v-for="item in projects" :key="item.id" class="project-option" @click="chooseProject(item)"><strong>{{ item.path_with_namespace }}</strong><small>{{ item.default_branch || 'без default branch' }}</small></button></div>
        <template v-if="selectedProject">
          <p>Выбран: <strong>{{ selectedProject.path_with_namespace }}</strong></p>
          <select v-model="selectedRef"><option v-for="item in refs" :key="item.name" :value="item.name">{{ item.name }}</option></select>
          <button :disabled="!selectedRef || busy === 'repository'" @click="addRepository">Добавить и индексировать</button>
        </template>
      </div>
    </section>

    <section class="source-section">
      <div class="section-head"><h3>Добавленные scopes</h3></div>
      <div class="table-card"><table><thead><tr><th>Источник</th><th>Scope</th><th>Тип</th><th>Последняя sync</th><th>Действия</th></tr></thead>
        <tbody><tr v-for="item in scopes" :key="item.id"><td><span :class="['source-badge', item.source_type]">{{ item.source_type }}</span></td><td>{{ item.name }}</td><td>{{ item.scope_type }}</td><td>{{ formatDate(item.last_synced_at) }}</td><td><div class="card-actions"><button class="secondary-button" @click="syncScope(item)">Sync</button><button class="secondary-button" @click="syncScope(item, true)">Reindex</button><button class="danger-button" @click="deleteScope(item)">Удалить</button></div></td></tr></tbody>
      </table><p v-if="!scopes.length" class="empty-state large">Scopes ещё не добавлены.</p></div>
    </section>

    <section class="source-section">
      <div class="section-head"><h3>Добавить или изменить подключение</h3></div>
      <form class="connection-form form-card" @submit.prevent="saveConnection">
        <select v-model="connectionForm.source_type" :disabled="editingId !== null" @change="resetConnectionForm(connectionForm.source_type)"><option value="confluence">Confluence</option><option value="gitlab">GitLab</option></select>
        <input v-model="connectionForm.name" required placeholder="Название" />
        <input v-model="connectionForm.base_url" required placeholder="https://knowledge.company.local" />
        <select v-if="connectionForm.source_type === 'confluence'" v-model="connectionForm.auth_type"><option value="bearer">Bearer</option><option value="basic">Basic</option></select>
        <input v-if="connectionForm.source_type === 'confluence' && connectionForm.auth_type === 'basic'" v-model="connectionForm.username" required placeholder="Username" />
        <input v-model="connectionForm.token" :required="!editingId" type="password" autocomplete="new-password" :placeholder="editingId ? 'Новый token (оставьте пустым, чтобы сохранить текущий)' : 'Token / password'" />
        <label class="tls-warning"><input v-model="connectionForm.skip_tls_verify" type="checkbox" /><span><strong>Отключить проверку TLS-сертификата</strong><small>Только для self-signed сертификатов. Это снижает защиту соединения.</small></span></label>
        <div class="card-actions"><button :disabled="busy === 'connection'">{{ editingId ? 'Сохранить' : 'Создать' }}</button><button v-if="editingId" type="button" class="secondary-button" @click="resetConnectionForm()">Отмена</button></div>
      </form>
    </section>

    <section class="source-section">
      <div class="section-head"><h3>Фоновые jobs</h3></div>
      <div class="table-card"><table><thead><tr><th>Job</th><th>Источник</th><th>Status</th><th>Mode</th><th>Found</th><th>Indexed</th><th>Skipped</th><th>Created</th><th>Error</th></tr></thead>
        <tbody><tr v-for="job in jobs" :key="job.id"><td class="mono">#{{ job.id }}</td><td><span :class="['source-badge', job.source_type]">{{ job.source_type }}</span></td><td><span :class="['status-pill', job.status]">{{ job.status }}</span></td><td>{{ job.mode }}</td><td>{{ job.documents_found }}</td><td class="good">{{ job.documents_indexed }}</td><td>{{ job.documents_skipped }}</td><td>{{ formatDate(job.created_at) }}</td><td>{{ job.error_message || '—' }}</td></tr></tbody>
      </table></div>
    </section>
  </section>
</template>
