import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, Play, Clock, Film, Compass, Sparkles } from 'lucide-react'

import { mediaAPI } from '../api/library'
import { playbackAPI, type HistoryItem } from '../api/playback'
import { imageURL } from '../api/client'
import type { Media } from '../types'

export function HomePage() {
  const [recent, setRecent] = useState<Media[]>([])
  const [history, setHistory] = useState<HistoryItem[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      mediaAPI.search('', 24).then((d) => d.items),
      playbackAPI.recentHistory().catch(() => [] as HistoryItem[]),
    ])
      .then(([items, hist]) => {
        setRecent(items)
        setHistory(hist.filter((h) => !h.completed && !!h.media))
      })
      .finally(() => setLoading(false))
  }, [])

  const empty = !loading && recent.length === 0 && history.length === 0

  return (
    <div className="animate-fade-in space-y-12">
      {/* ── Header ── */}
      <header className="flex items-end justify-between">
        <div>
          <h1 className="font-display text-3xl font-bold tracking-tight text-cream-100">
            媒体中心
          </h1>
          <p className="mt-1.5 text-sm text-cream-400">
            从你上次离开的地方继续
          </p>
        </div>
        {!loading && (
          <Link
            to="/discover"
            className="flex items-center gap-1.5 text-sm text-cream-400 transition-colors hover:text-brand-400"
          >
            <Compass size={14} />
            发现更多
            <ArrowRight size={14} />
          </Link>
        )}
      </header>

      {/* ── Loading ── */}
      {loading && (
        <div className="flex items-center gap-3 py-16">
          <div className="h-1.5 w-1.5 animate-pulse rounded-full bg-brand-500/60" />
          <p className="text-sm text-cream-500">加载媒体库…</p>
        </div>
      )}

      {/* ── Empty state ── */}
      {empty && (
        <div className="surface-card flex flex-col items-center gap-4 py-16 text-center">
          <Film className="h-12 w-12 text-cream-900/40" />
          <div>
            <p className="font-medium text-cream-300">还没有任何媒体</p>
            <p className="mt-1 text-sm text-cream-500">
              前往{' '}
              <Link to="/admin" className="text-brand-400 underline underline-offset-2">
                管理后台
              </Link>{' '}
              创建媒体库，然后触发一次扫描
            </p>
          </div>
        </div>
      )}

      {/* ── Continue Watching — wide horizontal stripe ── */}
      {history.length > 0 && (
        <section>
          <SectionHeading icon={<Play size={16} />} label="继续观看" />
          <div className="flex gap-4 overflow-x-auto pb-2">
            {history.slice(0, 6).map((h) => {
              const m = h.media!
              const progress = h.duration_ms > 0 ? h.position_ms / h.duration_ms : 0
              return <WideContinueCard key={h.id} media={m} progress={progress} />
            })}
          </div>
        </section>
      )}

      {/* ── Recently Added — bento grid ── */}
      {recent.length > 0 && (
        <section>
          <SectionHeading icon={<Clock size={16} />} label="最近添加" />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
            {recent.slice(0, 12).map((m, i) => {
              // First 2 items get double-column span on large screens
              const isHero = i < 2
              return (
                <PosterCard
                  key={m.id}
                  media={m}
                  className={isHero ? 'lg:col-span-2 lg:row-span-2' : ''}
                />
              )
            })}
          </div>
        </section>
      )}

      {/* ── Quick Links ── */}
      {!empty && (
        <section>
          <SectionHeading icon={<Sparkles size={16} />} label="快捷入口" />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <QuickLink to="/discover" label="发现" desc="探索新内容" />
            <QuickLink to="/favourites" label="收藏" desc="我的珍藏" />
            <QuickLink to="/history" label="历史" desc="观看记录" />
            <QuickLink to="/ai" label="AI" desc="智能推荐" />
          </div>
        </section>
      )}
    </div>
  )
}

