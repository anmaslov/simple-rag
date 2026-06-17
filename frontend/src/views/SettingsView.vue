<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api } from '../api/client'

const settings = ref<Record<string, any>>({})
onMounted(async () => { settings.value = await api('/api/settings') })
</script>

<template>
  <section class="panel">
    <div class="page-head">
      <div>
        <h2>Settings</h2>
        <p>Текущая конфигурация backend, доступная фронту.</p>
      </div>
    </div>
    <dl>
      <template v-for="(value, key) in settings" :key="key">
        <dt>{{ key }}</dt>
        <dd>{{ Array.isArray(value) ? value.join(', ') : value }}</dd>
      </template>
    </dl>
  </section>
</template>
