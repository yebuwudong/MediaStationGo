import { api } from './client'

export interface LibraryUsage {
  library_id: string
  name: string
  type: string
  path: string
  media_count: number
  total_bytes: number
  total_seconds: number
}

export interface ContainerStat {
  container: string
  count: number
  bytes: number
}

export interface StorageBreakdown {
  total_bytes: number
  total_seconds: number
  by_library: LibraryUsage[]
  by_container: ContainerStat[]
}

export const storageAPI = {
  breakdown: () => api.get<StorageBreakdown>('/storage').then((r) => r.data),
}
