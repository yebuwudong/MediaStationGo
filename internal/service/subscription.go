// Package service — RSS subscriptions for automated downloads.
//
// SubscriptionService periodically polls every Subscription row, fetches
// the configured RSS / Atom feed, and queues new items into the
// DownloadService. Items are deduplicated by GUID stored as a Setting key
// "subscription.<id>.last_guid" so the same episode is never re-queued.
package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SubscriptionService runs the polling loop.
type SubscriptionService struct {
	cfg       *config.Config
	log       *zap.Logger
	repo      *repository.Container
	downloads *DownloadService
	site      *SiteService
	scraper   *ScraperService
	hub       *Hub
	notify    *NotifyChannelService
	stop      chan struct{}
}

// NewSubscriptionService is the constructor.
func NewSubscriptionService(cfg *config.Config, log *zap.Logger, repo *repository.Container, downloads *DownloadService, site *SiteService, hub *Hub) *SubscriptionService {
	return &SubscriptionService{
		cfg:       cfg,
		log:       log,
		repo:      repo,
		downloads: downloads,
		site:      site,
		hub:       hub,
		stop:      make(chan struct{}),
	}
}

func (s *SubscriptionService) SetScraper(scraper *ScraperService) {
	s.scraper = scraper
}

func (s *SubscriptionService) SetNotifyChannels(notify *NotifyChannelService) {
	s.notify = notify
}

// Start runs the polling loop in the background.
func (s *SubscriptionService) Start(ctx context.Context) {
	go s.loop(ctx)
}

// Stop shuts the loop down.
func (s *SubscriptionService) Stop() { close(s.stop) }

// rssFeed is the minimal RSS subset we need to decode.
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Enclosure   struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
}

// Create persists a new subscription.
func (s *SubscriptionService) Create(ctx context.Context, sub *model.Subscription) error {
	if sub.Name == "" || sub.FeedURL == "" {
		return errors.New("name and feed_url required")
	}
	normalizeSubscriptionDefaults(sub)
	enabled := sub.Enabled
	if err := s.repo.Subscription.Create(ctx, sub); err != nil {
		return err
	}
	if !enabled {
		if err := s.repo.DB.WithContext(ctx).Model(sub).Update("enabled", false).Error; err != nil {
			return err
		}
		sub.Enabled = false
	}
	return nil
}

func normalizeSubscriptionDefaults(sub *model.Subscription) {
	if strings.TrimSpace(sub.SearchMode) == "" {
		sub.SearchMode = "keyword"
	}
	if strings.TrimSpace(sub.Resolution) == "" {
		sub.Resolution = "best"
	}
	if strings.TrimSpace(sub.WashPriority) == "" {
		sub.WashPriority = "balanced"
	}
	if sub.Priority == 0 {
		sub.Priority = 50
	}
}

// List returns every subscription rule.
func (s *SubscriptionService) List(ctx context.Context) ([]model.Subscription, error) {
	return s.repo.Subscription.List(ctx)
}

// History returns completed/archived subscription rules.
func (s *SubscriptionService) History(ctx context.Context) ([]model.Subscription, error) {
	return s.repo.Subscription.History(ctx)
}

// Restore moves an archived subscription back to the active management list.
// It also clears the per-subscription seen state so an unfinished historical
// rule can match resources again when it is run next.
func (s *SubscriptionService) Restore(ctx context.Context, id string) (*model.Subscription, error) {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		return nil, err
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"enabled":        true,
			"archive_reason": "",
			// 重置为 0:此前可能被 feed 低估并锁死(updateSubscriptionTotalEpisodes
			// 只增不减,resolveSubscriptionTotalEpisodes 见 >0 即不再回查元数据)。
			// 归零后下次 run 会从 TMDb/豆瓣等权威源重算真实总集数,避免恢复后
			// 因"误判已无缺集"而不再搜索资源。
			"total_episodes": 0,
		}).Error; err != nil {
		return nil, err
	}
	if err := s.repo.DB.WithContext(ctx).
		Exec("UPDATE subscriptions SET archived_at = NULL WHERE id = ?", id).Error; err != nil {
		return nil, err
	}
	if s.repo.Setting != nil {
		_ = s.repo.Setting.Delete(ctx, fmt.Sprintf("subscription.%s.seen", id))
	}
	var restored model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&restored).Error; err != nil {
		return nil, err
	}
	return &restored, nil
}

// Delete removes a subscription.
func (s *SubscriptionService) Delete(ctx context.Context, id string) error {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return s.repo.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Subscription{}).Error
	}
	if err := s.deleteSubscriptionDownloads(ctx, &sub); err != nil {
		return err
	}
	if s.repo.Setting != nil {
		_ = s.repo.Setting.Delete(ctx, fmt.Sprintf("subscription.%s.seen", id))
	}
	return s.repo.DB.Where("id = ?", id).Delete(&model.Subscription{}).Error
}

