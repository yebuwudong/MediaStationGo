// Package service — scraper orchestrator.
//
// ScraperService takes a Media row and tries to enrich it with metadata from
// local NFO first, then TMDb -> Douban -> Bangumi -> TheTVDB. Fanart.tv is
// artwork-only and upgrades poster/backdrop after a metadata match.
package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ScraperService coordinates metadata enrichment across providers.
type ScraperService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	tmdb    *TMDbProvider
	bangumi *BangumiProvider
	thetvdb *TheTVDBProvider
	douban  *DoubanProvider
	fanart  *FanartProvider
	adult   *AdultProvider
	hub     *Hub
	notify  *NotifyChannelService
	cache   *RuntimeCacheService
	images  *ImageProxy
}

// NewScraperService is the constructor.
func NewScraperService(
	cfg *config.Config,
	log *zap.Logger,
	repo *repository.Container,
	tmdb *TMDbProvider,
	bangumi *BangumiProvider,
	thetvdb *TheTVDBProvider,
	fanart *FanartProvider,
	hub *Hub,
	adult ...*AdultProvider,
) *ScraperService {
	var adultProvider *AdultProvider
	if len(adult) > 0 {
		adultProvider = adult[0]
	}
	return &ScraperService{
		cfg: cfg, log: log, repo: repo,
		tmdb: tmdb, bangumi: bangumi, thetvdb: thetvdb, fanart: fanart, adult: adultProvider, hub: hub,
	}
}

func (s *ScraperService) SetDouban(douban *DoubanProvider) {
	s.douban = douban
}

func (s *ScraperService) SetNotifyChannels(notify *NotifyChannelService) {
	if s != nil {
		s.notify = notify
	}
}

func (s *ScraperService) SetRuntimeCache(cache *RuntimeCacheService) *ScraperService {
	if s != nil {
		s.cache = cache
	}
	return s
}

func (s *ScraperService) SetImageProxy(images *ImageProxy) *ScraperService {
	if s != nil {
		s.images = images
	}
	return s
}

var tmdbDetailsTimeout = 8 * time.Second

const (
	defaultScrapeDelayMinMS = 250
	defaultScrapeDelayMaxMS = 500
	maxScrapeDelayMS        = 5 * 60 * 1000
)

// EnrichOne runs the provider chain for a single media row.
func (s *ScraperService) EnrichOne(ctx context.Context, m *model.Media) error {
	return s.EnrichOneWithOptions(ctx, m, ScrapeOptions{})
}

func (s *ScraperService) EnrichOneWithOptions(ctx context.Context, m *model.Media, options ScrapeOptions) error {
	lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil {
		return err
	}

	seriesLike := mediaIsEpisodic(m, lib)
	cloudMedia := isCloudMediaPath(m.Path) || (lib != nil && isCloudMediaPath(lib.Path))
	var local *LocalMetadata
	if !cloudMedia {
		if found, err := ReadLocalMetadata(m.Path, lib.Path, seriesLike); err == nil && found != nil {
			local = found
		} else if err != nil {
			s.log.Warn("read local metadata before scrape failed", zap.String("media_id", m.ID), zap.Error(err))
		}
	}
	if hinted, _ := pathHintMetadata(m.Path, seriesLike); hinted != nil {
		local = mergeScrapePathHintMetadata(local, hinted)
	}
	if local != nil {
		applyLocalMetadata(m, local)
	}

	year := mediaYearHint(m)

	if s.adult != nil && s.adult.Enabled() {
		if code := firstText(localAdultCode(local), AdultCodeFromMediaPath(m.Path), normalizeAdultCode(m.OriginalName), normalizeAdultCode(m.Title)); code != "" {
			if adultMatch, err := s.adult.Search(ctx, code); err == nil && adultMatch != nil {
				mergeLocalMetadataIntoMatch(adultMatch, local)
				return s.applyProviderMatchWithOptions(ctx, m, lib, adultMatch, options)
			} else if err != nil {
				s.log.Debug("adult metadata search failed", zap.String("media_id", m.ID), zap.String("code", code), zap.Error(err))
			}
		}
	}

	if match := s.matchFromMediaExternalIDs(ctx, m, lib); match != nil {
		s.applyFanartArtwork(ctx, match)
		mergeLocalMetadataIntoMatch(match, local)
		return s.applyProviderMatchWithOptions(ctx, m, lib, match, options)
	}

	candidates := scrapeQueryCandidates(m, lib)
	var query string
	match := (*Match)(nil)
	for _, candidate := range candidates {
		query = candidate
		candidateMatch := s.lookup(ctx, lib, m, candidate, year)
		if candidateMatch == nil {
			continue
		}
		if !organizeMetadataMatchTrusted(candidate, year, candidateMatch) {
			s.log.Warn("metadata scrape match rejected",
				zap.String("media_id", m.ID),
				zap.String("query", candidate),
				zap.String("title", candidateMatch.Title),
				zap.Int("source_year", year),
				zap.Int("match_year", candidateMatch.Year),
				zap.Int("tmdb_id", candidateMatch.TMDbID),
				zap.Int("bangumi_id", candidateMatch.BangumiID),
				zap.String("douban_id", candidateMatch.DoubanID),
				zap.String("thetvdb_id", candidateMatch.TheTVDBID))
			continue
		}
		preferLocalizedSearchTitle(candidate, candidateMatch)
		match = candidateMatch
		if match != nil {
			break
		}
	}
	if match == nil {
		if local != nil && !local.PathHint {
			return s.applyLocalMetadataMatch(ctx, m, local)
		}
		_ = s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
			Update("scrape_status", "no_match").Error
		s.invalidateMediaCache(ctx)
		s.log.Info("metadata scrape no match",
			zap.String("media_id", m.ID),
			zap.String("query", query),
			zap.String("library_type", lib.Type))
		return nil
	}
	s.applyFanartArtwork(ctx, match)
	mergeLocalMetadataIntoMatch(match, local)

	return s.applyProviderMatchWithOptions(ctx, m, lib, match, options)
}

