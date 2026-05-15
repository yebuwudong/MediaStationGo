import { api } from './client'
import type { Media } from '../types'

export interface DuplicateGroup {
  hash: string
  primary: Media
  duplicates: Media[]
}

export interface DuplicateReport {
  total_scanned: number
  groups_found: number
  items_marked: number
  groups: DuplicateGroup[]
}

export const duplicatesAPI = {
  scan: (libraryID = '') =>
    api
      .post<DuplicateReport>('/duplicates/scan', null, {
        params: libraryID ? { library_id: libraryID } : undefined,
      })
      .then((r) => r.data),
  unmark: (libraryID = '') =>
    api
      .post<{ unmarked: number }>('/duplicates/unmark', null, {
        params: libraryID ? { library_id: libraryID } : undefined,
      })
      .then((r) => r.data),
}
