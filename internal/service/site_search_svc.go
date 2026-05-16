// Package service — 跨站聚合搜索服务.
package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SiteSearchService 跨站聚合搜索服务.
type SiteSearchService struct {
	log  *zap.Logger
	repo *repository.Container
	site *SiteService
}

// NewSiteSearchService 创建跨站搜索服务.
func NewSiteSearchService(log *zap.Logger, repo *repository.Container, siteSvc *SiteService) *SiteSearchService {
	return &SiteSearchService{log: log, repo: repo, site: siteSvc}
}

// SearchAll 在所有启用的站点中搜索关键字.
func (s *SiteSearchService) SearchAll(ctx context.Context, keyword string, page, pageSize int) (*AggregatedResult, error) {
	sites, err := s.repo.Site.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled sites: %w", err)
	}

	if len(sites) == 0 {
		return &AggregatedResult{
			Keyword:  keyword,
			Items:    []TorrentItem{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	return s.SearchSites(ctx, keyword, sites, page, pageSize)
}

// SearchSites 在指定站点中搜索关键字.
func (s *SiteSearchService) SearchSites(ctx context.Context, keyword string, sites []model.Site, page, pageSize int) (*AggregatedResult, error) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var allItems []TorrentItem

	for _, site := range sites {
		wg.Add(1)
		go func(siteModel model.Site) {
			defer wg.Done()

			cfg, err := s.site.GetSiteConfig(ctx, siteModel.ID)
			if err != nil {
				s.log.Warn("get site config failed", zap.String("site_id", siteModel.ID), zap.Error(err))
				return
			}

			adapter := GetAdapterForType(siteModel.Type)
			result, err := adapter.Search(ctx, *cfg, keyword, page)
			if err != nil {
				s.log.Warn("site search failed",
					zap.String("site_id", siteModel.ID),
					zap.String("site_name", siteModel.Name),
					zap.Error(err),
				)
				return
			}

			mu.Lock()
			allItems = append(allItems, result.Items...)
			mu.Unlock()
		}(site)
	}

	wg.Wait()

	sort.Slice(allItems, func(i, j int) bool {
		if allItems[i].Seeders != allItems[j].Seeders {
			return allItems[i].Seeders > allItems[j].Seeders
		}
		return allItems[i].UploadTime.After(allItems[j].UploadTime)
	})

	allItems = deduplicateItems(allItems)

	total := len(allItems)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	return &AggregatedResult{
		Keyword:  keyword,
		Items:    allItems[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// SearchSite 在单个站点中搜索.
func (s *SiteSearchService) SearchSite(ctx context.Context, siteID, keyword string, page int) (*SearchResult, error) {
	cfg, err := s.site.GetSiteConfig(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("get site config: %w", err)
	}

	siteModel, err := s.repo.Site.FindByID(ctx, siteID)
	if err != nil || siteModel == nil {
		return nil, fmt.Errorf("find site: %w", err)
	}

	adapter := GetAdapterForType(siteModel.Type)
	result, err := adapter.Search(ctx, *cfg, keyword, page)
	if err != nil {
		return nil, fmt.Errorf("search site %s: %w", siteModel.Name, err)
	}

	return result, nil
}

// BrowseSite 浏览站点资源.
func (s *SiteSearchService) BrowseSite(ctx context.Context, siteID, category string, page int) (*SearchResult, error) {
	cfg, err := s.site.GetSiteConfig(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("get site config: %w", err)
	}

	siteModel, err := s.repo.Site.FindByID(ctx, siteID)
	if err != nil || siteModel == nil {
		return nil, fmt.Errorf("find site: %w", err)
	}

	adapter := GetAdapterForType(siteModel.Type)
	result, err := adapter.Browse(ctx, *cfg, category, page)
	if err != nil {
		return nil, fmt.Errorf("browse site %s: %w", siteModel.Name, err)
	}

	return result, nil
}

// GetTorrentDetail 获取种子详情.
func (s *SiteSearchService) GetTorrentDetail(ctx context.Context, siteID, torrentID string) (*TorrentDetail, error) {
	cfg, err := s.site.GetSiteConfig(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("get site config: %w", err)
	}

	siteModel, err := s.repo.Site.FindByID(ctx, siteID)
	if err != nil || siteModel == nil {
		return nil, fmt.Errorf("find site: %w", err)
	}

	adapter := GetAdapterForType(siteModel.Type)
	detail, err := adapter.GetDetail(ctx, *cfg, torrentID)
	if err != nil {
		return nil, fmt.Errorf("get detail from %s: %w", siteModel.Name, err)
	}

	return detail, nil
}

// AggregatedResult 聚合搜索结果.
type AggregatedResult struct {
	Keyword  string        `json:"keyword"`
	Items    []TorrentItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// deduplicateItems 通过标题相似性去重.
func deduplicateItems(items []TorrentItem) []TorrentItem {
	seen := make(map[string]bool)
	result := make([]TorrentItem, 0, len(items))

	for _, item := range items {
		key := normalizeTitle(item.Title)
		if key == "" {
			continue
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}

	return result
}

// normalizeTitle 标题标准化.
func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	title = strings.ReplaceAll(title, ".", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}
	return strings.TrimSpace(title)
}
