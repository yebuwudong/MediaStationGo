// 权限 API 模块
import { api } from './client'
import type { UserPermission } from '../types'

// 获取用户权限
export async function getUserPermissions(userId: string): Promise<UserPermission> {
  const resp = await api.get<UserPermission>(`/admin/users/${userId}/permissions`)
  return resp.data as unknown as UserPermission
}

// 更新用户权限
export async function updateUserPermissions(
  userId: string,
  permissions: Record<string, boolean>
): Promise<void> {
  await api.put(`/admin/users/${userId}/permissions`, { permissions })
}

// 重置用户权限为默认值
export async function resetUserPermissions(userId: string): Promise<void> {
  await api.post(`/admin/users/${userId}/permissions/reset`)
}

// 获取当前用户权限
export async function getMyPermissions(): Promise<{
  permissions: Record<string, boolean>
  role: string
  tier: string
  is_super: boolean
}> {
  const resp = await api.get('/auth/permissions')
  return resp.data as unknown as {
    permissions: Record<string, boolean>
    role: string
    tier: string
    is_super: boolean
  }
}
