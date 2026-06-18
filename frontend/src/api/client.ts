const base = ''

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(base + path, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) }
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  return res.json()
}

export type SearchResult = {
  document_id: number
  page_id?: number
  external_id: string
  confluence_id?: string
  source_type: 'confluence' | 'gitlab'
  connection_id: number
  scope_id: number
  source_label: string
  title: string
  url: string
  space_key?: string
  repository?: string
  ref?: string
  file_path?: string
  chunk: string
  score: number
}

export type Connection = {
  id: number
  source_type: 'confluence' | 'gitlab'
  name: string
  base_url: string
  auth_type: 'bearer' | 'basic' | 'token'
  username?: string
  has_token: boolean
  skip_tls_verify: boolean
}

export type SourceScope = {
  id: number
  connection_id: number
  source_type: 'confluence' | 'gitlab'
  scope_type: 'space' | 'page' | 'repository'
  external_id: string
  name: string
  config: Record<string, unknown>
  enabled: boolean
  last_synced_at?: string
}

export type SearchScope = {
  source_types: string[]
  connection_ids: number[]
  scope_ids: number[]
}
