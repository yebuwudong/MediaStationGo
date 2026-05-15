import { api } from './client'

export const strmAPI = {
  set: (mediaID: string, url: string) =>
    api.put(`/media/${mediaID}/strm`, { url }).then((r) => r.data),
  clear: (mediaID: string) => api.delete(`/media/${mediaID}/strm`).then((r) => r.data),
  importURL: (libraryID: string, title: string, url: string) =>
    api.post('/strm/import', { library_id: libraryID, title, url }).then((r) => r.data),
}
