// Package service — NFO writer (Kodi / Jellyfin compatibility).
//
// Kodi and Jellyfin index media using sidecar XML files alongside the
// source video. We export a minimal subset that those scrapers consume
// happily:
//
//   movie.mkv    -> movie.nfo            (<movie>...</movie>)
//   tvshow/      -> tvshow.nfo           (<tvshow>...</tvshow>)  [future]
//
// Today only the per-movie writer is implemented; the per-show / per-episode
// writers are stubbed with TODO markers.
package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// NFOService is the entry point used by the admin "导出 NFO" action.
type NFOService struct {
	log  *zap.Logger
	repo *repository.Container
}

// NewNFOService is the constructor.
func NewNFOService(log *zap.Logger, repo *repository.Container) *NFOService {
	return &NFOService{log: log, repo: repo}
}

// movieNFO is the on-disk schema. Tag names match Kodi's expectations.
type movieNFO struct {
	XMLName  xml.Name `xml:"movie"`
	Title    string   `xml:"title"`
	Original string   `xml:"originaltitle,omitempty"`
	Year     int      `xml:"year,omitempty"`
	Plot     string   `xml:"plot,omitempty"`
	Rating   float32  `xml:"rating,omitempty"`
	Poster   string   `xml:"thumb,omitempty"`
	Fanart   string   `xml:"fanart,omitempty"`
	TMDb     int      `xml:"tmdbid,omitempty"`
}

// ExportOne writes a movie.nfo file next to the media file. Existing files
// are overwritten so a re-scrape always reflects the latest metadata.
func (s *NFOService) ExportOne(ctx context.Context, mediaID string) (string, error) {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", errors.New("media not found")
	}
	if m.Path == "" {
		return "", errors.New("media has empty path")
	}

	doc := movieNFO{
		Title:    m.Title,
		Original: m.OriginalName,
		Year:     m.Year,
		Plot:     m.Overview,
		Rating:   m.Rating,
		Poster:   m.PosterURL,
		Fanart:   m.BackdropURL,
		TMDb:     m.TMDbID,
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	body := []byte(xml.Header + string(out) + "\n")

	dst := nfoPath(m.Path)
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return "", err
	}
	s.log.Info("nfo exported", zap.String("media_id", m.ID), zap.String("path", dst))
	return dst, nil
}

// ExportLibrary loops through every matched movie in a library and writes
// an .nfo for each. Returns (written, error).
func (s *NFOService) ExportLibrary(ctx context.Context, libraryID string) (int, error) {
	type row struct{ ID string }
	var ids []row
	q := s.repo.DB.Table("media").Select("id").Where("scrape_status = ?", "matched")
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	if err := q.Scan(&ids).Error; err != nil {
		return 0, err
	}
	written := 0
	for _, r := range ids {
		if _, err := s.ExportOne(ctx, r.ID); err == nil {
			written++
		}
	}
	return written, nil
}

func nfoPath(media string) string {
	dir := filepath.Dir(media)
	base := strings.TrimSuffix(filepath.Base(media), filepath.Ext(media))
	return filepath.Join(dir, fmt.Sprintf("%s.nfo", base))
}
