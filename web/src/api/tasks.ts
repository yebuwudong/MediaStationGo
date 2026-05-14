import { api } from './client'
import type { QBitTorrent } from '../types'

export interface ActiveTranscode {
  media_id: string
  encoder: string
  started_at: string
  playlist_ok: boolean
}

export interface TasksSnapshot {
  transcodes: ActiveTranscode[]
  torrents: QBitTorrent[] | null
}

export const tasksAPI = {
  snapshot: () => api.get<TasksSnapshot>('/tasks').then((r) => r.data),
}
