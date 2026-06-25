import { useState } from 'react'
import { Rss } from 'lucide-react'
import toast from 'react-hot-toast'

import type { ExternalMediaResult } from '../api/ai'
import { imageURL } from '../api/client'
import { buildSiteSearchFeedURL, buildSubscriptionAliases, subscriptionsAPI } from '../api/subscriptions'

type AIAssistantExternalResultsProps = {
  items: ExternalMediaResult[]
}

export function AIAssistantExternalResults({ items }: AIAssistantExternalResultsProps) {
  const [subscribing, setSubscribing] = useState('')

  if (items.length === 0) return null

  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {items.map((item) => {
        const keyword = item.subscribe_keyword || item.title
        const key = `${item.source}:${keyword}`
        return (
          <article key={key} className="rounded-2xl border border-gray-200 bg-gray-50 p-3">
            <div className="flex gap-3">
              <div className="h-24 w-16 shrink-0 overflow-hidden rounded-xl bg-white">
                {item.poster_url ? (
                  <img
                    src={imageURL(item.poster_url)}
                    alt={item.title}
                    className="h-full w-full object-cover"
                  />
                ) : null}
              </div>
              <div className="min-w-0 flex-1">
                <div className="mb-1 flex flex-wrap gap-2 text-[10px] uppercase text-brand-500">
                  <span>{item.source}</span>
                  {item.media_type && <span>{item.media_type}</span>}
                  {item.year ? <span>{item.year}</span> : null}
                </div>
                <h3 className="truncate font-semibold text-ink-600">{item.title}</h3>
                <p className="mt-1 line-clamp-2 text-xs text-ink-50">
                  {item.overview || `订阅关键词：${keyword}`}
                </p>
                <button
                  disabled={subscribing === key}
                  onClick={() => subscribeExternalItem(item, key, keyword, setSubscribing)}
                  className="mt-2 rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10 disabled:opacity-50"
                >
                  <Rss size={12} className="mr-1 inline" />
                  {subscribing === key ? '订阅中…' : '订阅并搜索 PT'}
                </button>
              </div>
            </div>
          </article>
        )
      })}
    </div>
  )
}

async function subscribeExternalItem(
  item: ExternalMediaResult,
  key: string,
  keyword: string,
  setSubscribing: (key: string) => void,
) {
  setSubscribing(key)
  try {
    const feed = buildSiteSearchFeedURL(keyword, item.source, buildSubscriptionAliases(item))
    const sub = await subscriptionsAPI.create({
      name: `${item.title} 自动订阅`,
      feed_url: feed,
      filter: keyword,
      media_type: item.media_type,
      source: item.source,
      poster_url: item.poster_url,
      backdrop_url: item.backdrop_url,
      overview: item.overview,
      original_name: item.original_name,
      year: item.year,
      total_episodes: item.total_episodes,
      enabled: true,
    })
    const run = await subscriptionsAPI.runNow(sub.id)
    toast.success(
      run.queued > 0
        ? `已订阅并加入 ${run.queued} 个下载`
        : '已订阅，暂未在 PT 站点找到可下载资源',
    )
  } catch (err: unknown) {
    const msg =
      (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
      '订阅失败'
    toast.error(msg)
  } finally {
    setSubscribing('')
  }
}
