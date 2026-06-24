import type { DiscoverItem, DiscoverSection } from '../api/discover'

export const defaultSections = [
  'tmdb_trending_day',
  'douban_hot_movie',
  'douban_hot_tv',
  'bangumi_calendar',
]

export const discoverStorageKey = 'mediastation.discover.sections'

export const defaultSectionDefs: DiscoverSection[] = [
  { key: 'tmdb_trending_day', label: 'TMDb 今日趋势', provider: 'tmdb' },
  { key: 'tmdb_popular_movie', label: 'TMDb 热门电影', provider: 'tmdb' },
  { key: 'douban_hot_movie', label: '豆瓣热门电影', provider: 'douban' },
  { key: 'douban_hot_tv', label: '豆瓣热门剧集', provider: 'douban' },
  { key: 'bangumi_calendar', label: 'Bangumi 每日放送', provider: 'bangumi' },
]

export function discoverItemSource(item: DiscoverItem): string {
  return item.source || (item.bangumi_id ? 'bangumi' : item.douban_id ? 'douban' : 'tmdb')
}

export function readSavedSections(sections: DiscoverSection[]): string[] {
  try {
    const raw = window.localStorage.getItem(discoverStorageKey)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    const allowed = new Set(sections.map((section) => section.key))
    return parsed.filter((key) => typeof key === 'string' && allowed.has(key))
  } catch {
    return []
  }
}

export function buildSubscribeKeyword(item: DiscoverItem): string {
  return [item.title, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' ')
}