// RunNow forces a poll for one subscription, ignoring its schedule. Used
// by the admin UI's "test now" button.
func (s *SubscriptionService) RunNow(ctx context.Context, id string) (int, error) {
	var sub model.Subscription
	if err := s.repo.DB.Where("id = ?", id).First(&sub).Error; err != nil {
		return 0, err
	}
	if sub.ArchivedAt != nil {
		return 0, nil
	}
	return s.runOne(ctx, &sub)
}

// loop polls every 10 minutes.
func (s *SubscriptionService) loop(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	// First run shortly after startup.
	first := time.NewTimer(30 * time.Second)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-first.C:
		case <-t.C:
		}
		s.runAll(ctx)
	}
}

func (s *SubscriptionService) runAll(ctx context.Context) {
	subs, err := s.repo.Subscription.List(ctx)
	if err != nil {
		s.log.Warn("subscription list failed", zap.Error(err))
		return
	}
	for i := range subs {
		if !subs[i].Enabled {
			continue
		}
		if n, err := s.runOne(ctx, &subs[i]); err != nil {
			s.log.Warn("subscription run failed",
				zap.String("name", subs[i].Name), zap.Error(err))
		} else if n > 0 {
			s.log.Info("subscription queued items",
				zap.String("name", subs[i].Name), zap.Int("count", n))
		}
	}
}

func (s *SubscriptionService) runOne(ctx context.Context, sub *model.Subscription) (int, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sub.FeedURL)), "site-search://") {
		return s.runSiteSearch(ctx, sub)
	}

	feed, err := s.fetch(ctx, sub.FeedURL)
	if err != nil {
		return 0, err
	}

	filter := compileFilter(sub.Filter)
	guidKey := fmt.Sprintf("subscription.%s.seen", sub.ID)
	seenRaw, _ := s.repo.Setting.Get(ctx, guidKey)
	seen := splitNonEmpty(seenRaw)
	seenSet := make(map[string]struct{}, len(seen))
	for _, g := range seen {
		seenSet[g] = struct{}{}
	}

	washOff := !sub.WashEnabled
	s.updateSubscriptionTotalEpisodes(ctx, sub, s.resolveSubscriptionTotalEpisodes(ctx, sub, inferRSSTotalEpisodes(feed.Channel.Items, sub, filter)))
	availQuery := availabilityQuery(subscriptionName(sub), subscriptionFilter(sub))
	// RSS 和站点搜索统一使用候选规划：先按订阅规则过滤，再按洗版优先级/集数去重择优。
	// 非洗版订阅成功下载一次即满足，媒体库与下载中任务会作为可用性输入避免重复下载。
	avail := mergeLocalAvailability(
		SubscriptionLocalAvailability(ctx, s.repo, sub),
		s.pendingDownloadAvailability(ctx, sub),
	)
	candidates := selectRSSSubscriptionCandidates(feed.Channel.Items, sub, filter, seenSet, avail)

	queued := 0
	for _, candidate := range candidates {
		item := candidate.Item
		guid := candidate.GUID
		download := candidate.Download
		mediaType, mediaCategory := s.classifySubscriptionItem(ctx, sub, item.Title, "")
		savePath := s.resolveSubscriptionSavePath(ctx, sub, mediaType, mediaCategory)
		if s.downloadPathHasCandidate(ctx, sub, item.Title, savePath) {
			if washOff {
				addAvailabilityTitle(item.Title, availQuery, &avail)
			}
			continue
		}
		if _, err := s.downloads.AddDownloadWithMeta(ctx, sub.UserID, download, savePath, DownloadTaskMeta{
			SubscriptionID:       sub.ID,
			Title:                firstNonEmpty(item.Title, sub.Name),
			PosterURL:            sub.PosterURL,
			BackdropURL:          sub.BackdropURL,
			Overview:             sub.Overview,
			MediaType:            mediaType,
			MediaCategory:        mediaCategory,
			AllowExistingLibrary: sub.WashEnabled,
		}); err != nil {
			if IsDownloadDedupError(err) {
				if washOff {
					addAvailabilityTitle(item.Title, availQuery, &avail)
				}
				seen = append(seen, guid)
				seenSet[guid] = struct{}{}
				continue
			}
			s.log.Warn("subscription enqueue failed",
				zap.String("title", item.Title),
				zap.String("media_type", mediaType),
				zap.String("media_category", mediaCategory),
				zap.String("save_path", savePath),
				zap.Error(err))
			continue
		}
		if washOff {
			addAvailabilityTitle(item.Title, availQuery, &avail)
		}
		queued++
		seen = append(seen, guid)
		seenSet[guid] = struct{}{}
	}
	avail = s.finalizePendingAvailability(sub, avail)
	// Remember the last 200 GUIDs so the seen set doesn't grow forever.
	if len(seen) > 200 {
		seen = seen[len(seen)-200:]
	}
	_ = s.repo.Setting.Set(ctx, guidKey, strings.Join(seen, "\n"))

	now := time.Now()
	_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
	_ = s.archiveCompletedSubscription(ctx, sub, avail)
	if queued > 0 {
		s.hub.Publish("subscription", map[string]any{
			"id":     sub.ID,
			"name":   sub.Name,
			"queued": queued,
		})
		s.notifySubscriptionHit(sub, queued, nil)
	}
	return queued, nil
}

