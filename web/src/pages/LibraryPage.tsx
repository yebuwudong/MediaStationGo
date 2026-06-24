import { useState } from 'react'
import { useLocation, useParams, useSearchParams } from 'react-router-dom'
import { motion, AnimatePresence } from 'framer-motion'

import type { Media } from '../types'
import { useAuthStore } from '../stores/auth'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import { LibrarySeriesEpisodes } from './LibrarySeriesEpisodes'
import { LibrarySeriesDetailHeader } from './LibrarySeriesDetailHeader'
import { LibraryPageHeader } from './LibraryPageHeader'
import { LibraryMediaSections } from './LibraryMediaSections'
import { useLibraryData } from './useLibraryData'
import { useLibraryScanStatus } from './useLibraryScanStatus'
import { useLibrarySeriesSelection } from './useLibrarySeriesSelection'
import { useLibraryAdminActions } from './useLibraryAdminActions'

export function LibraryPage() {
  const { id = '' } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const location = useLocation()
  const role = useAuthStore((s) => s.user?.role)

  const [manualSeriesScrapeOpen, setManualSeriesScrapeOpen] = useState(false)
  const [seriesMetadataEditOpen, setSeriesMetadataEditOpen] = useState(false)
  const [manualMovie, setManualMovie] = useState<Media | null>(null)

  // 剧集模式：选中某个剧集后展开详情
  const [selectedSeries, setSelectedSeries] = useState<SeriesCard | null>(null)
  const [selectedSeason, setSelectedSeason] = useState<number | null>(null)

  const {
    library,
    items,
    seriesEpisodeItems,
    total,
    loading,
    loadingSeriesEpisodes,
    isSeriesLibrary,
    isSeries,
    seriesCards,
    loadingAllText,
    reloadCurrentLibrary,
  } = useLibraryData(id, selectedSeries)

  const {
    scanning,
    scanProgress,
    handleScan,
  } = useLibraryScanStatus({
    libraryID: id,
    isAdmin: role === 'admin',
    onLibraryChanged: reloadCurrentLibrary,
  })

  const {
    selectedEpisodes,
    visibleEpisodes,
    selectedSeriesEpisodes,
    selectedSeriesMediaIDs,
    handleSeriesClick,
    clearSelectedSeries,
  } = useLibrarySeriesSelection({
    items,
    seriesEpisodeItems,
    isSeriesLibrary,
    isSeries,
    loading,
    seriesCards,
    searchParams,
    setSearchParams,
    selectedSeries,
    setSelectedSeries,
    selectedSeason,
    setSelectedSeason,
    onClearSeriesState: () => setSeriesMetadataEditOpen(false),
  })

  const {
    scraping,
    scrapeEpisodeArtwork,
    repairing,
    seriesToolBusy,
    setScrapeEpisodeArtwork,
    handleScrape,
    handleRepairRescrape,
    handleSeriesSmartScrape,
    handleSeriesProbe,
    handleSeriesNFO,
    handleSeriesOrganize,
    handleSeriesSoftDelete,
    movieActions,
  } = useLibraryAdminActions({
    libraryID: id,
    role,
    library,
    selectedSeries,
    selectedSeriesEpisodes,
    reloadCurrentLibrary,
    clearSelectedSeries,
    setManualMovie,
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center py-32">
        <motion.div animate={{ opacity: [0.4, 1, 0.4] }} transition={{ repeat: Infinity, duration: 2 }} className="flex items-center gap-3">
          <div className="h-2 w-2 rounded-full bg-brand-500" />
          <span className="text-sm text-sand-500">加载中…</span>
        </motion.div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <LibraryPageHeader
        library={library}
        itemCount={isSeries ? seriesCards.length : total}
        loadingAllText={loadingAllText}
        scanProgress={scanProgress}
        isAdmin={role === 'admin'}
        scrapeEpisodeArtwork={scrapeEpisodeArtwork}
        scanning={scanning}
        scraping={scraping}
        repairing={repairing}
        onScrapeEpisodeArtworkChange={setScrapeEpisodeArtwork}
        onScan={handleScan}
        onScrape={handleScrape}
        onRepairRescrape={handleRepairRescrape}
      />

      <LibraryMediaSections
        isSeries={isSeries}
        items={items}
        seriesCards={seriesCards}
        selectedSeries={selectedSeries}
        loading={loading}
        movieActions={movieActions}
        onSeriesClick={handleSeriesClick}
      />

      {/* 剧集详情：季/集选择器 */}
      <AnimatePresence mode="wait">
        {selectedSeries && (
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            className="space-y-6"
          >
            <LibrarySeriesDetailHeader
              series={selectedSeries}
              visibleEpisodes={visibleEpisodes}
              allEpisodes={selectedSeriesEpisodes}
              playbackFrom={`${location.pathname}${location.search}`}
              isAdmin={role === 'admin'}
              seriesToolBusy={seriesToolBusy}
              onBack={clearSelectedSeries}
              onSmartScrape={handleSeriesSmartScrape}
              onManualScrape={() => setManualSeriesScrapeOpen(true)}
              onMetadataEdit={() => setSeriesMetadataEditOpen(true)}
              onProbe={handleSeriesProbe}
              onNFO={handleSeriesNFO}
              onOrganize={handleSeriesOrganize}
              onSoftDelete={handleSeriesSoftDelete}
            />

            {/* 季 / 集列表 */}
            <div className="space-y-6">
              <LibrarySeriesEpisodes
                loading={loadingSeriesEpisodes}
                selectedEpisodes={selectedEpisodes}
                selectedSeason={selectedSeason}
                visibleEpisodes={visibleEpisodes}
                playbackFrom={`${location.pathname}${location.search}`}
                onSeasonChange={setSelectedSeason}
              />
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <ManualScrapeDialog
        open={manualSeriesScrapeOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        defaultQuery={selectedSeries ? seriesTitle(selectedSeries.rep) : ''}
        mediaType={selectedSeries ? scrapeMediaType(library?.type, selectedSeries.rep) : 'tv'}
        scopeLabel={selectedSeries ? seriesTitle(selectedSeries.rep) : '当前剧集'}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={() => setManualSeriesScrapeOpen(false)}
        onApplied={reloadCurrentLibrary}
      />
      <MetadataEditDialog
        open={seriesMetadataEditOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        mode="series"
        scopeLabel={selectedSeries ? seriesTitle(selectedSeries.rep) : '当前剧集'}
        onClose={() => setSeriesMetadataEditOpen(false)}
        onSaved={reloadCurrentLibrary}
      />
      <ManualScrapeDialog
        open={!!manualMovie}
        media={manualMovie}
        defaultQuery={manualMovie?.title ?? ''}
        mediaType={manualMovie ? scrapeMediaType(library?.type, manualMovie) : library?.type || 'movie'}
        scopeLabel={manualMovie?.title ?? '当前电影'}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={() => setManualMovie(null)}
        onApplied={reloadCurrentLibrary}
      />
    </div>
  )
}

function scrapeMediaType(libraryType: string | undefined, media: Media): string {
  if ((media.season_num ?? 0) > 0 || (media.episode_num ?? 0) > 0) {
    return 'tv'
  }
  return libraryType || 'movie'
}
