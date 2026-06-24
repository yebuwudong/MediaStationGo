import type { DiscoverItem, DiscoverSection } from '../api/discover'

export const defaultSections = [
  'tmdb_trending_day',
  'tmdb_latest_movie',
  'tmdb_latest_tv',
  'douban_hot_movie',
  'douban_hot_tv',
  'bangumi_calendar',
]

export const discoverStorageKey = 'mediastation.discover.sections'
const discoverStorageVersion = 2
const legacyDefaultAdditions = ['tmdb_latest_movie', 'tmdb_latest_tv']

export const defaultSectionDefs: DiscoverSection[] = [
  { key: 'tmdb_trending_day', label: 'TMDb 今日趋势', provider: 'tmdb' },
  { key: 'tmdb_latest_movie', label: 'TMDb 最新电影', provider: 'tmdb' },
  { key: 'tmdb_latest_tv', label: 'TMDb 最新剧集', provider: 'tmdb' },
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
    const allowed = new Set(sections.map((section) => section.key))
    if (Array.isArray(parsed)) {
      return orderSectionKeys(addLegacyDefaults(parsed, allowed), sections)
    }
    if (!parsed || !Array.isArray(parsed.selected)) return []
    const selected = sanitizeSectionKeys(parsed.selected, allowed)
    if (parsed.version === discoverStorageVersion) {
      return orderSectionKeys(selected, sections)
    }
    return orderSectionKeys(addLegacyDefaults(selected, allowed), sections)
  } catch {
    return []
  }
}

export function serializeSavedSections(selected: string[]): string {
  return JSON.stringify({ version: discoverStorageVersion, selected })
}

function sanitizeSectionKeys(keys: unknown[], allowed: Set<string>): string[] {
  return keys.filter((key): key is string => typeof key === 'string' && allowed.has(key))
}

function addLegacyDefaults(keys: unknown[], allowed: Set<string>): string[] {
  const out = sanitizeSectionKeys(keys, allowed)
  for (const key of legacyDefaultAdditions) {
    if (allowed.has(key) && !out.includes(key)) {
      out.push(key)
    }
  }
  return out
}

function orderSectionKeys(keys: string[], sections: DiscoverSection[]): string[] {
  const selected = new Set(keys)
  return sections.map((section) => section.key).filter((key) => selected.has(key))
}

export function buildSubscribeKeyword(item: DiscoverItem): string {
  return [item.title, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' ')
}