func (s *SubscriptionService) runSiteSearch(ctx context.Context, sub *model.Subscription) (int, error) {
	if s.site == nil {
		if s.log != nil {
			s.log.Warn("site-search subscription service unavailable", subscriptionSiteSearchLogFields(sub, "")...)
		}
		return 0, errors.New("site search service unavailable")
	}
	keywords := siteSearchKeywords(sub)
	keyword := ""
	if len(keywords) > 0 {
		keyword = keywords[0]
	}
	if keyword == "" {
		if s.log != nil {
			s.log.Warn("site-search subscription keyword missing", subscriptionSiteSearchLogFields(sub, "")...)
		}
		return 0, errors.New("site-search subscription keyword required")
	}
	if s.log != nil {
		s.log.Info("site-search subscription run started", subscriptionSiteSearchLogFields(sub, keyword)...)
	}

	var (
		results       []SearchResult
		lastSearchErr error
		searchErrors  int
	)
	for _, searchKeyword := range keywords {
		found, err := s.site.Search(ctx, searchKeyword)
		if err != nil {
			lastSearchErr = err
			searchErrors++
			if s.log != nil {
				fields := subscriptionSiteSearchLogFields(sub, searchKeyword)
				fields = append(fields, zap.Error(err))
				s.log.Warn("site-search subscription search failed", fields...)
			}
			continue
		}
		results = append(results, found...)
	}
	results = dedupeSiteSearchResults(results)
	if len(results) == 0 && lastSearchErr != nil && searchErrors == len(keywords) {
		return 0, lastSearchErr
	}
	if len(results) == 0 {
		if s.log != nil {
			fields := subscriptionSiteSearchLogFields(sub, keyword)
			fields = append(fields, zap.Int("results_count", 0))
			s.log.Info("site-search subscription no results", fields...)
		}
		now := time.Now()
		_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
		return 0, nil
	}
	s.updateSubscriptionTotalEpisodes(ctx, sub, s.resolveSubscriptionTotalEpisodes(ctx, sub, inferSearchTotalEpisodes(results, sub)))

	guidKey := fmt.Sprintf("subscription.%s.seen", sub.ID)
	seenRaw, _ := s.repo.Setting.Get(ctx, guidKey)
	seen := splitNonEmpty(seenRaw)
	seenSet := make(map[string]struct{}, len(seen))
	for _, g := range seen {
		seenSet[g] = struct{}{}
	}

	availability := mergeLocalAvailability(
		SubscriptionLocalAvailability(ctx, s.repo, sub),
		s.pendingDownloadAvailability(ctx, sub),
	)
	candidates, selectionStats := selectSiteSearchCandidatesWithStats(results, sub, seenSet, availability)
	if s.log != nil {
		fields := subscriptionSiteSearchLogFields(sub, keyword)
		fields = appendSiteSearchSelectionLogFields(fields, selectionStats)
		fields = appendAvailabilityLogFields(fields, availability)
		s.log.Info("site-search subscription selection summary", fields...)
	}
	var lastEnqueueErr error
	queued := 0
	var resources []string
	for _, candidate := range candidates {
		item := candidate.Item
		matchText := subscriptionSearchResultText(item)
		mediaType, mediaCategory := s.classifySubscriptionItem(ctx, sub, matchText, item.Category)
		if s.shouldSkipExistingTorrent(ctx, mediaType, candidate) {
			addSiteSearchCandidateAvailability(candidate, &availability)
			seen = append(seen, candidate.GUID)
			seenSet[candidate.GUID] = struct{}{}
			if s.log != nil {
				fields := subscriptionSiteSearchLogFields(sub, keyword)
				fields = append(fields,
					zap.String("reason", "existing_torrent"),
					zap.String("title", item.Title),
					zap.String("subtitle", item.Subtitle),
					zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
					zap.String("site_category", item.Category),
					zap.Int("season", candidate.Season),
					zap.Int("episode", candidate.Episode),
					zap.Bool("pack", candidate.Pack),
					zap.String("media_type", mediaType),
				)
				s.log.Info("site-search subscription candidate skipped", fields...)
			}
			continue
		}
		realURL := s.site.ResolveDownloadURL(ctx, candidate.Download)
		savePath := s.resolveSubscriptionSavePath(ctx, sub, mediaType, mediaCategory)
		if s.downloadPathHasCandidate(ctx, sub, matchText, savePath) {
			addSiteSearchCandidateAvailability(candidate, &availability)
			seen = append(seen, candidate.GUID)
			seenSet[candidate.GUID] = struct{}{}
			if s.log != nil {
				fields := subscriptionSiteSearchLogFields(sub, keyword)
				fields = append(fields,
					zap.String("reason", "download_path_has_candidate"),
					zap.String("title", item.Title),
					zap.String("subtitle", item.Subtitle),
					zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
					zap.String("site_category", item.Category),
					zap.Int("season", candidate.Season),
					zap.Int("episode", candidate.Episode),
					zap.Bool("pack", candidate.Pack),
					zap.String("media_type", mediaType),
					zap.String("media_category", mediaCategory),
					zap.String("save_path", savePath),
				)
				s.log.Info("site-search subscription candidate skipped", fields...)
			}
			continue
		}
		if _, err := s.downloads.AddDownloadWithMeta(ctx, sub.UserID, realURL, savePath, DownloadTaskMeta{
			SubscriptionID:       sub.ID,
			Title:                firstNonEmpty(item.Title, sub.Name),
			PosterURL:            sub.PosterURL,
			BackdropURL:          sub.BackdropURL,
			Overview:             sub.Overview,
			MediaType:            mediaType,
			MediaCategory:        mediaCategory,
			SourceCategory:       item.Category,
			AllowExistingLibrary: sub.WashEnabled,
		}); err != nil {
			if IsDownloadDedupError(err) {
				addSiteSearchCandidateAvailability(candidate, &availability)
				seen = append(seen, candidate.GUID)
				seenSet[candidate.GUID] = struct{}{}
				if s.log != nil {
					fields := subscriptionSiteSearchLogFields(sub, keyword)
					fields = append(fields,
						zap.String("reason", "download_dedup"),
						zap.String("title", item.Title),
						zap.String("subtitle", item.Subtitle),
						zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
						zap.String("site_category", item.Category),
						zap.Int("season", candidate.Season),
						zap.Int("episode", candidate.Episode),
						zap.Bool("pack", candidate.Pack),
						zap.String("media_type", mediaType),
						zap.String("media_category", mediaCategory),
						zap.String("save_path", savePath),
					)
					s.log.Info("site-search subscription candidate skipped", fields...)
				}
				continue
			}
			lastEnqueueErr = err
			s.log.Warn("site-search subscription enqueue failed",
				zap.String("subscription_id", sub.ID),
				zap.String("subscription", sub.Name),
				zap.String("keyword", keyword),
				zap.String("title", item.Title),
				zap.String("subtitle", item.Subtitle),
				zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
				zap.String("site_category", item.Category),
				zap.String("media_type", mediaType),
				zap.String("media_category", mediaCategory),
				zap.String("save_path", savePath),
				zap.Error(err))
			continue
		}
		queued++
		addSiteSearchCandidateAvailability(candidate, &availability)
		resources = append(resources, item.Title)
		seen = append(seen, candidate.GUID)
		seenSet[candidate.GUID] = struct{}{}
		if s.log != nil {
			fields := subscriptionSiteSearchLogFields(sub, keyword)
			fields = append(fields,
				zap.String("title", item.Title),
				zap.String("subtitle", item.Subtitle),
				zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
				zap.String("site_category", item.Category),
				zap.Int("season", candidate.Season),
				zap.Int("episode", candidate.Episode),
				zap.Bool("pack", candidate.Pack),
				zap.Int("score", candidate.Score),
				zap.String("media_type", mediaType),
				zap.String("media_category", mediaCategory),
				zap.String("save_path", savePath),
			)
			s.log.Info("site-search subscription candidate queued", fields...)
		}
	}
	availability = s.finalizePendingAvailability(sub, availability)
	if len(seen) > 200 {
		seen = seen[len(seen)-200:]
	}
	_ = s.repo.Setting.Set(ctx, guidKey, strings.Join(seen, "\n"))
	now := time.Now()
	_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
	_ = s.archiveCompletedSubscription(ctx, sub, availability)
	if queued > 0 {
		s.hub.Publish("subscription", map[string]any{
			"id":        sub.ID,
			"name":      sub.Name,
			"queued":    queued,
			"keyword":   keyword,
			"resources": resources,
		})
		s.notifySubscriptionHit(sub, queued, resources)
		return queued, nil
	}
	if lastEnqueueErr != nil {
		return 0, fmt.Errorf("找到 PT 资源但加入下载器失败: %w", lastEnqueueErr)
	}
	if s.log != nil {
		fields := subscriptionSiteSearchLogFields(sub, keyword)
		fields = appendSiteSearchSelectionLogFields(fields, selectionStats)
		fields = appendAvailabilityLogFields(fields, availability)
		fields = append(fields, zap.Int("queued", queued))
		s.log.Info("site-search subscription no candidate queued", fields...)
	}
	return 0, nil
}

