import { FormEvent, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Download,
  ExternalLink,
  Eye,
  Filter,
  RefreshCw,
  Rss,
  Search,
  ShieldAlert,
} from 'lucide-react'

import { sitesAPI, type SiteCategory, type SiteSearchResult } from '../api/sites'
import { imageURL } from '../api/client'
import type { Site } from '../types'

function fmtBytes(n: number): string {
  if (!n || n <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = n
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx++
  }
  return `${value.toFixed(1)} ${units[idx]}`
}

function unwrapSites(payload: unknown): Site[] {
  const data = payload as { data?: unknown; items?: unknown }
  if (Array.isArray(data.data)) return data.data as Site[]
  if (Array.isArray(data.items)) return data.items as Site[]
  if (Array.isArray(payload)) return payload as Site[]
  return []
}

function categoryKey(category: SiteCategory): string {
  return `${category.site_id || category.site_type || 'all'}:${category.id}:${category.name}`
}

function itemKey(item: SiteSearchResult, idx: number): string {
  return `${item.site_id}:${item.id || item.torrent_url || item.download_url || idx}`
}

function cleanTitle(title: string): string {
  return title.length > 110 ? `${title.slice(0, 110)}...` : title
}

function detailFromItem(item: SiteSearchResult): Record<string, unknown> {
  return {
    id: item.id,
    title: item.title,
    subtitle: item.subtitle,
    category: item.category,
    poster_url: item.poster_url,
    backdrop_url: item.backdrop_url,
    size: item.size,
    seeders: item.seeders,
    leechers: item.leechers,
    snatched: item.snatched,
    free: item.free,
    adult: item.adult,
    upload_time: item.upload_time,
    detail_url: item.torrent_url,
    download_url: item.download_url,
    site_name: item.site_name,
  }
}

function mergeDetail(item: SiteSearchResult, incoming: Record<string, unknown>): Record<string, unknown> {
  const base = detailFromItem(item)
  for (const [key, value] of Object.entries(incoming)) {
    if (value === null || value === undefined || value === '') continue
    if (typeof value === 'number' && value === 0 && typeof base[key] === 'number' && Number(base[key]) > 0) {
      continue
    }
    if (key === 'upload_time' && value === '0001-01-01T00:00:00Z') continue
    base[key] = value
  }
  return base
}

function resourceVisual(item: SiteSearchResult | Record<string, unknown>): string {
  const poster = typeof item.poster_url === 'string' ? item.poster_url : ''
  const backdrop = typeof item.backdrop_url === 'string' ? item.backdrop_url : ''
  return poster || backdrop
}

