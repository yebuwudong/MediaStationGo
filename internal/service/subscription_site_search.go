package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