func subscriptionSiteSearchLogFields(sub *model.Subscription, keyword string) []zap.Field {
	fields := []zap.Field{zap.String("keyword", keyword), zap.Strings("search_keywords", siteSearchKeywords(sub))}
	if sub == nil {
		return fields
	}
	fields = append(fields,
		zap.String("subscription_id", sub.ID),
		zap.String("subscription", sub.Name),
		zap.String("filter", sub.Filter),
		zap.String("media_type", sub.MediaType),
		zap.String("media_category", sub.MediaCategory),
		zap.String("search_mode", sub.SearchMode),
		zap.String("imdb_id", sub.IMDBID),
		zap.Bool("wash_enabled", sub.WashEnabled),
		zap.String("wash_priority", sub.WashPriority),
		zap.Int("total_episodes", sub.TotalEpisodes),
	)
	return fields
}

func appendSiteSearchSelectionLogFields(fields []zap.Field, stats siteSearchSelectionStats) []zap.Field {
	return append(fields,
		zap.Int("results_count", stats.Total),
		zap.Int("query_mismatch_count", stats.QueryMismatch),
		zap.Int("relaxed_query_match_count", stats.RelaxedQueryMatch),
		zap.Int("rule_mismatch_count", stats.RuleMismatch),
		zap.Int("missing_download_count", stats.MissingDownload),
		zap.Int("seen_count", stats.Seen),
		zap.Int("prepared_count", stats.Prepared),
		zap.Int("selected_count", stats.Selected),
		zap.Bool("local_already_satisfied", stats.LocalAlreadySatisfied),
		zap.Bool("local_series_pack_present", stats.LocalSeriesPackPresent),
		zap.Bool("series_complete", stats.SeriesComplete),
		zap.Int("existing_episode_skipped_count", stats.ExistingEpisodeSkipped),
		zap.Int("not_missing_episode_skipped_count", stats.NotMissingEpisodeSkipped),
		zap.Int("no_episode_skipped_count", stats.NoEpisodeSkipped),
		zap.Bool("pack_fallback_available", stats.PackFallbackAvailable),
		zap.Bool("pack_fallback_used", stats.PackFallbackUsed),
	)
}

