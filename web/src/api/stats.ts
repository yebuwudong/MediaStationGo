import { api } from './client'
import type { Hardware, StatsSnapshot } from '../types'

export const statsAPI = {
  snapshot: () => api.get<StatsSnapshot>('/stats').then((r) => r.data),
  monitor: () => api.get<Hardware>('/stats/monitor').then((r) => r.data),
}
