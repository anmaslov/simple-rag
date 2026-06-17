<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

type Job = { id: number; status: string; mode: string; space_key: string; cql: string; pages_found: number; pages_indexed: number; pages_skipped: number; error_message: string; created_at: string }
const jobs = ref<Job[]>([])
const spaceKey = ref('')
const cql = ref('')
const loadingMode = ref('')

async function load() {
  const resp = await api<{ jobs: Job[] }>('/api/sync/status')
  jobs.value = resp.jobs
}
async function start(mode: string) {
  loadingMode.value = mode
  try {
    await api('/api/sync', { method: 'POST', body: JSON.stringify({ mode, space_key: spaceKey.value, cql: cql.value }) })
    await load()
  } finally {
    loadingMode.value = ''
  }
}

function modeLabel(mode: string) {
  const labels: Record<string, string> = {
    full: 'Configured roots',
    space: 'Space',
    cql: 'CQL',
    incremental: 'Incremental'
  }
  return labels[mode] || mode
}

function scopeLabel(job: Job) {
  if (job.space_key) return job.space_key
  if (job.cql) return job.cql
  if (job.mode === 'full') return 'CONFLUENCE_ROOT_PAGE_IDS'
  if (job.mode === 'incremental') return 'Changed in last 7 days'
  return '-'
}

function formatDate(value: string) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}
onMounted(load)
</script>

<template>
  <section class="panel">
    <div class="page-head">
      <div>
        <h2>Sync</h2>
        <p>Запускайте индексацию Confluence и отслеживайте последние jobs.</p>
      </div>
      <button class="ghost-button" @click="load">Обновить</button>
    </div>

    <div class="sync-actions">
      <section class="action-box primary-action">
        <div>
          <h3>Configured roots</h3>
          <p>Полная переиндексация корневых страниц из env и всех дочерних страниц.</p>
        </div>
        <button @click="start('full')" :disabled="!!loadingMode">
          {{ loadingMode === 'full' ? 'Запускаю...' : 'Переиндексировать' }}
        </button>
      </section>

      <section class="action-box">
        <div>
          <h3>Incremental</h3>
          <p>Забрать страницы, измененные за последние 7 дней.</p>
        </div>
        <button class="secondary-button" @click="start('incremental')" :disabled="!!loadingMode">
          {{ loadingMode === 'incremental' ? 'Запускаю...' : 'Запустить' }}
        </button>
      </section>

      <section class="action-box input-action">
        <label>
          <span>Space key</span>
          <input v-model="spaceKey" placeholder="Например: HR" />
        </label>
        <button class="secondary-button" @click="start('space')" :disabled="!!loadingMode || !spaceKey.trim()">
          {{ loadingMode === 'space' ? 'Запускаю...' : 'Sync space' }}
        </button>
      </section>

      <section class="action-box input-action wide">
        <label>
          <span>CQL query</span>
          <input v-model="cql" placeholder="type=page and ancestor=123456" />
        </label>
        <button class="secondary-button" @click="start('cql')" :disabled="!!loadingMode || !cql.trim()">
          {{ loadingMode === 'cql' ? 'Запускаю...' : 'Run CQL' }}
        </button>
      </section>
    </div>

    <div class="table-card">
      <table>
        <thead>
          <tr>
            <th>Job</th>
            <th>Status</th>
            <th>Mode</th>
            <th>Scope</th>
            <th class="num">Found</th>
            <th class="num">Indexed</th>
            <th class="num">Skipped</th>
            <th>Created</th>
            <th>Error</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="j in jobs" :key="j.id">
            <td class="mono">#{{ j.id }}</td>
            <td><span :class="['status-pill', j.status]">{{ j.status }}</span></td>
            <td>{{ modeLabel(j.mode) }}</td>
            <td class="scope-cell">{{ scopeLabel(j) }}</td>
            <td class="num">{{ j.pages_found }}</td>
            <td class="num good">{{ j.pages_indexed }}</td>
            <td class="num muted">{{ j.pages_skipped }}</td>
            <td>{{ formatDate(j.created_at) }}</td>
            <td class="error-cell">{{ j.error_message || '-' }}</td>
          </tr>
        </tbody>
      </table>
      <p v-if="!jobs.length" class="empty-state large">Jobs пока нет. Запустите индексацию выше.</p>
    </div>
  </section>
</template>