func appendAvailabilityLogFields(fields []zap.Field, availability LocalAvailability) []zap.Field {
	missingSample, missingMore := limitedEpisodeSample(availability.MissingEpisodes, 20)
	return append(fields,
		zap.Int("local_media_count", availability.LocalMediaCount),
		zap.Bool("in_library", availability.InLibrary),
		zap.Bool("has_series_pack", availability.HasSeriesPack),
		zap.Int("downloaded_episodes", availability.DownloadedEpisodes),
		zap.Int("availability_total_episodes", availability.TotalEpisodes),
		zap.Int("missing_episode_count", len(availability.MissingEpisodes)),
		zap.Ints("missing_episodes", missingSample),
		zap.Int("missing_episodes_more", missingMore),
	)
}

func limitedEpisodeSample(values []int, limit int) ([]int, int) {
	if limit <= 0 || len(values) == 0 {
		return nil, len(values)
	}
	if len(values) <= limit {
		out := append([]int(nil), values...)
		return out, 0
	}
	out := append([]int(nil), values[:limit]...)
	return out, len(values) - limit
}

func (s *SubscriptionService) notifySubscriptionHit(sub *model.Subscription, queued int, resources []string) {
	if s == nil || s.notify == nil || sub == nil || queued <= 0 {
		return
	}
	body := fmt.Sprintf("订阅：%s\n新增资源：%d", sub.Name, queued)
	if len(resources) > 0 {
		body += "\n资源：\n- " + strings.Join(resources, "\n- ")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data := map[string]interface{}{}
		if strings.TrimSpace(sub.PosterURL) != "" {
			data["poster_url"] = sub.PosterURL
		}
		if strings.TrimSpace(sub.BackdropURL) != "" {
			data["backdrop_url"] = sub.BackdropURL
		}
		if strings.TrimSpace(sub.MediaType) != "" {
			data["media_type"] = sub.MediaType
		}
		if strings.TrimSpace(sub.MediaCategory) != "" {
			data["media_category"] = sub.MediaCategory
		}
		// 补充媒体通知模板(formatTelegramMediaNotification)所需字段:片名 / 原名 /
		// 语言 / 年份 / 评分 / 类型 / 简介 / 外链 / 资源标题(供模板提取季集 + 版本)。
		// 仅填现成可用的,缺失项模板会自动略过。
		if strings.TrimSpace(sub.Name) != "" {
			data["title"] = sub.Name
		}
		if strings.TrimSpace(sub.OriginalName) != "" {
			data["original_title"] = sub.OriginalName
		}
		if strings.TrimSpace(sub.OriginalLanguage) != "" {
			data["original_language"] = sub.OriginalLanguage
		}
		if sub.Year > 0 {
			data["year"] = sub.Year
		}
		if sub.Rating > 0 {
			data["rating"] = sub.Rating
		}
		if strings.TrimSpace(sub.Genres) != "" {
			data["genres"] = sub.Genres
		}
		if strings.TrimSpace(sub.Overview) != "" {
			data["overview"] = sub.Overview
		}
		if id := strings.TrimSpace(sub.IMDBID); id != "" {
			data["imdb_url"] = "https://www.imdb.com/title/" + id + "/"
		}
		if len(resources) > 0 {
			data["resource_title"] = resources[0]
		}
		s.notify.BroadcastEvent(ctx, NotifyEvent{
			Type:    EventSubscriptionHit,
			Title:   "MediaStationGo 订阅命中新资源",
			Message: body,
			Data:    data,
		})
	}()
}