export function PTResourcesPage() {
  const [sites, setSites] = useState<Site[]>([])
  const [categories, setCategories] = useState<SiteCategory[]>([])
  const [siteID, setSiteID] = useState('')
  const [category, setCategory] = useState('')
  const [keyword, setKeyword] = useState('')
  const [submittedKeyword, setSubmittedKeyword] = useState('')
  const [includeAdult, setIncludeAdult] = useState(false)
  const [page, setPage] = useState(1)
  const [pageInput, setPageInput] = useState('1')
  const [items, setItems] = useState<SiteSearchResult[]>([])
  const [total, setTotal] = useState(0)
  const [pageSize, setPageSize] = useState(50)
  const [totalPages, setTotalPages] = useState(0)
  const [loading, setLoading] = useState(false)
  const [actingKey, setActingKey] = useState('')
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null)

  const loadSites = async () => {
    const payload = await sitesAPI.list()
    setSites(unwrapSites(payload).filter((site) => site.enabled !== false))
  }

  const loadCategories = async (nextSiteID = siteID) => {
    const rows = await sitesAPI.categories(nextSiteID)
    setCategories(rows)
    if (category && !rows.some((row) => row.id === category || row.name === category)) {
      setCategory('')
    }
  }

  const loadResources = async (nextPage = page, nextKeyword = submittedKeyword) => {
    setLoading(true)
    try {
      const data = await sitesAPI.browse({
        site_id: siteID || undefined,
        category: category || undefined,
        keyword: nextKeyword || undefined,
        page: nextPage,
        include_adult: includeAdult,
      })
      setItems(data.items || [])
      setTotal(data.total || 0)
      setPage(data.page || nextPage)
      setPageInput(String(data.page || nextPage))
      setPageSize(data.page_size || 50)
      setTotalPages(data.total_pages || (data.total ? Math.ceil(data.total / (data.page_size || 50)) : 0))
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '加载 PT 资源失败'
      toast.error(message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    Promise.all([loadSites(), loadCategories('')])
      .then(() => loadResources(1, ''))
      .catch(() => toast.error('加载 PT 资源中心失败'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    loadCategories(siteID).catch(() => toast.error('加载分类失败'))
    setPage(1)
    loadResources(1).catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [siteID, category, includeAdult])

  const groupedCategories = useMemo(() => {
    const fromItems = items
      .filter((item) => item.category)
      .map((item) => ({
        id: item.category || '',
        name: item.category || '',
        group: '当前结果',
        site_id: item.site_id,
        site_name: item.site_name,
        site_type: '',
        adult: Boolean(item.adult),
      }))
    const byKey = new Map<string, SiteCategory>()
    for (const item of [...categories, ...fromItems]) {
      byKey.set(categoryKey(item), item)
    }
    const visible = includeAdult
      ? Array.from(byKey.values())
      : Array.from(byKey.values()).filter((item) => !item.adult)
    const groups = new Map<string, SiteCategory[]>()
    for (const item of visible) {
      const key = item.site_name || item.group || '其他'
      groups.set(key, [...(groups.get(key) || []), item])
    }
    return Array.from(groups.entries())
  }, [categories, includeAdult, items])

  const selectedSite = sites.find((site) => site.id === siteID)
  const canGoNext = totalPages > 0 ? page < totalPages : items.length > 0
  const resultText = `${selectedSite ? selectedSite.name : '全部站点'} / ${submittedKeyword || '全站'} / ${total || items.length} 条`

  const onSearch = (event: FormEvent) => {
    event.preventDefault()
    const next = keyword.trim()
    setSubmittedKeyword(next)
    setPage(1)
    loadResources(1, next)
  }

  const onJumpPage = (event: FormEvent) => {
    event.preventDefault()
    const parsed = Number.parseInt(pageInput, 10)
    if (!Number.isFinite(parsed) || parsed <= 0) {
      toast.error('页码不正确')
      setPageInput(String(page))
      return
    }
    const next = totalPages > 0 ? Math.min(parsed, totalPages) : parsed
    loadResources(next)
  }

  const onDownload = async (item: SiteSearchResult) => {
    const key = `download:${itemKey(item, 0)}`
    setActingKey(key)
    try {
      await sitesAPI.download({
        site_id: item.site_id,
        id: item.id,
        title: item.title,
        download_url: item.download_url,
        torrent_url: item.torrent_url,
        source_category: item.category,
        media_category: item.adult ? '成人' : undefined,
      })
      toast.success('已加入下载')
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '加入下载失败'
      toast.error(message)
    } finally {
      setActingKey('')
    }
  }

  const onSubscribe = async (item: SiteSearchResult) => {
    const key = `subscribe:${itemKey(item, 0)}`
    setActingKey(key)
    try {
      await sitesAPI.subscribe({
        site_id: item.site_id,
        category: category || item.category,
        include_adult: includeAdult || item.adult,
        name: item.title,
        keyword: item.title,
        filter: item.title,
        media_category: item.adult ? '成人' : undefined,
      })
      toast.success('已创建订阅')
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '创建订阅失败'
      toast.error(message)
    } finally {
      setActingKey('')
    }
  }

  const onDetail = async (item: SiteSearchResult) => {
    setDetail(detailFromItem(item))
    if (!item.id) {
      if (item.torrent_url) window.open(item.torrent_url, '_blank', 'noopener,noreferrer')
      return
    }
    const key = `detail:${itemKey(item, 0)}`
    setActingKey(key)
    try {
      const data = await sitesAPI.detail(item.site_id, item.id)
      setDetail(mergeDetail(item, data as Record<string, unknown>))
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '站点详情接口不可用，已显示列表信息'
      toast.error(message)
    } finally {
      setActingKey('')
    }
  }

  const detailDescription = typeof detail?.description === 'string' ? detail.description : ''
  const detailFiles = Array.isArray(detail?.files) ? detail.files.map((file) => String(file)) : []

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-center gap-3">
          <Filter className="h-6 w-6 text-brand-500" />
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">PT 资源中心</h1>
            <p className="text-sm text-ink-50">站点资源、分类、成人资源、订阅下载</p>
          </div>
        </div>
        <button className="neon-button flex items-center gap-2" onClick={() => loadResources(1)}>
          <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
      </header>

      <section className="grid gap-3 md:grid-cols-3">
        {sites.map((site) => (
          <button
            key={site.id}
            className={`glass-panel text-left transition ${
              siteID === site.id ? '!border-primary-400/70 !bg-primary-400/10' : 'hover:border-primary-400/40'
            }`}
            onClick={() => setSiteID(siteID === site.id ? '' : site.id)}
          >
            <div className="flex items-center justify-between gap-3">
              <p className="font-semibold text-ink-600">{site.name}</p>
              <span className="rounded border border-gray-200 px-2 py-0.5 text-xs text-ink-100">{site.type}</span>
            </div>
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              <span className={site.login_status === 'ok' ? 'text-emerald-500' : 'text-sand-500'}>
                {site.login_status || 'unknown'}
              </span>
              {site.downloader && <span className="text-ink-50">{site.downloader}</span>}
              {site.rss_url && <span className="text-brand-500">RSS</span>}
            </div>
          </button>
        ))}
        {!sites.length && (
          <div className="glass-panel md:col-span-3 text-sand-500">暂无已启用站点。</div>
        )}
      </section>

      <section className="glass-panel space-y-4">
        <form onSubmit={onSearch} className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_180px_140px]">
          <div className="flex min-w-0 items-center gap-2">
            <Search size={16} className="text-sand-500" />
            <input
              className="input-base min-w-0 flex-1"
              placeholder="关键词"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
            />
          </div>
          <select
            className="input-base"
            value={siteID}
            onChange={(event) => setSiteID(event.target.value)}
          >
            <option value="">全部站点</option>
            {sites.map((site) => (
              <option key={site.id} value={site.id}>
                {site.name}
              </option>
            ))}
          </select>
          <button className="neon-button" type="submit" disabled={loading}>
            {loading ? '加载中...' : '搜索'}
          </button>
        </form>

        <div className="flex flex-wrap items-center gap-2">
          <button
            className={`rounded border px-3 py-1.5 text-sm ${
              category === '' ? 'border-primary-400 bg-primary-400/10 text-brand-500' : 'border-gray-200 text-ink-100'
            }`}
            onClick={() => setCategory('')}
          >
            全部
          </button>
          <label className="ml-auto flex items-center gap-2 rounded border border-red-300/50 px-3 py-1.5 text-sm text-red-500">
            <input
              type="checkbox"
              checked={includeAdult}
              onChange={(event) => setIncludeAdult(event.target.checked)}
            />
            <ShieldAlert size={14} />
            成人
          </label>
        </div>

        <div className="space-y-3">
          {groupedCategories.map(([group, rows]) => (
            <div key={group} className="flex flex-wrap items-center gap-2">
              <span className="w-14 text-xs text-sand-500">{group}</span>
              {rows.map((row) => (
                <button
                  key={categoryKey(row)}
                  className={`rounded border px-2.5 py-1 text-xs ${
                    category === row.id || category === row.name
                      ? row.adult
                        ? 'border-red-400 bg-red-400/10 text-red-500'
                        : 'border-primary-400 bg-primary-400/10 text-brand-500'
                      : 'border-gray-200 text-ink-100 hover:border-primary-400/40'
                  }`}
                  onClick={() => setCategory(row.id)}
                >
                  {row.adult && <AlertTriangle size={12} className="mr-1 inline" />}
                  {row.name}
                </button>
              ))}
            </div>
          ))}
        </div>
      </section>

      <section className="glass-panel">
        <div className="mb-4 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
          <div className="text-sm text-ink-50">
            <p>{resultText}</p>
            <p className="mt-1 text-xs text-sand-500">
              第 {page} 页{totalPages > 0 ? ` / 共 ${totalPages} 页` : ''} · 每页 {pageSize} 条
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button
              className="rounded border border-gray-200 p-2 text-ink-100 disabled:opacity-40"
              disabled={page <= 1 || loading}
              onClick={() => loadResources(page - 1)}
              title="上一页"
            >
              <ChevronLeft size={16} />
            </button>
            <form onSubmit={onJumpPage} className="flex items-center gap-2">
              <input
                className="input-base h-9 w-20 px-2 py-1 text-center text-sm"
                inputMode="numeric"
                value={pageInput}
                onChange={(event) => setPageInput(event.target.value)}
                disabled={loading}
              />
              <button className="btn-outline h-9 px-3 py-1 text-xs" type="submit" disabled={loading}>
                跳转
              </button>
            </form>
            <button
              className="rounded border border-gray-200 p-2 text-ink-100 disabled:opacity-40"
              disabled={loading || !canGoNext}
              onClick={() => loadResources(page + 1)}
              title="下一页"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>

        <div className="grid gap-3 md:hidden">
          {items.map((item, idx) => {
            const key = itemKey(item, idx)
            return (
              <ResourceCard
                key={key}
                item={item}
                itemKeyValue={key}
                actingKey={actingKey}
                onDetail={onDetail}
                onDownload={onDownload}
                onSubscribe={onSubscribe}
              />
            )
          })}
        </div>

        <div className="hidden overflow-x-auto md:block">
          <table className="w-full min-w-[900px] text-left text-sm">
          <thead className="text-xs uppercase tracking-wider text-sand-500">
            <tr>
              <th className="py-2">图</th>
              <th className="py-2">站点</th>
              <th>标题</th>
              <th>分类</th>
              <th>大小</th>
              <th>S</th>
              <th>L</th>
              <th>完成</th>
              <th className="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, idx) => {
              const key = itemKey(item, idx)
              return (
                <tr key={key} className="border-t border-gray-200 align-top">
                  <td className="py-3">
                    {resourceVisual(item) && (
                      <button
                        className="h-16 w-11 overflow-hidden rounded border border-gray-200 bg-gray-100"
                        onClick={() => onDetail(item)}
                      >
                        <img
                          src={imageURL(resourceVisual(item))}
                          alt={item.title}
                          loading="lazy"
                          referrerPolicy="no-referrer"
                          className="h-full w-full object-cover"
                        />
                      </button>
                    )}
                  </td>
                  <td className="py-3 text-brand-500">{item.site_name}</td>
                  <td className="max-w-xl py-3">
                    <div className="flex flex-wrap items-center gap-2">
                      {item.free && <span className="rounded border border-emerald-400/40 px-1.5 py-0.5 text-xs text-emerald-500">Free</span>}
                      {item.adult && <span className="rounded border border-red-400/40 px-1.5 py-0.5 text-xs text-red-500">成人</span>}
                    </div>
                    <button className="mt-1 text-left text-ink-600 hover:text-brand-500" onClick={() => onDetail(item)}>
                      {cleanTitle(item.title)}
                    </button>
                    {item.subtitle && <p className="mt-1 text-xs text-ink-50">{cleanTitle(item.subtitle)}</p>}
                    {item.upload_time && <p className="mt-1 text-xs text-sand-500">{new Date(item.upload_time).toLocaleString()}</p>}
                  </td>
                  <td className="whitespace-nowrap text-ink-100">{item.category || '-'}</td>
                  <td className="whitespace-nowrap text-ink-100">{fmtBytes(item.size)}</td>
                  <td className="text-emerald-400">{item.seeders || '-'}</td>
                  <td className="text-red-400">{item.leechers || '-'}</td>
                  <td className="text-ink-100">{item.snatched || '-'}</td>
                  <td className="py-3 text-right">
                    <div className="flex justify-end gap-2">
                      <button
                        className="rounded border border-gray-200 p-1.5 text-ink-100 hover:border-primary-400/50"
                        onClick={() => onDetail(item)}
                        disabled={actingKey === `detail:${key}`}
                        title="详情"
                      >
                        <Eye size={14} />
                      </button>
                      {item.torrent_url && (
                        <a
                          className="rounded border border-gray-200 p-1.5 text-ink-100 hover:border-primary-400/50"
                          href={item.torrent_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          title="打开"
                        >
                          <ExternalLink size={14} />
                        </a>
                      )}
                      <button
                        className="rounded border border-primary-400/50 p-1.5 text-brand-500 hover:bg-primary-400/10"
                        onClick={() => onDownload(item)}
                        disabled={actingKey === `download:${key}`}
                        title="下载"
                      >
                        <Download size={14} />
                      </button>
                      <button
                        className="rounded border border-emerald-400/50 p-1.5 text-emerald-500 hover:bg-emerald-400/10"
                        onClick={() => onSubscribe(item)}
                        disabled={actingKey === `subscribe:${key}`}
                        title="订阅"
                      >
                        <Rss size={14} />
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
          </table>
        </div>

        {!loading && items.length === 0 && (
          <div className="py-10 text-center text-sand-500">暂无资源。</div>
        )}
      </section>

      {detail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4" onClick={() => setDetail(null)}>
          <div className="glass-panel max-h-[80vh] w-full max-w-2xl overflow-y-auto" onClick={(event) => event.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between gap-3">
              <h2 className="font-display text-xl font-bold text-ink-600">{String(detail.title || detail.id || '详情')}</h2>
              <button className="rounded border border-gray-200 px-3 py-1 text-sm text-ink-100" onClick={() => setDetail(null)}>
                关闭
              </button>
            </div>
            {resourceVisual(detail) && (
              <div className="mb-4 overflow-hidden rounded border border-gray-200 bg-gray-100">
                <img
                  src={imageURL(resourceVisual(detail))}
                  alt={String(detail.title || '')}
                  className="max-h-80 w-full object-contain"
                  referrerPolicy="no-referrer"
                />
              </div>
            )}
            <div className="grid gap-3 text-sm md:grid-cols-2">
              <Info label="站点" value={String(detail.site_name || '-')} />
              <Info label="分类" value={String(detail.category || '-')} />
              <Info label="大小" value={fmtBytes(Number(detail.size || 0))} />
              <Info label="做种" value={String(detail.seeders || '-')} />
              <Info label="下载" value={String(detail.leechers || '-')} />
              <Info label="完成" value={String(detail.snatched || '-')} />
              <Info label="InfoHash" value={String(detail.info_hash || '-')} />
            </div>
            {detailDescription && (
              <pre className="mt-4 whitespace-pre-wrap rounded border border-gray-200 bg-white/60 p-3 text-xs text-ink-100">
                {detailDescription}
              </pre>
            )}
            {detailFiles.length > 0 && (
              <div className="mt-4 space-y-1 text-xs text-ink-100">
                {detailFiles.slice(0, 80).map((file) => (
                  <p key={file} className="rounded bg-white/50 px-2 py-1">{file}</p>
                ))}
              </div>
            )}
            {typeof detail.detail_url === 'string' && detail.detail_url && (
              <a
                className="btn-outline mt-4 inline-flex"
                href={detail.detail_url}
                target="_blank"
                rel="noopener noreferrer"
              >
                <ExternalLink size={14} />
                打开原站详情
              </a>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function ResourceCard({
  item,
  itemKeyValue,
  actingKey,
  onDetail,
  onDownload,
  onSubscribe,
}: {
  item: SiteSearchResult
  itemKeyValue: string
  actingKey: string
  onDetail: (item: SiteSearchResult) => void
  onDownload: (item: SiteSearchResult) => void
  onSubscribe: (item: SiteSearchResult) => void
}) {
  const visual = resourceVisual(item)
  return (
    <article className="rounded-lg border border-gray-200 bg-white p-3 shadow-sm">
      <div className="flex gap-3">
        {visual && (
          <button
            type="button"
            onClick={() => onDetail(item)}
            className="h-28 w-20 shrink-0 overflow-hidden rounded border border-gray-200 bg-gray-100"
          >
            <img
              src={imageURL(visual)}
              alt={item.title}
              loading="lazy"
              referrerPolicy="no-referrer"
              className="h-full w-full object-cover"
            />
          </button>
        )}
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex flex-wrap items-center gap-1.5 text-[11px]">
            <span className="rounded border border-gray-200 px-1.5 py-0.5 text-brand-500">{item.site_name}</span>
            {item.category && <span className="rounded border border-gray-200 px-1.5 py-0.5 text-ink-100">{item.category}</span>}
            {item.free && <span className="rounded border border-emerald-400/40 px-1.5 py-0.5 text-emerald-500">Free</span>}
            {item.adult && <span className="rounded border border-red-400/40 px-1.5 py-0.5 text-red-500">成人</span>}
          </div>
          <button
            type="button"
            className="line-clamp-2 text-left text-sm font-semibold leading-5 text-ink-600 hover:text-brand-500"
            onClick={() => onDetail(item)}
          >
            {item.title}
          </button>
          {item.subtitle && <p className="mt-1 line-clamp-2 text-xs text-ink-50">{item.subtitle}</p>}
          <div className="mt-2 grid grid-cols-4 gap-2 text-xs">
            <InfoPill label="大小" value={fmtBytes(item.size)} />
            <InfoPill label="S" value={String(item.seeders || '-')} tone="seed" />
            <InfoPill label="L" value={String(item.leechers || '-')} tone="leech" />
            <InfoPill label="完成" value={String(item.snatched || '-')} />
          </div>
        </div>
      </div>
      <div className="mt-3 grid grid-cols-4 gap-2">
        <button
          className="btn-outline justify-center px-2 py-2 text-xs"
          onClick={() => onDetail(item)}
          disabled={actingKey === `detail:${itemKeyValue}`}
        >
          <Eye size={14} />
          详情
        </button>
        {item.torrent_url ? (
          <a
            className="btn-outline justify-center px-2 py-2 text-xs"
            href={item.torrent_url}
            target="_blank"
            rel="noopener noreferrer"
          >
            <ExternalLink size={14} />
            原站
          </a>
        ) : (
          <span className="btn-outline justify-center px-2 py-2 text-xs opacity-40">原站</span>
        )}
        <button
          className="btn-outline justify-center px-2 py-2 text-xs text-brand-500"
          onClick={() => onDownload(item)}
          disabled={actingKey === `download:${itemKeyValue}`}
        >
          <Download size={14} />
          下载
        </button>
        <button
          className="btn-outline justify-center px-2 py-2 text-xs text-emerald-500"
          onClick={() => onSubscribe(item)}
          disabled={actingKey === `subscribe:${itemKeyValue}`}
        >
          <Rss size={14} />
          订阅
        </button>
      </div>
    </article>
  )
}

function InfoPill({ label, value, tone }: { label: string; value: string; tone?: 'seed' | 'leech' }) {
  const color = tone === 'seed' ? 'text-emerald-500' : tone === 'leech' ? 'text-red-500' : 'text-ink-600'
  return (
    <div className="min-w-0 rounded border border-gray-100 bg-gray-50 px-2 py-1">
      <p className="truncate text-[10px] text-sand-500">{label}</p>
      <p className={`truncate font-semibold ${color}`}>{value}</p>
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-gray-200 bg-white/50 p-3">
      <p className="text-xs text-sand-500">{label}</p>
      <p className="mt-1 break-all text-ink-600">{value}</p>
    </div>
  )
}
