import { useEffect, useMemo, useState } from 'react'
import { ImageOff, Info } from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { discoverItemSource } from './discoverPageModel'

export function ContentRow({
  title,
  items,
  imageVersion,
  onSelect,
}: {
  title: string
  items: DiscoverItem[]
  imageVersion?: string
  onSelect: (item: DiscoverItem) => void
}) {
  return (
    <section className="space-y-4">
      <h2 className="pl-1 font-display text-2xl font-semibold text-ink-600">{title}</h2>
      <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-7 xl:grid-cols-8">
        {items.map((item, index) => (
          <DiscoverCard key={discoverKey(item, index)} item={item} imageVersion={imageVersion} onSelect={onSelect} />
        ))}
      </div>
    </section>
  )
}

export function DiscoverSkeleton() {
  return (
    <div className="space-y-8">
      {[1, 2, 3].map((section) => (
        <section key={section} className="space-y-4">
          <div className="h-8 w-48 animate-pulse rounded-xl bg-gray-100" />
          <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-7 xl:grid-cols-8">
            {[1, 2, 3, 4, 5, 6, 7, 8].map((item) => (
              <div key={item} className="aspect-[2/3] animate-pulse rounded-xl bg-gray-100" />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function DiscoverCard({
  item,
  imageVersion,
  onSelect,
}: {
  item: DiscoverItem
  imageVersion?: string
  onSelect: (item: DiscoverItem) => void
}) {
  const source = discoverItemSource(item)
  const [posterFailed, setPosterFailed] = useState(false)
  const posterSrc = useMemo(
    () => imageURL(item.poster_url, imageVersion, true),
    [imageVersion, item.poster_url],
  )

  useEffect(() => {
    setPosterFailed(false)
  }, [posterSrc])

  const showFallback = !posterSrc || posterFailed

  return (
    <button
      type="button"
      onClick={() => onSelect(item)}
      className="group relative overflow-hidden rounded-xl border border-gray-200 bg-gray-50 text-left transition-all duration-300 hover:-translate-y-1 hover:border-primary-500/30 hover:shadow-xl focus:outline-none focus:ring-2 focus:ring-primary-400/40"
    >
      <div className="relative aspect-[2/3] w-full overflow-hidden bg-surface-900">
        {posterSrc && (
          <img
            src={posterSrc}
            alt={item.title}
            loading="lazy"
            referrerPolicy="no-referrer"
            onError={() => setPosterFailed(true)}
            onLoad={(event) => {
              const img = event.currentTarget
              setPosterFailed(img.naturalWidth <= 1 && img.naturalHeight <= 1)
            }}
            className={
              'h-full w-full object-cover transition-transform duration-500 group-hover:scale-105 ' +
              (posterFailed ? 'opacity-0' : 'opacity-100')
            }
          />
        )}
        {showFallback && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-gray-100 px-3 text-center text-gray-500">
            <ImageOff size={22} className="text-gray-400" />
            <span className="line-clamp-2 text-xs font-medium text-gray-600">{item.title}</span>
            <span className="text-[10px] text-gray-400">{posterSrc ? '海报待刷新' : '无海报'}</span>
          </div>
        )}
        <div className="absolute left-1.5 top-1.5 rounded-xl border border-white/20 bg-black/65 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-white backdrop-blur-sm">
          {source}
        </div>
        {(item.rating ?? 0) > 0 && (
          <div className="absolute right-1.5 top-1.5 rounded-xl border border-yellow-400/30 bg-black/70 px-1.5 py-0.5 text-[11px] font-semibold text-yellow-400 backdrop-blur-sm">
            ★ {(item.rating ?? 0).toFixed(1)}
          </div>
        )}
      </div>
      <div className="space-y-0.5 px-2.5 py-2">
        <p className="truncate text-xs font-medium text-ink-600 transition-colors group-hover:text-brand-500">
          {item.title}
        </p>
        <p className="text-[11px] text-sand-500">
          {[item.media_type, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' · ') || '推荐'}
        </p>
        <p className="flex items-center gap-1 pt-1 text-[10px] font-semibold text-brand-500">
          <Info size={10} />
          详情 / 订阅
        </p>
      </div>
    </button>
  )
}

function discoverKey(item: DiscoverItem, index: number): string {
  return `${item.source || 'source'}:${item.tmdb_id || item.douban_id || item.bangumi_id || item.title}:${index}`
}