func (s *SubscriptionService) archiveCompletedSubscription(ctx context.Context, sub *model.Subscription, availability LocalAvailability) error {
	if s == nil || s.repo == nil || s.repo.Subscription == nil || sub == nil {
		return nil
	}
	if !subscriptionShouldArchive(sub, availability) {
		return nil
	}
	now := time.Now()
	reason := subscriptionArchiveReason(sub, availability)
	if err := s.repo.Subscription.Archive(ctx, sub.ID, reason, now); err != nil {
		return err
	}
	sub.Enabled = false
	sub.ArchivedAt = &now
	sub.ArchiveReason = reason
	if s.log != nil {
		s.log.Info("subscription completed, moved to history",
			zap.String("id", sub.ID),
			zap.String("name", sub.Name),
			zap.String("reason", reason))
	}
	if s.hub != nil {
		s.hub.Publish("subscription", map[string]any{
			"id":       sub.ID,
			"name":     sub.Name,
			"archived": true,
			"reason":   reason,
		})
	}
	return nil
}

func subscriptionShouldArchive(sub *model.Subscription, availability LocalAvailability) bool {
	if sub == nil || sub.WashEnabled || sub.ArchivedAt != nil {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(sub.MediaType))
	if !isSubscriptionSeriesType(mediaType) {
		return availability.InLibrary || availability.LocalMediaCount > 0 || availability.DownloadedEpisodes > 0
	}
	if availability.HasSeriesPack {
		return true
	}
	total := sub.TotalEpisodes
	if total <= 0 {
		total = availability.TotalEpisodes
	}
	if total > 0 {
		return availability.DownloadedEpisodes >= total && len(availability.MissingEpisodes) == 0
	}
	return subscriptionLooksSingleEpisode(sub) && availability.DownloadedEpisodes > 0
}

func subscriptionArchiveReason(sub *model.Subscription, availability LocalAvailability) string {
	if sub != nil && sub.WashEnabled {
		return ""
	}
	if availability.HasSeriesPack {
		return "整季资源已加入下载/入库"
	}
	if availability.TotalEpisodes > 0 {
		return fmt.Sprintf("订阅完成：%d/%d", availability.DownloadedEpisodes, availability.TotalEpisodes)
	}
	if availability.DownloadedEpisodes > 0 {
		return "单集订阅已加入下载/入库"
	}
	return "订阅媒体已加入下载/入库"
}

func (s *SubscriptionService) updateSubscriptionTotalEpisodes(ctx context.Context, sub *model.Subscription, total int) {
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil || total <= sub.TotalEpisodes {
		return
	}
	sub.TotalEpisodes = total
	_ = s.repo.DB.WithContext(ctx).Model(sub).Update("total_episodes", total).Error
}