func (s *ScraperService) applyProviderMatch(ctx context.Context, m *model.Media, lib *model.Library, match *Match) error {
	return s.applyProviderMatchWithOptions(ctx, m, lib, match, ScrapeOptions{})
}

func (s *ScraperService) applyProviderMatchWithOptions(ctx context.Context, m *model.Media, lib *model.Library, match *Match, options ScrapeOptions) error {
	posterCandidate := match.PosterURL
	backdropCandidate := match.BackdropURL
	posterURL, removePoster := s.prepareScrapedArtworkURL(ctx, m.ID, "poster_url", m.PosterURL, posterCandidate)
	backdropURL, removeBackdrop := s.prepareScrapedArtworkURL(ctx, m.ID, "backdrop_url", m.BackdropURL, backdropCandidate)
	updates := map[string]any{
		"title":         match.Title,
		"overview":      match.Overview,
		"poster_url":    posterURL,
		"backdrop_url":  backdropURL,
		"rating":        match.Rating,
		"year":          match.Year,
		"scrape_status": "matched",
	}
	if match.OriginalName != "" {
		updates["original_name"] = match.OriginalName
	}
	if strings.TrimSpace(m.EpisodeTitle) != "" {
		updates["episode_title"] = strings.TrimSpace(m.EpisodeTitle)
	}
	if match.TMDbID > 0 {
		updates["tm_db_id"] = match.TMDbID
	}
	if match.BangumiID > 0 {
		updates["bangumi_id"] = match.BangumiID
	}
	if match.DoubanID != "" {
		updates["douban_id"] = match.DoubanID
	}
	if match.TheTVDBID != "" {
		updates["thetvdb_id"] = match.TheTVDBID
	}
	if match.NSFW {
		updates["nsfw"] = true
	}
	if len(match.Genres) > 0 {
		updates["genres"] = strings.Join(match.Genres, ",")
	}
	if len(match.Countries) > 0 {
		updates["countries"] = strings.Join(match.Countries, ",")
	}
	if len(match.Languages) > 0 {
		updates["languages"] = strings.Join(match.Languages, ",")
	}

	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		return err
	}
	s.removeCachedScrapedArtwork(removePoster, removeBackdrop)

	// Fetch extended metadata after the selected match is already saved.
	// Manual cloud/batch applies must not fail just because an optional provider
	// details request is slow or unavailable.
	if match.TMDbID > 0 && s.tmdb != nil && s.tmdb.Enabled() {
		mediaType := s.determineMediaTypeForMedia(lib, m, match)
		s.fetchAndSaveTMDbExtendedMetadata(ctx, m.ID, match.TMDbID, mediaType)
		if mediaType == "tv" && !options.DeferEpisodeDetails {
			s.fetchAndSaveTMDbEpisodeDetails(ctx, m, match.TMDbID, match.Year, options)
		}
	}
	if !(options.DeferEpisodeDetails && m != nil && m.EpisodeNum > 0) {
		s.writeMediaNFOAfterScrape(ctx, m, lib)
	}
	s.invalidateMediaCache(ctx)
	s.hub.Publish("scrape", map[string]any{
		"media_id":   m.ID,
		"title":      match.Title,
		"tmdb_id":    match.TMDbID,
		"bangumi_id": match.BangumiID,
		"douban_id":  match.DoubanID,
		"thetvdb_id": match.TheTVDBID,
		"source":     map[bool]string{true: "adult"}[match.NSFW],
	})
	return nil
}

// AnyEnabled reports whether at least one provider can run.
func (s *ScraperService) AnyEnabled() bool {
	if s.tmdb != nil && s.tmdb.Enabled() {
		return true
	}
	if s.bangumi != nil && s.bangumi.Enabled() {
		return true
	}
	if s.thetvdb != nil && s.thetvdb.Enabled() {
		return true
	}
	if s.adult != nil && s.adult.Enabled() {
		return true
	}
	if s.douban != nil && s.douban.Enabled() {
		return true
	}
	return false
}

// determineMediaType returns "tv" for TV shows and "movie" for movies.
// It uses the library type as the primary signal.
func (s *ScraperService) determineMediaType(lib *model.Library, match *Match) string {
	return s.determineMediaTypeForMedia(lib, nil, match)
}

func (s *ScraperService) determineMediaTypeForMedia(lib *model.Library, media *model.Media, match *Match) string {
	if match != nil {
		switch normalizeOrganizeMediaType(match.MediaType) {
		case "tv", "anime", "variety":
			return "tv"
		case "movie", "adult":
			return "movie"
		}
	}
	if media != nil && mediaIsEpisodic(media, lib) {
		return "tv"
	}
	if lib != nil {
		switch lib.Type {
		case "tv", "anime", "variety", "show", "shows":
			return "tv"
		}
	}
	// Fallback: if Bangumi ID is present, treat as TV/anime
	if match != nil && match.BangumiID > 0 {
		return "tv"
	}
	return "movie"
}
