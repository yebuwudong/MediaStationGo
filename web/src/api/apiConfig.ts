// API 配置 API 模块
import { api } from './client'
import type { ApiConfig, ApiProvider } from '../types'

// 获取所有 API 配置
export async function listApiConfigs(): Promise<ApiConfig[]> {
  const resp = await api.get<ApiConfig[]>('/api-config')
  return resp.data as unknown as ApiConfig[]
}

// 获取提供者列表
export async function getProviders(): Promise<ApiProvider[]> {
  const resp = await api.get<ApiProvider[]>('/api-config/providers/list')
  return resp.data as unknown as ApiProvider[]
}

// 获取指定提供者的配置
export async function getApiConfig(provider: string): Promise<ApiConfig> {
  const resp = await api.get<ApiConfig>(`/api-config/${provider}`)
  return resp.data as unknown as ApiConfig
}

// 获取生效的配置
export async function getEffectiveConfig(provider: string): Promise<ApiConfig> {
  const resp = await api.get<ApiConfig>(`/api-config/${provider}/effective`)
  return resp.data as unknown as ApiConfig
}

// 更新 API 配置
export interface UpdateApiConfigRequest {
  api_key?: string
  base_url?: string
  extra?: string
  enabled?: boolean
}

export async function updateApiConfig(
  provider: string,
  data: UpdateApiConfigRequest
): Promise<ApiConfig> {
  const resp = await api.post<ApiConfig>(`/api-config/${provider}`, data)
  return resp.data as unknown as ApiConfig
}

// 删除 API 配置
export async function deleteApiConfig(provider: string): Promise<void> {
  await api.delete(`/api-config/${provider}`)
}

// 测试 API 连接
export interface TestApiConfigResponse {
  result: 'success' | 'error' | 'invalid' | 'unknown'
}

export async function testApiConfig(provider: string): Promise<TestApiConfigResponse> {
  const resp = await api.post<TestApiConfigResponse>(`/api-config/${provider}/test`)
  return resp.data as unknown as TestApiConfigResponse
}
