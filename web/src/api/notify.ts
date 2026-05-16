import { api } from './client'
import type { NotifyChannel, NotifyProviderInfo } from '../types'

export interface NotifyChannelCreateParams {
  name: string
  type: 'telegram' | 'wechat' | 'bark' | 'webhook' | 'email'
  enabled?: boolean
  config: Record<string, string>
  events?: string[]
}

export interface NotifyChannelUpdateParams {
  name?: string
  enabled?: boolean
  config?: Record<string, string>
  events?: string[]
}

export const notifyAPI = {
  list: () =>
    api
      .get<{ code: number; data: NotifyChannel[] }>('/notify-channels')
      .then((r) => r.data.data),

  get: (id: string) =>
    api
      .get<{ code: number; data: NotifyChannel }>(`/notify-channels/${id}`)
      .then((r) => r.data.data),

  getTypes: () =>
    api
      .get<{ code: number; data: NotifyProviderInfo[] }>('/notify-channels/types')
      .then((r) => r.data.data),

  create: (params: NotifyChannelCreateParams) =>
    api
      .post<{ code: number; data: NotifyChannel }>('/notify-channels', params)
      .then((r) => r.data.data),

  update: (id: string, params: NotifyChannelUpdateParams) =>
    api
      .put<{ code: number; data: NotifyChannel }>(`/notify-channels/${id}`, params)
      .then((r) => r.data.data),

  delete: (id: string) =>
    api.delete(`/notify-channels/${id}`).then((r) => r.data),

  test: (id: string) =>
    api.post(`/notify-channels/${id}/test`).then((r) => r.data),
}
