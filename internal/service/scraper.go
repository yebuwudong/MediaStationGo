// Package service — scraper orchestrator.
//
// ScraperService takes a Media row and tries to enrich it with metadata
// from one or more providers. Selection is driven by the library type:
//
//   library.type == "anime"   -> Bangumi  (fallback: TMDb)
//   library.type == "tv"      -> TheTVDB  (fallback: TMDb)
//   default                   -> TMDb
//
// After the primary match we optionally upgrade poster / backdrop with
// Fanart.tv when an API key is configured.
package service

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
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
	fanart  *FanartProvider
	hub     *Hub
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
) *ScraperService {
	return &ScraperService{
		cfg: cfg, log: log, repo: repo,
		tmdb: tmdb, bangumi: bangumi, thetvdb: thetvdb, fanart: fanart, hub: hub,
	}
}

// yearPattern extracts a 4-digit year (1900-2099).
var yearPattern = regexp.MustCompile(`(?:^|[^\d])(19\d{2}|20\d{2})(?:[^\d]|$)`)

// noiseTokens are stripped before search.
var noiseTokens = []string{
	"1080p", "2160p", "4k", "720p", "480p",
	"hdrip", "bluray", "blu-ray", "webrip", "web-dl", "web",
	"x264", "x265", "h264", "h265", "hevc", "avc",
	"hdr", "sdr", "dts", "ddp", "atmos", "aac", "ac3", "flac",
	"remux", "extended", "uncut", "directors-cut", "directors_cut",
	"hkfree", "yify", "rarbg", "ettv", "fgt",
}

// bracketedTag matches "[anything]" or "(anything)" segments.
var bracketedTag = regexp.MustCompile(`[\[\(][^\]\)]*[\]\)]`)

// CleanQuery converts a filename like "Inception.2010.1080p.BluRay.x264.mkv"
// into a TMDb-friendly title plus an optional year hint.
func CleanQuery(raw string) (title string, year int) {
	name := strings.TrimSuffix(filepath.Base(raw), filepath.Ext(raw))
	lower := strings.ToLower(name)

	if m := yearPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			year = v
			lower = strings.ReplaceAll(lower, m[1], " ")
		}
	}

	lower = bracketedTag.ReplaceAllString(lower, " ")

	lower = patSEnE.ReplaceAllString(lower, " ")
	lower = patNxE.ReplaceAllString(lower, " ")
	lower = patEP.ReplaceAllString(lower, " ")
	lower = patCN.ReplaceAllString(lower, " ")

	for _, t := range noiseTokens {
		lower = strings.ReplaceAll(lower, t, " ")
	}
	for _, sep := range []string{".", "_", "-", "[", "]", "(", ")"} {
		lower = strings.ReplaceAll(lower, sep, " ")
	}
	fields := strings.Fields(lower)
	title = strings.Join(fields, " ")
	return strings.TrimSpace(title), year
}

// EnrichOne runs the provider chain for a single media row.
func (s *ScraperService) EnrichOne(ctx context.Context, m *model.Media) error {
	lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil {
		return err
	}

	query := m.Title
	if query == "" {
		query, _ = CleanQuery(m.Path)
	} else {
		query, _ = CleanQuery(query)
	}
	year := m.Year
	if year == 0 {
		_, year = CleanQuery(filepath.Base(m.Path))
	}

	match := s.lookup(ctx, lib, query, year)
	if match == nil {
		_ = s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
			Update("scrape_status", "no_match").Error
		return nil
	}

	// Optional Fanart upgrade.
	if s.fanart != nil && s.fanart.Enabled() && match.TMDbID > 0 {
		if a, err := s.fanart.MovieArtwork(ctx, match.TMDbID); err == nil && a != nil {
			if a.Poster != "" {
				match.PosterURL = a.Poster
			}
			if a.Backdrop != "" {
				match.BackdropURL = a.Backdrop
			}
		}
	}

	updates := map[string]any{
		"title":         match.Title,
		"overview":      match.Overview,
		"poster_url":    match.PosterURL,
		"backdrop_url":  match.BackdropURL,
		"rating":        match.Rating,
		"year":          match.Year,
		"scrape_status": "matched",
	}
	if match.TMDbID > 0 {
		updates["tmdb_id"] = match.TMDbID
	}
	if match.BangumiID > 0 {
		updates["bangumi_id"] = match.BangumiID
	}
	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		return err
	}
	s.hub.Publish("scrape", map[string]any{
		"media_id":   m.ID,
		"title":      match.Title,
		"tmdb_id":    match.TMDbID,
		"bangumi_id": match.BangumiID,
	})
	return nil
}

// lookup runs the provider chain. When the library is missing we fall
// back to TMDb only.
func (s *ScraperService) lookup(ctx context.Context, lib *model.Library, query string, year int) *Match {
	kind := ""
	if lib != nil {
		kind = lib.Type
	}
	switch kind {
	case "anime":
		if s.bangumi != nil {
			if m, err := s.bangumi.Search(ctx, query); err == nil && m != nil {
				return m
			}
		}
	case "tv":
		if s.thetvdb != nil && s.thetvdb.Enabled() {
			if m, err := s.thetvdb.SearchSeries(ctx, query); err == nil && m != nil {
				return m
			}
		}
	}
	if s.tmdb != nil && s.tmdb.Enabled() {
		if m, err := s.tmdb.SearchMovie(ctx, query, year); err == nil && m != nil {
			return m
		}
	}
	return nil
}

// EnrichLibrary runs the provider chain for every "pending" media in a
// library. It throttles to 4 RPS and publishes a summary event when done.
func (s *ScraperService) EnrichLibrary(ctx context.Context, libraryID string) (int, error) {
	var rows []model.Media
	q := s.repo.DB.Where("scrape_status = ?", "pending")
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return 0, err
	}
	matched := 0
	for i := range rows {
		select {
		case <-ctx.Done():
			return matched, ctx.Err()
		default:
		}
		if err := s.EnrichOne(ctx, &rows[i]); err != nil {
			s.log.Warn("enrich failed", zap.String("media", rows[i].ID), zap.Error(err))
			continue
		}
		matched++
		time.Sleep(250 * time.Millisecond) // ~4 RPS
	}
	s.hub.Publish("scrape", map[string]any{
		"library_id": libraryID,
		"finished":   true,
		"matched":    matched,
	})
	return matched, nil
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
	return false
}
