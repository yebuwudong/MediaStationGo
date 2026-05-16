import { api } from './client'
import type { Media } from '../types'

// TMDb-derived "Match" rows used by trending/popular rails. We re-use the
// Media interface — only TMDb id / poster / overview are populated.
export interface DiscoverItem extends Partial<Media> {
  tmdb_id: number
  title: string
  overview: string
  poster_url: string
  backdrop_url: string
  year: number
  rating: number
}

export const discoverAPI = {
  trending: () =>
    api.get<{ items: DiscoverItem[] }>('/discover/trending').then((r) => r.data.items ?? []),
  popular: () =>
    api.get<{ items: DiscoverItem[] }>('/discover/popular').then((r) => r.data.items ?? []),
}
