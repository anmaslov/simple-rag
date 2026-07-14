<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type SearchResult, type SearchScope } from '../api/client'
import ScopePicker from '../components/ScopePicker.vue'

const message = ref('')
const loading = ref(false)
const sessionId = ref('')
type ChatMessage = { role: string; content: string; status?: string; sources?: SearchResult[] }
type ChatSession = { id: string; title: string; created_at: string; updated_at: string }
const messages = ref<ChatMessage[]>([])
const sessions = ref<ChatSession[]>([])
const loadingHistory = ref(false)
const activeStream = ref<AbortController | null>(null)
const scope = ref<SearchScope>({ source_types: [], connection_ids: [], scope_ids: [] })

type StreamEvent = {
  type: 'status' | 'sources' | 'session' | 'delta' | 'done' | 'error'
  message?: string
  session_id?: string
  delta?: string
  sources?: SearchResult[]
}

async function send() {
  if (!message.value.trim()) return
  if (loading.value) return
  const text = message.value
  messages.value.push({ role: 'user', content: text })
  const assistantIndex = messages.value.length
  messages.value.push({ role: 'assistant', content: '', status: 'Ищу подходящие материалы' })
  message.value = ''
  loading.value = true
  const controller = new AbortController()
  activeStream.value = controller
  try {
    await streamChat({
      session_id: sessionId.value || undefined,
      message: text,
      scope: scope.value
    }, controller.signal, (event) => {
      const assistant = messages.value[assistantIndex]
      if (!assistant) return
      if (event.type === 'status') {
        assistant.status = friendlyStatus(event.message || '')
      } else if (event.type === 'sources') {
        assistant.status = event.sources?.length ? 'Нашел источники, формулирую ответ' : ''
        assistant.sources = event.sources || []
      } else if (event.type === 'session' && event.session_id) {
        sessionId.value = event.session_id
      } else if (event.type === 'delta') {
        assistant.status = ''
        assistant.content += event.delta || ''
      } else if (event.type === 'done') {
        assistant.status = ''
      } else if (event.type === 'error') {
        assistant.status = ''
        assistant.content += `\n\nОшибка: ${event.message || 'stream failed'}`
      }
    })
  } catch (e: any) {
    const assistant = messages.value[assistantIndex]
    if (!assistant) return
    assistant.status = ''
    if (e?.name === 'AbortError') {
      assistant.content = assistant.content.trim() || 'Запрос остановлен'
      return
    }
    assistant.content = e.message
  } finally {
    if (activeStream.value === controller) {
      activeStream.value = null
    }
    loading.value = false
    await loadSessions()
  }
}

function stopGeneration() {
  activeStream.value?.abort()
}

function newChat() {
  sessionId.value = ''
  messages.value = []
}

async function loadSessions() {
  loadingHistory.value = true
  try {
    const resp = await api<{ sessions: ChatSession[] }>('/api/chat/sessions')
    sessions.value = resp.sessions || []
  } finally {
    loadingHistory.value = false
  }
}

async function openSession(id: string) {
  if (loading.value) return
  const resp = await api<{ messages: Array<{ role: string; content: string; sources?: SearchResult[] }> }>(`/api/chat/sessions/${id}/messages`)
  sessionId.value = id
  messages.value = resp.messages.map((m) => ({ role: m.role, content: m.content, sources: m.sources || [] }))
}

async function deleteSession(id: string) {
  if (loading.value) return
  await api(`/api/chat/sessions/${id}`, { method: 'DELETE' })
  sessions.value = sessions.value.filter((s) => s.id !== id)
  if (sessionId.value === id) {
    newChat()
  }
}

async function streamChat(payload: Record<string, unknown>, signal: AbortSignal, onEvent: (event: StreamEvent) => void) {
  const resp = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal
    })
  if (!resp.ok || !resp.body) {
    const body = await resp.json().catch(() => ({}))
    throw new Error(body.error || `HTTP ${resp.status}`)
  }
  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')
    const events = buffer.split('\n\n')
    buffer = events.pop() || ''
    for (const raw of events) {
      const event = parseSSE(raw)
      if (event) onEvent(event)
    }
  }
  const tail = parseSSE(buffer)
  if (tail) onEvent(tail)
}

