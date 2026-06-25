import type { Subscription } from '../types'

export function subscriptionRuleBadges(subscription: Subscription): string[] {
  const labels = [
    subscription.search_mode === 'imdb' ? `IMDB ${subscription.imdb_id || '未填'}` : '关键词搜索',
    subscription.resolution && subscription.resolution !== 'best' ? subscription.resolution : '分辨率不限',
    subscription.quality || '质量不限',
    subscription.effects || '',
    subscription.release_groups ? `发布组 ${subscription.release_groups}` : '',
    subscription.wash_enabled ? `洗版 ${washPriorityLabel(subscription.wash_priority)}` : '未启用洗版',
  ]
  return labels.filter(Boolean)
}

export function subscriptionProgressLabel(subscription: Subscription): string {
  const isSeries = ['tv', 'anime', 'variety'].includes((subscription.media_type || '').toLowerCase())
  if (!isSeries) {
    if (subscription.in_library) return '本地已入库'
    return (subscription.downloaded_episodes || subscription.local_media_count || 0) > 0 ? '已下载未入库' : '本地未入库'
  }
  const downloaded = subscription.downloaded_episodes || 0
  const total = subscription.total_episodes || 0
  if (total > 0) {
    const missing = subscription.missing_episodes?.length || 0
    return missing > 0 ? `已下载 ${downloaded}/${total} 集，缺 ${missing} 集` : `已下载 ${downloaded}/${total} 集`
  }
  return `已下载 ${downloaded}/未知 集`
}

function washPriorityLabel(priority?: string): string {
  switch (priority) {
    case 'resolution':
      return '分辨率优先'
    case 'quality':
      return '片源质量优先'
    case 'effects':
      return '特效优先'
    case 'seeders':
      return '做种数优先'
    default:
      return '均衡'
  }
}