/* ─── Section Heading ─── */
function SectionHeading({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <div className="mb-4 flex items-center gap-2">
      <span className="text-cream-400">{icon}</span>
      <h2 className="font-display text-lg font-semibold tracking-tight text-cream-200">
        {label}
      </h2>
    </div>
  )
}

/* ─── Wide Continue-Watching Card ─── */
function WideContinueCard({ media, progress }: { media: Media; progress: number }) {
  return (
    <Link
      to={`/media/${media.id}`}
      className="group flex shrink-0 w-72 items-center gap-4 rounded-lg border border-cream-900/15 bg-surface-400 p-3 transition-all hover:border-brand-500/30 hover:bg-surface-300"
    >
      {/* Poster thumbnail */}
      <div className="relative h-20 w-14 shrink-0 overflow-hidden rounded-md bg-surface-600">
        {media.poster_url ? (
          <img
            src={imageURL(media.poster_url)}
            alt={media.title}
            loading="lazy"
            className="h-full w-full object-cover"
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="flex h-full items-center justify-center text-cream-900/30">
            <Film size={20} />
          </div>
        )}
      </div>

      {/* Info */}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-cream-200 group-hover:text-cream-100">
          {media.title}
        </p>
        {media.year > 0 && (
          <p className="mt-0.5 text-xs text-cream-500">{media.year}</p>
        )}

        {/* Progress bar */}
        {progress > 0 && (
          <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-cream-900/20">
            <div
              className="h-full rounded-full bg-brand-500/70 transition-all group-hover:bg-brand-400"
              style={{ width: `${Math.round(progress * 100)}%` }}
            />
          </div>
        )}
      </div>

      <Play
        size={14}
        className="shrink-0 text-cream-600 opacity-0 transition-all group-hover:opacity-100 group-hover:text-brand-400"
      />
    </Link>
  )
}

/* ─── Poster Card (grid item) ─── */
function PosterCard({ media, className = '' }: { media: Media; className?: string }) {
  return (
    <Link
      to={`/media/${media.id}`}
      className={`group block overflow-hidden rounded-lg border border-cream-900/15 bg-surface-400 transition-all hover:border-brand-500/30 hover:bg-surface-300 ${className}`}
    >
      <div className="relative aspect-[2/3] w-full overflow-hidden bg-surface-600">
        {media.poster_url ? (
          <img
            src={imageURL(media.poster_url)}
            alt={media.title}
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-cream-900/30">
            <Film size={48} />
          </div>
        )}
        {/* Overlay on hover */}
        <div className="absolute inset-0 flex items-end bg-gradient-to-t from-black/60 via-transparent to-transparent p-3 opacity-0 transition-opacity group-hover:opacity-100">
          <span className="flex items-center gap-1.5 text-xs text-white/90">
            <Play size={12} />
            播放
          </span>
        </div>
      </div>
      <div className="px-3 py-2.5">
        <p className="truncate text-sm font-medium text-cream-200 group-hover:text-cream-100">
          {media.title}
        </p>
        {media.year > 0 && (
          <p className="mt-0.5 text-xs text-cream-500">{media.year}</p>
        )}
      </div>
    </Link>
  )
}

/* ─── Quick Link Tile ─── */
function QuickLink({ to, label, desc }: { to: string; label: string; desc: string }) {
  return (
    <Link
      to={to}
      className="group flex items-center gap-3 rounded-lg border border-cream-900/15 bg-surface-400 px-4 py-3.5 transition-all hover:border-brand-500/20 hover:bg-surface-300"
    >
      <div className="flex-1">
        <p className="text-sm font-medium text-cream-300 group-hover:text-cream-100">{label}</p>
        <p className="mt-0.5 text-xs text-cream-500">{desc}</p>
      </div>
      <ArrowRight size={14} className="text-cream-600 transition-colors group-hover:text-brand-400" />
    </Link>
  )
}
