// Package service — minimal Emby/Jellyfin compatibility shim.
//
// EmbyService produces JSON envelopes shaped like the most-consumed
// Emby-API endpoints so existing players (Infuse / Kodi NextPVR
// extension / iOS native clients) can talk to MediaStationGo without a
// custom plugin.
//
// Implemented surface (matches what nowen-video exposes):
//
//	GET /emby/System/Info               server identity
//	GET /emby/Users                     list of users (admin only field)
//	GET /emby/Users/{userId}/Views      virtual root: one entry per library
//	GET /emby/Users/{userId}/Items      paginated media listing
//	GET /emby/Items/{id}                single item
//	GET /emby/Items/{id}/PlaybackInfo   stream URL (delegates to /api/stream)
//
// The shim is read-only — Emby write operations (mark watched, etc.) are
// not implemented; the React UI stays the canonical control plane.
package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// EmbyService produces Emby-shaped JSON.
type EmbyService struct {
	cfg  *config.Config
	log  *zap.Logger
	repo *repository.Container
}

// NewEmbyService is the constructor.
func NewEmbyService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *EmbyService {
	return &EmbyService{cfg: cfg, log: log, repo: repo}
}

// SystemInfo returns the Emby identity payload.
func (e *EmbyService) SystemInfo() map[string]any {
	return map[string]any{
		"ServerName":      "MediaStationGo",
		"Version":         "0.1.0",
		"Id":              "mediastation-go",
		"OperatingSystem": "Linux",
		"ProductName":     "MediaStationGo",
	}
}

// ListUsers returns Emby-shaped users.
func (e *EmbyService) ListUsers(ctx context.Context) ([]map[string]any, error) {
	users, err := e.repo.User.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, e.userPayload(&u))
	}
	return out, nil
}

func (e *EmbyService) userPayload(u *model.User) map[string]any {
	return map[string]any{
		"Id":                  u.ID,
		"Name":                u.Username,
		"ServerId":            "mediastation-go",
		"HasPassword":         true,
		"HasConfiguredEasyPassword": false,
		"Policy": map[string]any{
			"IsAdministrator":      u.Role == "admin",
			"IsHidden":             false,
			"IsDisabled":           false,
			"EnableUserPreferenceAccess": true,
		},
	}
}

// Views (Emby's name for libraries).
func (e *EmbyService) Views(ctx context.Context) (map[string]any, error) {
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(libs))
	for _, l := range libs {
		collectionType := "movies"
		if l.Type == "tv" {
			collectionType = "tvshows"
		} else if l.Type == "music" {
			collectionType = "music"
		}
		items = append(items, map[string]any{
			"Id":             l.ID,
			"Name":           l.Name,
			"CollectionType": collectionType,
			"ServerId":       "mediastation-go",
			"Type":           "CollectionFolder",
		})
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items)}, nil
}

// Items paginates media in Emby's flat shape.
func (e *EmbyService) Items(ctx context.Context, libraryID string, limit, offset int) (map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.Media
	if err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, e.itemPayload(&m))
	}
	return map[string]any{
		"Items":            items,
		"TotalRecordCount": total,
		"StartIndex":       offset,
	}, nil
}

func (e *EmbyService) itemPayload(m *model.Media) map[string]any {
	itemType := "Movie"
	if m.SeasonNum > 0 || m.EpisodeNum > 0 {
		itemType = "Episode"
	}
	return map[string]any{
		"Id":                m.ID,
		"Name":              m.Title,
		"ServerId":          "mediastation-go",
		"Type":              itemType,
		"ProductionYear":    m.Year,
		"ParentIndexNumber": m.SeasonNum,
		"IndexNumber":       m.EpisodeNum,
		"Overview":          m.Overview,
		"RunTimeTicks":      int64(m.DurationSec) * 10_000_000,
		"CommunityRating":   m.Rating,
		"MediaSources": []map[string]any{{
			"Id":        m.ID,
			"Path":      m.Path,
			"Container": m.Container,
			"Size":      m.SizeBytes,
		}},
	}
}

// PlaybackInfo returns the stream URL (caller must append ?token=).
func (e *EmbyService) PlaybackInfo(ctx context.Context, mediaID string) (map[string]any, error) {
	m, err := e.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return nil, err
	}
	url := "/api/stream/" + m.ID
	if m.STRMURL != "" {
		url = m.STRMURL
	}
	return map[string]any{
		"MediaSources": []map[string]any{{
			"Id":            m.ID,
			"Path":          url,
			"Protocol":      "Http",
			"DirectStreamUrl": url,
			"Container":     m.Container,
			"Size":          m.SizeBytes,
		}},
		"PlaySessionId": m.ID,
	}, nil
}
