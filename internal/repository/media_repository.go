package repository

import (
	"context"
	"errors"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// MediaRepository persists model.Media records.
type MediaRepository struct {
	db *gorm.DB

	searchIndexOnce      sync.Once
	searchIndexAvailable bool
	searchBackend        MediaSearchBackend
}

type MediaSearchBackend interface {
	SearchMediaIDs(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]string, int64, error)
}

type MediaSearchSyncBackend interface {
	MediaSearchBackend
	EnsureIndex(ctx context.Context) error
	IndexMedia(ctx context.Context, rows []model.Media) error
}

func (r *MediaRepository) SetSearchBackend(backend MediaSearchBackend) {
	if r != nil {
		r.searchBackend = backend
	}
}

// MediaQueryFilter is applied to user-facing media queries so NSFW items and
// profile-restricted libraries are filtered in SQL instead of only in React.
type MediaQueryFilter struct {
	IncludeNSFW       bool
	AllowedLibraryIDs []string
	HiddenLibraryIDs  []string
}

func applyMediaQueryFilter(q *gorm.DB, filter MediaQueryFilter) *gorm.DB {
	if !filter.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		q = q.Where("library_id NOT IN ?", filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", filter.AllowedLibraryIDs)
	}
	return q
}

// Upsert inserts or updates a media row keyed by Path (unique index).
//
// 重要：当一条行已经存在时，scanner 重扫只应该刷新文件级元数据
// （时长、宽高、编码、容器、大小），不能把刮削器维护的字段（标题改写、
// 海报、TMDb/Bangumi ID、scrape_status 等）覆盖回零值。
//
// 之前用 Assign(*m).FirstOrCreate(m) 会把整张零值结构体写回，导致：
//  1. scrape_status 从 'matched' / 'no_match' 被清空成 ”；
//  2. 新建行使 GORM `default:pending` 也得不到应用（因为 zero value 被
//     显式写入）。这两个问题都让 EnrichLibrary(WHERE scrape_status='pending')
//     永远捞不到数据。
func (r *MediaRepository) Upsert(ctx context.Context, m *model.Media) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.upsert(ctx, m)
	})
}

