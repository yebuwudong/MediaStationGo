import { api } from './client'
import type { Media } from '../types'

export interface SearchIntent {
  query: string
  year?: number
  genre?: string
  type?: string
  sort?: string
  language?: string
}

export const aiAPI = {
  status: () =>
    api
      .get<{ enabled: boolean; provider: string; model: string }>('/ai/status')
      .then((r) => r.data),

  smartSearch: (query: string) =>
    api
      .post<{ intent: SearchIntent; items: Media[] }>('/ai/search', { query })
      .then((r) => r.data),

  recommend: () => api.get<{ titles: string[] }>('/ai/recommend').then((r) => r.data.titles),
}
