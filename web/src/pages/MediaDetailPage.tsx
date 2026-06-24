import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Heart, Play, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { mediaAPI } from '../api/library'
import { playbackAPI } from '../api/playback'
import { recycleAPI } from '../api/recycle'
import { useAuthStore } from '../stores/auth'
import { api } from '../api/client'
import type { Media } from '../types'
import { confirmAction } from '../components/confirmAction'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import { OrganizeMediaDialog } from '../components/OrganizeMediaDialog'
import { getSeriesKey, isEpisodeLike } from '../utils/groupSeries'
import { MediaDetailAdminPanel } from './MediaDetailAdminPanel'
import { MediaDetailBackdrop, MediaDetailPoster } from './MediaDetailArtwork'
import { MediaDetailMetadata } from './MediaDetailMetadata'

function mediaLibraryBackTarget(media: Media): string {
  const libraryID = media.display_library_id || media.library_id
  if (!libraryID) return ''
  if (!isEpisodeLike(media)) return `/library/${encodeURIComponent(libraryID)}`

  const seriesKey = getSeriesKey(media)
  const target = `/library/${encodeURIComponent(libraryID)}`
  return seriesKey ? `${target}?series=${encodeURIComponent(seriesKey)}` : target
}

export function MediaDetailPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const role = useAuthStore((s) => s.user?.role)
  const [media, setMedia] = useState<Media | null>(null)
  const [favourite, setFavourite] = useState(false)
  const [loading, setLoading] = useState(true)
  const [manualScrapeOpen, setManualScrapeOpen] = useState(false)
  const [metadataEditOpen, setMetadataEditOpen] = useState(false)
  const [organizeOpen, setOrganizeOpen] = useState(false)
  const [scrapeEpisodeArtwork, setScrapeEpisodeArtwork] = useState(false)

  const refresh = useCallback(async () => {
    if (!id) return
    setLoading(true)
    try {
      const m = await mediaAPI.get(id)
      setMedia(m)
      const favs = await playbackAPI.listFavourites().catch(() => [])
      setFavourite(favs.some((f) => f.id === m.id))
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [refresh])

  const toggleFav = async () => {
    if (!media) return
    const state = await playbackAPI.toggleFavourite(media.id)
    setFavourite(state)
    toast.success(state ? '已加入我的收藏' : '已取消收藏')
  }

  const rescrape = async () => {
    if (!media) return
    await api.post(`/media/${media.id}/scrape`, {
      episode_images: scrapeEpisodeArtwork,
      refresh_matched: true,
      include_matched: true,
    })
    toast.success('已触发重新刮削')
    await refresh()
  }

  const reprobe = async () => {
    if (!media) return
    try {
      const r = await api.post(`/media/${media.id}/probe`)
      if (r.data?.code === 0) {
        toast.success('重新探测成功')
      } else {
        toast.error(r.data?.error || '探测失败')
      }
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '探测失败，请检查 ffprobe 是否已安装'
      toast.error(msg)
    }
  }

  const exportNFO = async () => {
    if (!media) return
    try {
      const r = await recycleAPI.exportNFO(media.id)
      toast.success(`NFO 已成功写入 ${r.path}`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '导出失败'
      toast.error(msg)
    }
  }

  const softDelete = async () => {
    if (!media) return
    if (!(await confirmAction({ title: '移入回收站', message: `将「${media.title}」移至回收站? (磁盘文件保留)`, confirmText: '移入回收站' }))) return
    await recycleAPI.softDelete(media.id)
    toast.success('已移至回收站')
    const backTarget = mediaLibraryBackTarget(media)
    if (backTarget) navigate(backTarget, { replace: true })
    else navigate(-1)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-48">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-gray-100 border-t-gray-900" />
      </div>
    )
  }
  if (!media) {
    return (
      <div className="text-center py-24 bg-white rounded-2xl border border-gray-200">
        <p className="text-gray-500">媒体资源已被移除或不存在</p>
      </div>
    )
  }

  return (
    <div className="relative overflow-hidden rounded-3xl bg-white border border-gray-200/90 shadow-[0_1px_3px_rgba(0,0,0,0.01),0_1px_2px_rgba(0,0,0,0.015)]">
      <MediaDetailBackdrop media={media} />

      <div className="relative z-20 px-6 pt-6 sm:px-10 sm:pt-8">
        <button
          type="button"
          onClick={() => {
            const backTarget = mediaLibraryBackTarget(media)
            if (backTarget) navigate(backTarget)
            else navigate(-1)
          }}
          className="btn-ghost gap-2 bg-white/80 shadow-sm backdrop-blur hover:bg-white"
        >
          <ArrowLeft size={16} />
          <span>返回媒体库</span>
        </button>
      </div>

      <div className="relative z-10 p-6 sm:p-10 flex flex-col md:flex-row gap-8 lg:gap-12">
        <MediaDetailPoster media={media} />

        <div className="flex-1 space-y-6">
          <MediaDetailMetadata media={media} />

          <div className="divider border-gray-200/60" />

          <div className="flex flex-col gap-5">
            <div className="flex flex-wrap gap-3">
              <Link to={`/play/${media.id}`} className="btn-primary px-6 py-3.5 shadow-sm">
                <Play size={16} fill="currentColor" />
                <span>立即播放</span>
              </Link>

              <Link
                to={`/play/${media.id}?mode=hls`}
                className="btn-outline border-brand-500/30 hover:border-brand-500 text-[#c9954a] hover:bg-brand-50 px-5"
              >
                <RefreshCw size={14} className="animate-spin-slow" />
                <span>HLS 兼容转码播放</span>
              </Link>

              <ExternalPlayerButton mediaId={media.id} />

              <button
                onClick={toggleFav}
                className={
                  'btn-outline gap-2 ' +
                  (favourite
                    ? '!border-red-200 !bg-red-50 !text-red-600 hover:!bg-red-100/50'
                    : 'hover:border-red-200 hover:text-red-600 hover:bg-red-50/50')
                }
              >
                <Heart size={14} fill={favourite ? 'currentColor' : 'none'} />
                <span>{favourite ? '取消收藏' : '加入收藏'}</span>
              </button>
            </div>

            {role === 'admin' && (
              <MediaDetailAdminPanel
                media={media}
                scrapeEpisodeArtwork={scrapeEpisodeArtwork}
                onScrapeEpisodeArtworkChange={setScrapeEpisodeArtwork}
                onSmartScrape={rescrape}
                onManualScrape={() => setManualScrapeOpen(true)}
                onMetadataEdit={() => setMetadataEditOpen(true)}
                onOrganize={() => setOrganizeOpen(true)}
                onProbe={reprobe}
                onExportNFO={exportNFO}
                onSoftDelete={softDelete}
              />
            )}
          </div>
        </div>
      </div>
      <ManualScrapeDialog
        open={manualScrapeOpen}
        media={media}
        defaultQuery={media.title}
        mediaType={media.season_num > 0 || media.episode_num > 0 ? 'tv' : undefined}
        scopeLabel={media.title}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={() => setManualScrapeOpen(false)}
        onApplied={refresh}
      />
      <MetadataEditDialog
        open={metadataEditOpen}
        media={media}
        onClose={() => setMetadataEditOpen(false)}
        onSaved={async (next) => {
          setMedia(next)
          await refresh()
        }}
      />
      <OrganizeMediaDialog
        open={organizeOpen}
        media={media}
        onClose={() => setOrganizeOpen(false)}
        onOrganized={refresh}
      />
    </div>
  )
}