func (r *MediaRepository) upsert(ctx context.Context, m *model.Media) error {
	var existing model.Media
	err := r.db.WithContext(ctx).Unscoped().Where("path = ?", m.Path).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 新行：保证 scrape_status 走 GORM default:pending（即留空让数据库填）。
		if m.ScrapeStatus == "" {
			m.ScrapeStatus = "pending"
		}
		if createErr := r.db.WithContext(ctx).Create(m).Error; createErr == nil {
			r.indexMediaBestEffort(ctx, *m)
			return nil
		} else if retryErr := r.db.WithContext(ctx).Unscoped().Where("path = ?", m.Path).First(&existing).Error; retryErr != nil {
			return createErr
		}
	}
	if err != nil {
		return err
	}

	// 已存在：仅刷新文件层面的字段。
	updates := map[string]any{}
	setIfChanged(updates, "size_bytes", existing.SizeBytes, m.SizeBytes)
	setIfChanged(updates, "duration_sec", existing.DurationSec, m.DurationSec)
	setIfChanged(updates, "width", existing.Width, m.Width)
	setIfChanged(updates, "height", existing.Height, m.Height)
	setIfChanged(updates, "video_codec", existing.VideoCodec, m.VideoCodec)
	setIfChanged(updates, "audio_codec", existing.AudioCodec, m.AudioCodec)
	setIfChanged(updates, "container", existing.Container, m.Container)
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	// 回填硬链接身份标识，便于后续扫描去重（避免重复识别/多倍占用）。
	if m.FileID != "" && m.FileID != existing.FileID {
		updates["file_id"] = m.FileID
	}
	if m.Title != "" {
		// scanner 给出的标题只是从路径推导，刮削后 title 已被替换为
		// 真实剧名。仅在 existing 还停留在 'pending'/'' 时回填扫描标题，
		// 避免覆盖刮削结果。
		if m.ScrapeStatus == "matched" || existing.ScrapeStatus == "pending" || existing.ScrapeStatus == "" || existing.ScrapeStatus == "no_match" {
			setIfChanged(updates, "title", existing.Title, m.Title)
			if m.Year > 0 {
				setIfChanged(updates, "year", existing.Year, m.Year)
			}
		}
	}
	status := strings.TrimSpace(existing.ScrapeStatus)
	canRefreshExternalIDs := status == "pending" || status == "" || status == "no_match" ||
		m.ScrapeStatus == "matched" || strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://")
	if canRefreshExternalIDs {
		changedExternalID := false
		if m.TMDbID > 0 && existing.TMDbID != m.TMDbID {
			updates["tm_db_id"] = m.TMDbID
			changedExternalID = true
		}
		if m.BangumiID > 0 && existing.BangumiID != m.BangumiID {
			updates["bangumi_id"] = m.BangumiID
			changedExternalID = true
		}
		if m.DoubanID != "" && strings.TrimSpace(existing.DoubanID) != strings.TrimSpace(m.DoubanID) {
			updates["douban_id"] = m.DoubanID
			changedExternalID = true
		}
		if m.TheTVDBID != "" && strings.TrimSpace(existing.TheTVDBID) != strings.TrimSpace(m.TheTVDBID) {
			updates["thetvdb_id"] = m.TheTVDBID
			changedExternalID = true
		}
		if m.Year > 0 && existing.Year <= 0 {
			updates["year"] = m.Year
		}
		if changedExternalID && (status == "no_match" || status == "matched") && m.ScrapeStatus != "matched" {
			updates["scrape_status"] = "pending"
		}
	}
	if m.ScrapeStatus == "matched" {
		setIfChanged(updates, "scrape_status", existing.ScrapeStatus, m.ScrapeStatus)
		if m.OriginalName != "" {
			setIfChanged(updates, "original_name", existing.OriginalName, m.OriginalName)
		}
		if m.EpisodeTitle != "" {
			setIfChanged(updates, "episode_title", existing.EpisodeTitle, m.EpisodeTitle)
		}
		if m.PosterURL != "" {
			setIfChanged(updates, "poster_url", existing.PosterURL, m.PosterURL)
		}
		if m.BackdropURL != "" {
			setIfChanged(updates, "backdrop_url", existing.BackdropURL, m.BackdropURL)
		}
		if m.Overview != "" {
			setIfChanged(updates, "overview", existing.Overview, m.Overview)
		}
		if m.Rating > 0 {
			setIfChanged(updates, "rating", existing.Rating, m.Rating)
		}
		if m.Year > 0 {
			setIfChanged(updates, "year", existing.Year, m.Year)
		}
		if m.TMDbID > 0 {
			setIfChanged(updates, "tm_db_id", existing.TMDbID, m.TMDbID)
		}
		if m.BangumiID > 0 {
			setIfChanged(updates, "bangumi_id", existing.BangumiID, m.BangumiID)
		}
		if m.DoubanID != "" {
			setIfChanged(updates, "douban_id", existing.DoubanID, m.DoubanID)
		}
		if m.TheTVDBID != "" {
			setIfChanged(updates, "thetvdb_id", existing.TheTVDBID, m.TheTVDBID)
		}
		if m.Languages != "" {
			setIfChanged(updates, "languages", existing.Languages, m.Languages)
		}
		if m.Countries != "" {
			setIfChanged(updates, "countries", existing.Countries, m.Countries)
		}
		if m.Genres != "" {
			setIfChanged(updates, "genres", existing.Genres, m.Genres)
		}
		if m.NSFW && !existing.NSFW {
			updates["nsfw"] = true
		}
	}
	if m.PosterURL != "" {
		setIfChanged(updates, "poster_url", existing.PosterURL, m.PosterURL)
	}
	if m.BackdropURL != "" {
		setIfChanged(updates, "backdrop_url", existing.BackdropURL, m.BackdropURL)
	}
	// 云盘媒体：同一 cloud:// 文件可能先被父目录库扫描入库，之后用户按二级
	// 分类重新挂载/扫描到更精确的分类库。此时让 library_id 迁移到当前扫描库，
	// 否则媒体被钉死在旧库、新分类库里看不到(表现为"媒体部分消失")。
	// 本地媒体物理位置固定：仅在原 library_id 为空时回填，不迁移。
	if isCloudMediaPath := strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://"); m.LibraryID != "" && m.LibraryID != existing.LibraryID {
		if isCloudMediaPath || existing.LibraryID == "" {
			updates["library_id"] = m.LibraryID
		}
	}
	if (m.SeasonNum > 0 || m.EpisodeNum > 0) && existing.SeasonNum != m.SeasonNum {
		updates["season_num"] = m.SeasonNum
	}
	if m.EpisodeNum > 0 && existing.EpisodeNum != m.EpisodeNum {
		updates["episode_num"] = m.EpisodeNum
	}
	if m.STRMURL != "" {
		setIfChanged(updates, "strm_url", existing.STRMURL, m.STRMURL)
	}

	if len(updates) == 0 {
		*m = existing
		return nil
	}
	if err := r.db.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return err
	}
	// 回写 ID / 不可变字段，让 caller 拿到完整的现有行。
	*m = existing
	if fresh, err := r.FindByID(ctx, existing.ID); err == nil && fresh != nil {
		r.indexMediaBestEffort(ctx, *fresh)
	}
	return nil
}

