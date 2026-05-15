import { api } from './client'

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  modified: number
}

export interface FileListing {
  path: string
  parent?: string
  roots?: { label: string; path: string }[]
  entries: FileEntry[] | null
}

export const filesAPI = {
  list: (path = '', max = 1000) =>
    api
      .get<FileListing>('/files', { params: { path, max } })
      .then((r) => r.data),
}
