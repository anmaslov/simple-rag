<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api, type Connection, type SearchScope, type SourceScope } from '../api/client'

const props = defineProps<{ modelValue: SearchScope }>()
const emit = defineEmits<{ 'update:modelValue': [value: SearchScope] }>()
const tab = ref<'all' | 'confluence' | 'gitlab'>('all')
const connections = ref<Connection[]>([])
const scopes = ref<SourceScope[]>([])

const visibleConnections = computed(() => tab.value === 'all' ? connections.value : connections.value.filter((item) => item.source_type === tab.value))
const visibleScopes = computed(() => {
  const source = tab.value
  return scopes.value.filter((item) => (source === 'all' || item.source_type === source) &&
    (!props.modelValue.connection_ids.length || props.modelValue.connection_ids.includes(item.connection_id)))
})

async function load() {
  const [connectionResp, scopeResp] = await Promise.all([
    api<{ connections: Connection[] }>('/api/connections'),
    api<{ scopes: SourceScope[] }>('/api/scopes')
  ])
  connections.value = connectionResp.connections || []
  scopes.value = scopeResp.scopes || []
}

function setTab(value: typeof tab.value) {
  tab.value = value
  emit('update:modelValue', {
    source_types: value === 'all' ? [] : [value],
    connection_ids: [],
    scope_ids: []
  })
}

function toggleConnection(id: number) {
  const current = new Set(props.modelValue.connection_ids)
  current.has(id) ? current.delete(id) : current.add(id)
  emit('update:modelValue', { ...props.modelValue, connection_ids: [...current], scope_ids: [] })
}

function toggleScope(id: number) {
  const current = new Set(props.modelValue.scope_ids)
  current.has(id) ? current.delete(id) : current.add(id)
  emit('update:modelValue', { ...props.modelValue, scope_ids: [...current] })
}

watch(() => props.modelValue.source_types, (types) => {
  tab.value = types.length === 1 && (types[0] === 'confluence' || types[0] === 'gitlab') ? types[0] : 'all'
}, { immediate: true })

onMounted(load)
</script>

<template>
  <section class="scope-picker">
    <div class="segmented" aria-label="Область поиска">
      <button type="button" :class="{ active: tab === 'all' }" @click="setTab('all')">Все</button>
      <button type="button" :class="{ active: tab === 'confluence' }" @click="setTab('confluence')">Confluence</button>
      <button type="button" :class="{ active: tab === 'gitlab' }" @click="setTab('gitlab')">GitLab</button>
    </div>
    <div v-if="tab !== 'all'" class="scope-options">
      <div>
        <strong>Подключения</strong>
        <label v-for="item in visibleConnections" :key="item.id" class="check-option">
          <input type="checkbox" :checked="modelValue.connection_ids.includes(item.id)" @change="toggleConnection(item.id)" />
          <span>{{ item.name }}</span>
        </label>
        <small v-if="!visibleConnections.length">Нет подключений этого типа</small>
      </div>
      <div>
        <strong>{{ tab === 'confluence' ? 'Пространства и страницы' : 'Репозитории и refs' }}</strong>
        <label v-for="item in visibleScopes" :key="item.id" class="check-option">
          <input type="checkbox" :checked="modelValue.scope_ids.includes(item.id)" @change="toggleScope(item.id)" />
          <span>{{ item.name }}</span>
        </label>
        <small v-if="!visibleScopes.length">Нет подходящих scopes</small>
      </div>
    </div>
    <p class="scope-summary">
      Область: <strong>{{ tab === 'all' ? 'все проиндексированные источники' : tab }}</strong>
      <template v-if="modelValue.scope_ids.length"> · scopes: {{ modelValue.scope_ids.length }}</template>
    </p>
  </section>
</template>
