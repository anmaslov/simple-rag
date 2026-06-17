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
  page_id: number
  confluence_id: string
  title: string
  url: string
  space_key: string
  chunk: string
  score: number
}