func inferRSSTotalEpisodes(items []rssItem, sub *model.Subscription, filter *regexp.Regexp) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	maxEpisode := 0
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		if filter != nil && !filter.MatchString(title) {
			continue
		}
		if !subscriptionTitleMatchesQuery(sub, title) {
			continue
		}
		if !matchesSubscriptionRules(sub, title) {
			continue
		}
		_, episode := ParseEpisode(title)
		if episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func inferSearchTotalEpisodes(results []SearchResult, sub *model.Subscription) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	maxEpisode := 0
	for _, item := range results {
		matchText := subscriptionSearchResultText(item)
		if !subscriptionTitleMatchesQuery(sub, matchText) {
			continue
		}
		if !matchesSubscriptionRules(sub, matchText) {
			continue
		}
		_, episode := ParseEpisode(matchText)
		if episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func subscriptionShouldInferTotal(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	return isSubscriptionSeriesType(mediaType)
}

func (s *SubscriptionService) resolveSubscriptionTotalEpisodes(ctx context.Context, sub *model.Subscription, fallback int) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	if sub.TotalEpisodes > 0 {
		return sub.TotalEpisodes
	}
	if total := s.resolveSubscriptionMetadataTotalEpisodes(ctx, sub); total > 0 {
		return total
	}
	return fallback
}