func (r *MediaRepository) indexMediaBestEffort(ctx context.Context, media model.Media) {
	backend, ok := r.searchBackend.(MediaSearchSyncBackend)
	if !ok {
		return
	}
	_ = backend.IndexMedia(ctx, []model.Media{media})
}

func setIfChanged[T comparable](updates map[string]any, key string, current, next T) {
	if current != next {
		updates[key] = next
	}
}

// FindByID returns the media row or (nil, nil).
func (r *MediaRepository) FindByID(ctx context.Context, id string) (*model.Media, error) {
	var m model.Media
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByLibrary returns paginated media items for a library.
func (r *MediaRepository) ListByLibrary(ctx context.Context, libraryID string, offset, limit int) ([]model.Media, int64, error) {
	return r.ListByLibraryFiltered(ctx, libraryID, offset, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) ListByLibraryFiltered(ctx context.Context, libraryID string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	return r.ListByLibrariesFiltered(ctx, []string{libraryID}, offset, limit, filter)
}

func (r *MediaRepository) ListByLibrariesFiltered(ctx context.Context, libraryIDs []string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	var items []model.Media
	var total int64
	if len(libraryIDs) == 0 {
		return items, 0, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{})
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	// 多级排序消除"随机"观感:
	//  1. year desc        — 上映年份新→旧(用户期望的上映时间维度)
	//  2. updated_at desc  — 同年按最近更新(刮削/补集会刷新)
	//  3. created_at desc  — 再按入库时间
	//  4. id desc          — 稳定 tie-breaker:云盘批量扫描同批 created_at 相同时,
	//                        没有它 DB 返回顺序不确定,正是"随机排序"的根因。
	err := q.Order("year DESC, updated_at DESC, created_at DESC, id DESC").
		Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

// DeleteByLibrary purges all media tied to a library.
func (r *MediaRepository) DeleteByLibrary(ctx context.Context, libraryID string) error {
	// FTS 行由 media 表上的触发器同步清理（软删/硬删都覆盖）。
	return r.db.WithContext(ctx).Where("library_id = ?", libraryID).Delete(&model.Media{}).Error
}

// PurgeByLibrary permanently removes media tied to a library. Used for virtual
// cloud mounts where "remove mount" must not populate the recycle bin.
func (r *MediaRepository) PurgeByLibrary(ctx context.Context, libraryID string) error {
	return r.db.WithContext(ctx).Unscoped().Where("library_id = ?", libraryID).Delete(&model.Media{}).Error
}
