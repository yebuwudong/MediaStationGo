import { api } from './client'
import type { User } from '../types'

export const profileAPI = {
  update: (patch: {
    username?: string
    nickname?: string
    email?: string
    avatar_url?: string
    hide_adult?: boolean
    password?: string
  }) =>
    api.patch<User>('/me', patch).then((r) => r.data),

  adminUpdateRole: (id: string, role: 'admin' | 'user') =>
    api.patch<User>(`/admin/users/${id}/role`, { role }).then((r) => r.data),
}