func (s *SubscriptionService) resolveSubscriptionMetadataTotalEpisodes(ctx context.Context, sub *model.Subscription) int {
	if s == nil || s.scraper == nil || sub == nil {
		return 0
	}
	queries := subscriptionEpisodeMetadataQueries(sub)

	// Priority: TMDb -> Douban -> Bangumi -> TheTVDB -> Fanart -> title fallback.
	// Fanart.tv is artwork-only in MediaStationGo, so it intentionally does not
	// claim episode counts and lets the title fallback handle the final layer.
	if s.scraper.tmdb != nil {
		if id := subscriptionExplicitTMDbID(sub); id > 0 {
			if total, err := s.scraper.tmdb.GetTVEpisodeCount(ctx, id); err == nil && total > 0 {
				return total
			} else if err != nil && s.log != nil {
				s.log.Debug("subscription tmdb episode count failed", zap.Int("tmdb_id", id), zap.Error(err))
			}
		}
		for _, query := range queries {
			match, err := s.scraper.tmdb.SearchTV(ctx, query, 0)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription tmdb search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || match.TMDbID <= 0 {
				continue
			}
			total, err := s.scraper.tmdb.GetTVEpisodeCount(ctx, match.TMDbID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription tmdb episode count failed", zap.Int("tmdb_id", match.TMDbID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	if s.scraper.douban != nil {
		for _, query := range queries {
			total, err := s.scraper.douban.GetEpisodeCount(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription douban episode count failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	if s.scraper.bangumi != nil {
		for _, query := range queries {
			match, err := s.scraper.bangumi.Search(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription bangumi search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || match.BangumiID <= 0 {
				continue
			}
			total, err := s.scraper.bangumi.GetEpisodeCount(ctx, match.BangumiID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription bangumi episode count failed", zap.Int("bangumi_id", match.BangumiID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	if s.scraper.thetvdb != nil {
		for _, query := range queries {
			match, err := s.scraper.thetvdb.SearchSeries(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription thetvdb search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || strings.TrimSpace(match.TheTVDBID) == "" {
				continue
			}
			total, err := s.scraper.thetvdb.GetSeriesEpisodeCount(ctx, match.TheTVDBID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription thetvdb episode count failed", zap.String("thetvdb_id", match.TheTVDBID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	return 0
}

func subscriptionTitleMatchesQuery(sub *model.Subscription, title string) bool {
	if strings.TrimSpace(title) == "" {
		return false
	}
	for _, query := range subscriptionTitleMatchQueries(sub) {
		if strings.Contains(normalizeAvailabilityComparable(title), normalizeAvailabilityComparable(query)) {
			return true
		}
	}
	return len(subscriptionTitleMatchQueries(sub)) == 0
}

func subscriptionTitleMatchQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := []string{
		availabilityQuery(subscriptionName(sub), subscriptionFilter(sub)),
		cleanAvailabilityTitle(subscriptionFilter(sub)),
		cleanAvailabilityTitle(subscriptionName(sub)),
	}
	for _, alias := range subscriptionFeedAliases(sub) {
		values = append(values, alias, cleanAvailabilityTitle(alias))
	}
	return compactUniqueStrings(values...)
}

func subscriptionEpisodeMetadataQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	raw := []string{
		siteSearchKeyword(sub),
		sub.Filter,
		sub.Name,
		availabilityQuery(subscriptionName(sub), subscriptionFilter(sub)),
	}
	out := make([]string, 0, len(raw)*2)
	for _, value := range raw {
		value = cleanAvailabilityTitle(value)
		if value == "" {
			continue
		}
		if cleaned, _ := CleanQuery(value); cleaned != "" {
			out = append(out, cleaned)
		}
		out = append(out, value)
	}
	return compactUniqueStrings(out...)
}

func subscriptionExplicitTMDbID(sub *model.Subscription) int {
	if sub == nil {
		return 0
	}
	values := []string{sub.Name, sub.Filter, sub.FeedURL}
	for _, raw := range values {
		for _, pattern := range []string{`(?i)\btmdb[_:\-\s=]+(\d{2,})`, `(?i)\btmdbid[_:\-\s=]+(\d{2,})`} {
			if m := regexp.MustCompile(pattern).FindStringSubmatch(raw); len(m) >= 2 {
				var id int
				if _, err := fmt.Sscanf(m[1], "%d", &id); err == nil && id > 0 {
					return id
				}
			}
		}
		if u, err := url.Parse(raw); err == nil {
			for _, key := range []string{"tmdb_id", "tmdb", "tmdbid"} {
				var id int
				if _, err := fmt.Sscanf(u.Query().Get(key), "%d", &id); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	return 0
}

func compactUniqueStrings(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeAvailabilityComparable(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func subscriptionLooksSingleEpisode(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	for _, value := range []string{sub.Name, sub.Filter} {
		_, episode := ParseEpisode(value)
		if episode > 0 {
			return true
		}
	}
	return false
}

func (s *SubscriptionService) shouldSkipExistingTorrent(ctx context.Context, mediaType string, candidate siteSearchCandidate) bool {
	if s == nil || s.downloads == nil {
		return false
	}
	if isSubscriptionSeriesType(mediaType) && !candidate.Pack && candidate.Episode > 0 {
		return false
	}
	return s.downloads.TorrentExistsByName(ctx, candidate.Item.Title)
}

func siteSearchKeywords(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := make([]string, 0, 8)
	if strings.EqualFold(strings.TrimSpace(sub.SearchMode), "imdb") && strings.TrimSpace(sub.IMDBID) != "" {
		values = append(values, strings.TrimSpace(sub.IMDBID))
	}
	if u, err := url.Parse(sub.FeedURL); err == nil {
		if keyword := strings.TrimSpace(u.Query().Get("keyword")); keyword != "" {
			values = append(values, keyword)
		}
	}
	if strings.TrimSpace(sub.Filter) != "" {
		values = append(values, sub.Filter)
	}
	if len(values) == 0 && strings.TrimSpace(sub.Name) != "" {
		values = append(values, sub.Name)
	}
	values = append(values, subscriptionFeedAliases(sub)...)
	for _, value := range append([]string(nil), values...) {
		if cleaned := cleanAvailabilityTitle(value); cleaned != "" {
			values = append(values, cleaned)
		}
	}
	return compactUniqueStrings(values...)
}

func siteSearchKeyword(sub *model.Subscription) string {
	keywords := siteSearchKeywords(sub)
	if len(keywords) == 0 {
		return ""
	}
	return keywords[0]
}

func subscriptionFeedAliases(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	u, err := url.Parse(sub.FeedURL)
	if err != nil {
		return nil
	}
	q := u.Query()
	values := make([]string, 0, len(q["alias"])+2)
	values = append(values, q["alias"]...)
	for _, raw := range q["aliases"] {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == '|' || r == '\n' || r == '\r' || r == '\t'
		}) {
			values = append(values, part)
		}
	}
	return compactUniqueStrings(values...)
}

func dedupeSiteSearchResults(results []SearchResult) []SearchResult {
	if len(results) < 2 {
		return results
	}
	seen := make(map[string]struct{}, len(results))
	out := make([]SearchResult, 0, len(results))
	for _, item := range results {
		download := strings.TrimSpace(item.DownloadURL)
		if download == "" {
			download = strings.TrimSpace(item.TorrentURL)
		}
		key := stableSiteSearchGUID(item, download)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (s *SubscriptionService) fetch(ctx context.Context, feedURL string) (*rssFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rss %s: %d", feedURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var f rssFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func compileFilter(pat string) *regexp.Regexp {
	pat = strings.TrimSpace(pat)
	if pat == "" {
		return nil
	}
	if r, err := regexp.Compile("(?i)" + pat); err == nil {
		return r
	}
	return nil
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0)
	for _, p := range strings.Split(s, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
