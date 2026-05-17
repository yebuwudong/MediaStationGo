import { Link } from 'react-router-dom'
import { Film, Play } from 'lucide-react'

import { imageURL } from '../api/client'
import type { Media } from '../types'

/** Compact poster tile — used by library, search, favourites, poster wall, etc. */
export function MediaCard({
  media,
  progress,
}: {
  media: Media
  progress?: number
}) {
  return (
    <Link
      to={`/media/${media.id}`}
      className="group block overflow-hidden rounded-lg border border-cream-900/15 bg-surface-400 transition-all hover:border-brand-500/30 hover:bg-surface-300"
    >
      {/* Poster */}
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

        {/* Hover overlay */}
        <div className="absolute inset-0 flex items-end bg-gradient-to-t from-black/60 via-transparent to-transparent p-3 opacity-0 transition-opacity group-hover:opacity-100">
          <span className="flex items-center gap-1.5 text-xs text-white/90">
            <Play size={12} />
            播放
          </span>
        </div>

        {/* Progress bar */}
        {progress !== undefined && progress > 0 && progress < 1 && (
          <div className="absolute inset-x-0 bottom-0 h-1 bg-black/40">
            <div
              className="h-full bg-brand-500/80 transition-colors group-hover:bg-brand-400"
              style={{ width: `${Math.round(progress * 100)}%` }}
            />
          </div>
        )}
      </div>

      {/* Info */}
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
