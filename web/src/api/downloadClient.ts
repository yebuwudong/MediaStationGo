import { api } from './client'
import type { DownloadClient } from '../types'

export interface DownloadClientCreateParams {
  name: string
  type: 'qbittorrent' | 'transmission' | 'aria2'
  host: string
  username?: string
  password?: string
  is_default?: boolean
  extra?: Record<string, string>
}

export interface DownloadClientUpdateParams {
  name?: string
  type?: 'qbittorrent' | 'transmission' | 'aria2'
  host?: string
  username?: string
  password?: string
  is_default?: boolean
  enabled?: boolean
  extra?: Record<string, string>
}

export const downloadClientAPI = {
  list: () =>
    api
      .get<{ code: number; data: DownloadClient[] }>('/download-clients')
      .then((r) => r.data.data),

  get: (id: string) =>
    api
      .get<{ code: number; data: DownloadClient }>(`/download-clients/${id}`)
      .then((r) => r.data.data),

  create: (params: DownloadClientCreateParams) =>
    api
      .post<{ code: number; data: DownloadClient }>('/download-clients', params)
      .then((r) => r.data.data),

  update: (id: string, params: DownloadClientUpdateParams) =>
    api
      .put<{ code: number; data: DownloadClient }>(`/download-clients/${id}`, params)
      .then((r) => r.data.data),

  delete: (id: string) =>
    api.delete(`/download-clients/${id}`).then((r) => r.data),

  test: (id: string) =>
    api.post(`/download-clients/${id}/test`).then((r) => r.data),
}