function parseSSE(raw: string): StreamEvent | null {
  const data = raw
    .split('\n')
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trimStart())
    .join('\n')
  if (!data) return null
  try {
    return JSON.parse(data) as StreamEvent
  } catch {
    return null
  }
}

function formatSessionTime(value: string) {
  return new Intl.DateTimeFormat('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onMounted(loadSessions)

function friendlyStatus(value: string) {
  if (value.includes('Передал контекст') || value.includes('жду первый токен')) {
    return 'Готовлю ответ по найденным материалам'
  }
  if (value.includes('Ищу релевантные')) return 'Ищу подходящие материалы'
  return value
}

function displayContent(m: ChatMessage) {
  if (!m.sources?.length) return m.content
  return stripSourceSection(m.content)
}

function stripSourceSection(value: string) {
  return value
    .replace(/\n{0,2}(?:#{1,6}\s*)?Источники:\s*[\s\S]*$/i, '')
    .replace(/\n{0,2}(?:#{1,6}\s*)?Список источников:\s*[\s\S]*$/i, '')
    .trim()
}

function uniqueSources(sources: SearchResult[] = []) {
  const seen = new Set<string>()
  return sources.filter((source) => {
    const key = source.url || `${source.title}:${source.scope_id}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function sourceHost(url: string) {
  try {
    return new URL(url).host
  } catch {
    return ''
  }
}

function sourceKind(source: SearchResult) {
  return source.source_type === 'gitlab' ? 'GitLab' : 'Confluence'
}

function sourceTitle(source: SearchResult) {
  if (source.source_type === 'gitlab') {
    return source.repository || source.source_label || `GitLab scope #${source.scope_id}`
  }
  return source.space_key || source.source_label || `Confluence scope #${source.scope_id}`
}

function sourceDetail(source: SearchResult) {
  const parts = []
  if (source.source_type === 'gitlab') {
    if (source.ref) parts.push(source.ref)
    if (source.file_path) parts.push(source.file_path)
  } else {
    if (source.title) parts.push(source.title)
    if (source.confluence_id || source.external_id) parts.push(source.confluence_id || source.external_id)
  }
  const host = sourceHost(source.url)
  if (host) parts.push(host)
  return parts.join(' · ')
}

function renderMarkdown(value: string) {
  const lines = value.replace(/\r\n/g, '\n').split('\n')
  const html: string[] = []
  let paragraph: string[] = []
  let listType: 'ol' | 'ul' | '' = ''
  let listItems: string[] = []

  function flushParagraph() {
    if (!paragraph.length) return
    html.push(`<p>${renderInline(paragraph.map(escapeHtml).join('<br>'))}</p>`)
    paragraph = []
  }

  function flushList() {
    if (!listType) return
    html.push(`<${listType}>${listItems.join('')}</${listType}>`)
    listType = ''
    listItems = []
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const trimmed = line.trim()

    if (trimmed.startsWith('```')) {
      flushParagraph()
      flushList()
      const code: string[] = []
      i++
      while (i < lines.length && !lines[i].trim().startsWith('```')) {
        code.push(lines[i])
        i++
      }
      html.push(`<pre><code>${escapeHtml(code.join('\n'))}</code></pre>`)
      continue
    }

    if (!trimmed) {
      flushParagraph()
      flushList()
      continue
    }

    if (/^-{3,}$/.test(trimmed)) {
      flushParagraph()
      flushList()
      html.push('<hr>')
      continue
    }

    const heading = trimmed.match(/^(#{1,6})\s+(.+)$/)
    if (heading) {
      flushParagraph()
      flushList()
      const level = heading[1].length
      html.push(`<h${level}>${renderInline(escapeHtml(heading[2]))}</h${level}>`)
      continue
    }

    const ordered = line.match(/^\s*\d+\.\s+(.+)$/)
    if (ordered) {
      flushParagraph()
      if (listType && listType !== 'ol') flushList()
      listType = 'ol'
      listItems.push(`<li>${renderInline(escapeHtml(ordered[1]))}</li>`)
      continue
    }

    const unordered = line.match(/^\s*[-*]\s+(.+)$/)
    if (unordered) {
      flushParagraph()
      if (listType && listType !== 'ul') flushList()
      listType = 'ul'
      listItems.push(`<li>${renderInline(escapeHtml(unordered[1]))}</li>`)
      continue
    }

    flushList()
    paragraph.push(line)
  }

  flushParagraph()
  flushList()
  return html.join('')
}

function renderInline(value: string) {
  return value
    .replace(/\[\[([^\]]+)\](?:,\s*\[([^\]]+)\])*\]/g, (match: string) => {
      return match.replace(/^\[\[|\]\]$/g, '').replace(/\[|\]/g, '')
    })
    .replace(/\[([^\]]+)]\((https?:\/\/[^)\s]+)\)/g, (_match, label: string, url: string) => {
      return `<a href="${escapeAttr(url)}" target="_blank" rel="noreferrer">${label}</a>`
    })
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
}

function escapeHtml(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function escapeAttr(value: string) {
  return value.replace(/"/g, '&quot;')
}
</script>

<template>
  <section class="chat-layout">
    <aside class="chat-history">
      <div class="chat-history-head">
        <h2>Chat</h2>
        <button @click="newChat" :disabled="loading">New</button>
      </div>
      <div class="chat-history-list">
        <div
          v-for="s in sessions"
          :key="s.id"
          :class="['chat-session', { active: s.id === sessionId }]"
          @click="openSession(s.id)"
        >
          <button class="chat-session-open" :disabled="loading">
            <span>{{ s.title || 'Untitled chat' }}</span>
            <small>{{ formatSessionTime(s.updated_at) }}</small>
          </button>
          <button class="chat-session-delete" :disabled="loading" title="Delete chat" @click.stop="deleteSession(s.id)">×</button>
        </div>
        <p v-if="!sessions.length && !loadingHistory" class="empty-state">No chats yet</p>
      </div>
    </aside>

    <section class="chat-main">
      <header class="chat-topbar">
        <div>
          <span class="eyebrow">Knowledge assistant</span>
          <h2>{{ sessionId ? 'Диалог по базе знаний' : 'Новый диалог' }}</h2>
        </div>
        <div class="chat-mode-pill">RAG</div>
      </header>
      <div class="messages">
        <div v-if="!messages.length" class="chat-empty">
          <span class="chat-empty-mark">?</span>
          <h3>Спросите что-нибудь по индексам</h3>
          <p>Поиск выполняется по проиндексированным документам из выбранных источников.</p>
        </div>
        <article v-for="(m, i) in messages" :key="i" :class="['message', m.role]">
          <div class="message-avatar">{{ m.role === 'user' ? 'Вы' : 'AI' }}</div>
          <div class="message-card">
            <div class="message-meta">
              <span>{{ m.role === 'user' ? 'Вы' : 'Ассистент' }}</span>
              <small v-if="m.role === 'assistant'">RAG</small>
            </div>
            <div v-if="m.status" class="message-status">{{ m.status }}</div>
            <div class="message-body" v-html="renderMarkdown(displayContent(m))" />
            <details v-if="m.sources?.length" class="sources-panel">
              <summary class="sources-head">
                <span>Источники</span>
                <small>{{ uniqueSources(m.sources).length }}</small>
              </summary>
              <div class="sources-list">
                <a
                  v-for="(s, idx) in uniqueSources(m.sources)"
                  :key="s.url || `${s.title}-${idx}`"
                  :href="s.url"
                  target="_blank"
                  rel="noreferrer"
                  class="source-item"
                >
                  <span :class="['source-origin', s.source_type]">{{ sourceKind(s) }}</span>
                  <span class="source-text">
                    <strong>{{ sourceTitle(s) }}</strong>
                    <small>
                      {{ sourceDetail(s) }}
                    </small>
                  </span>
                </a>
              </div>
            </details>
          </div>
        </article>
      </div>
      <div class="chat-controls">
        <ScopePicker v-model="scope" />
      </div>
      <form class="composer" @submit.prevent="send">
        <input v-model="message" placeholder="Спросите по выбранным проиндексированным источникам" />
        <div class="composer-actions">
          <button v-if="loading" type="button" class="stop-button" @click="stopGeneration">Стоп</button>
          <button :disabled="loading">{{ loading ? 'Отвечаю...' : 'Отправить' }}</button>
        </div>
      </form>
    </section>
  </section>
</template>
