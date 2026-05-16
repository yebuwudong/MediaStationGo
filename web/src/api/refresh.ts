// 令牌刷新 API 模块
import { api } from './client'

// 刷新令牌请求/响应
export interface RefreshTokenRequest {
  refresh_token: string
}

export interface RefreshTokenResponse {
  token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

// 刷新访问令牌
export async function refreshToken(refreshToken: string): Promise<RefreshTokenResponse> {
  const resp = await api.post<RefreshTokenResponse>('/auth/refresh', {
    refresh_token: refreshToken,
  })
  return resp.data as unknown as RefreshTokenResponse
}

// 登出
export async function logout(): Promise<void> {
  await api.post('/me/logout')
}
