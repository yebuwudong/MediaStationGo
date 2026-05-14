import { api } from './client'
import type { Media } from '../types'

export const recycleAPI = {
  list: () => api.get<{ items: Media[] }>('/recycle').then((r) => r.data.items),

  softDelete: (id: string) => api.delete(`/media/${id}`).then((r) => r.data),

  restore: (id: string) => api.post(`/media/${id}/restore`).then((r) => r.data),

  purge: (id: string) => api.delete(`/media/${id}/purge`).then((r) => r.data),

  exportNFO: (id: string) =>
    api.post<{ path: string }>(`/media/${id}/nfo`).then((r) => r.data),

  exportLibraryNFO: (id: string) =>
    api.post<{ written: number }>(`/libraries/${id}/nfo`).then((r) => r.data),
}
