package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type SiteCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	SiteID      string `json:"site_id,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	SiteType    string `json:"site_type,omitempty"`
	Adult       bool   `json:"adult"`
	Description string `json:"description,omitempty"`
}

type SiteBrowseParams struct {
	SiteID       string
	Keyword      string
	Category     string
	Page         int
	IncludeAdult bool
}

type SiteBrowseResult struct {
	Items      []SearchResult `json:"items"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
	Category   string         `json:"category,omitempty"`
	Keyword    string         `json:"keyword,omitempty"`
}

type siteCategorySearcher interface {
	SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error)
}

type siteCategoryProvider interface {
	Categories(ctx context.Context, cfg SiteConfig) ([]SiteCategory, error)
}

const siteBrowsePageSize = 50

func (s *SiteService) Categories(ctx context.Context, siteID string) ([]SiteCategory, error) {
	sites, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	byKey := map[string]SiteCategory{}
	for _, site := range sites {
		if siteID != "" && site.ID != siteID {
			continue
		}
		if !site.Enabled {
			continue
		}
		cats := s.originalCategoriesForSite(ctx, site)
		for _, cat := range cats {
			key := strings.ToLower(strings.Join([]string{cat.SiteType, cat.ID, cat.Name}, "\x00"))
			if _, ok := byKey[key]; !ok {
				byKey[key] = cat
			}
		}
	}
	out := make([]SiteCategory, 0, len(byKey))
	for _, cat := range byKey {
		out = append(out, cat)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Adult != out[j].Adult {
			return !out[i].Adult
		}
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *SiteService) originalCategoriesForSite(ctx context.Context, site model.Site) []SiteCategory {
	adapter := NewSiteAdapter(&site)
	if provider, ok := adapter.(siteCategoryProvider); ok {
		cfg := s.siteModelToConfig(&site)
		timeout := time.Duration(site.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cats, err := provider.Categories(reqCtx, cfg)
		if err == nil && len(cats) > 0 {
			return normalizeSiteCategories(site, cats)
		}
		if err != nil {
			s.log.Warn("site categories failed", zap.String("site", site.Name), zap.String("type", site.Type), zap.Error(err))
		}
	}
	return normalizeSiteCategories(site, siteCategoriesForSite(site))
}

func (s *SiteService) Browse(ctx context.Context, p SiteBrowseParams) (*SiteBrowseResult, error) {
	if p.Page <= 0 {
		p.Page = 1
	}
	p.Keyword = strings.TrimSpace(p.Keyword)
	p.Category = strings.TrimSpace(p.Category)
	sites, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var (
		mu         sync.Mutex
		wg         sync.WaitGroup
		results    []SearchResult
		total      int
		matched    int
		failed     int
		lastErrMsg string
	)
	for i := range sites {
		site := sites[i]
		if !site.Enabled {
			continue
		}
		if p.SiteID != "" && site.ID != p.SiteID {
			continue
		}
		matched++
		wg.Add(1)
		go func(site model.Site) {
			defer wg.Done()
			adapter := NewSiteAdapter(&site)
			if adapter == nil {
				return
			}
			cfg := s.siteModelToConfig(&site)
			timeout := time.Duration(site.Timeout) * time.Second
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			reqCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			result, err := browseSiteResources(reqCtx, adapter, cfg, p.Keyword, p.Category, p.Page)
			if err != nil {
				mu.Lock()
				failed++
				lastErrMsg = err.Error()
				mu.Unlock()
				s.log.Warn("site browse failed",
					zap.String("site", site.Name),
					zap.String("type", site.Type),
					zap.String("category", p.Category),
					zap.String("keyword", p.Keyword),
					zap.Error(err))
				return
			}
			if result == nil {
				return
			}
			catAdult := siteCategoryIsAdult(site, p.Category)
			items := result.Items
			if items == nil {
				items = []TorrentItem{}
			}
			local := make([]SearchResult, 0, len(items))
			for _, item := range items {
				row := siteSearchResultFromItem(site, item, catAdult)
				if !siteCategoryMatches(site, row.Category, p.Category) {
					continue
				}
				if !p.IncludeAdult && row.Adult {
					continue
				}
				local = append(local, row)
			}
			mu.Lock()
			results = append(results, local...)
			total += result.Total
			mu.Unlock()
		}(site)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Seeders != results[j].Seeders {
			return results[i].Seeders > results[j].Seeders
		}
		if results[i].UploadTime != results[j].UploadTime {
			return results[i].UploadTime > results[j].UploadTime
		}
		return results[i].Size > results[j].Size
	})
	if results == nil {
		results = []SearchResult{}
	}
	if len(results) == 0 && failed > 0 && failed >= matched {
		return nil, fmt.Errorf("site browse failed: %s", lastErrMsg)
	}
	if total == 0 {
		total = len(results)
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + siteBrowsePageSize - 1) / siteBrowsePageSize
	}
	return &SiteBrowseResult{
		Items:      results,
		Total:      total,
		Page:       p.Page,
		PageSize:   siteBrowsePageSize,
		TotalPages: totalPages,
		Category:   p.Category,
		Keyword:    p.Keyword,
	}, nil
}

func browseSiteResources(ctx context.Context, adapter SiteAdapter, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	if strings.TrimSpace(keyword) != "" {
		if strings.TrimSpace(category) != "" {
			if searcher, ok := adapter.(siteCategorySearcher); ok {
				return searcher.SearchWithCategory(ctx, cfg, keyword, category, page)
			}
		}
		return adapter.Search(ctx, cfg, keyword, page)
	}
	return adapter.Browse(ctx, cfg, category, page)
}

func (s *SiteService) Detail(ctx context.Context, siteID, torrentID string) (*TorrentDetail, error) {
	siteID = strings.TrimSpace(siteID)
	torrentID = strings.TrimSpace(torrentID)
	if siteID == "" || torrentID == "" {
		return nil, errors.New("site_id and id required")
	}
	site, err := s.FindByID(ctx, siteID)
	if err != nil || site == nil {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("site not found")
	}
	adapter := NewSiteAdapter(site)
	if adapter == nil {
		return nil, errors.New("site adapter unavailable")
	}
	cfg := s.siteModelToConfig(site)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return adapter.GetDetail(reqCtx, cfg, torrentID)
}

func (s *SiteService) DownloadURL(ctx context.Context, siteID, torrentID, fallback string) (string, error) {
	fallback = strings.TrimSpace(fallback)
	siteID = strings.TrimSpace(siteID)
	torrentID = strings.TrimSpace(torrentID)
	if siteID == "" || torrentID == "" {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", errors.New("site_id and id required")
	}
	site, err := s.FindByID(ctx, siteID)
	if err != nil || site == nil {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		if err != nil {
			return "", err
		}
		return "", errors.New("site not found")
	}
	adapter := NewSiteAdapter(site)
	if adapter == nil {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", errors.New("site adapter unavailable")
	}
	cfg := s.siteModelToConfig(site)
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resolved, err := adapter.GetDownloadURL(reqCtx, cfg, torrentID)
	if err != nil || strings.TrimSpace(resolved) == "" {
		if fallback != "" {
			return s.ResolveDownloadURL(ctx, fallback), nil
		}
		return "", err
	}
	return resolved, nil
}

func siteSearchResultFromItem(site model.Site, item TorrentItem, forcedAdult bool) SearchResult {
	upload := ""
	if !item.UploadTime.IsZero() {
		upload = item.UploadTime.UTC().Format(time.RFC3339)
	}
	adult := forcedAdult || siteCategoryIsAdult(site, item.Category) || looksAdultPTResource(item.Title+" "+item.Subtitle+" "+item.Category)
	return SearchResult{
		SiteName:    site.Name,
		SiteID:      site.ID,
		ID:          item.ID,
		Title:       item.Title,
		Subtitle:    item.Subtitle,
		PosterURL:   item.PosterURL,
		BackdropURL: item.BackdropURL,
		TorrentURL:  item.DetailURL,
		DownloadURL: item.DownloadURL,
		Category:    item.Category,
		Size:        item.Size,
		Seeders:     item.Seeders,
		Leechers:    item.Leechers,
		Snatched:    item.Snatched,
		Free:        item.Free,
		Adult:       adult,
		UploadTime:  upload,
	}
}

func siteCategoriesForSite(site model.Site) []SiteCategory {
	out := append([]SiteCategory(nil), defaultSiteCategories(site.Type)...)
	out = append(out, extraSiteCategories(site)...)
	if len(out) == 0 {
		out = append(out, SiteCategory{ID: "", Name: "全部", Group: "全部", SiteType: site.Type})
	}
	for i := range out {
		out[i].SiteType = site.Type
	}
	return out
}

func normalizeSiteCategories(site model.Site, cats []SiteCategory) []SiteCategory {
	out := make([]SiteCategory, 0, len(cats)+1)
	hasAll := false
	seen := map[string]struct{}{}
	for _, cat := range cats {
		cat.ID = strings.TrimSpace(cat.ID)
		cat.Name = strings.TrimSpace(cat.Name)
		if cat.Name == "" {
			cat.Name = cat.ID
		}
		if cat.Name == cat.ID && cat.ID != "" {
			cat.Name = "原站分类 " + cat.ID
		}
		if cat.ID == "" && (cat.Name == "" || cat.Name == "全部") {
			hasAll = true
			cat.Name = "全部"
		}
		if cat.ID == "" && cat.Name == "" {
			continue
		}
		if cat.Group == "" {
			cat.Group = inferSiteCategoryGroup(cat.Name, cat.ID)
		}
		cat.SiteID = site.ID
		cat.SiteName = site.Name
		cat.SiteType = site.Type
		cat.Adult = cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID+" "+cat.Group)
		key := strings.ToLower(strings.Join([]string{cat.SiteID, cat.ID, cat.Name}, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cat)
	}
	if !hasAll {
		out = append([]SiteCategory{{
			ID:       "",
			Name:     "全部",
			Group:    "全部",
			SiteID:   site.ID,
			SiteName: site.Name,
			SiteType: site.Type,
		}}, out...)
	}
	return out
}

func inferSiteCategoryGroup(name, id string) string {
	text := strings.ToLower(strings.TrimSpace(name + " " + id))
	switch {
	case looksAdultPTResource(text):
		return "成人"
	case strings.Contains(text, "movie") || strings.Contains(text, "电影") || strings.Contains(text, "剧") || strings.Contains(text, "tv") || strings.Contains(text, "anime") || strings.Contains(text, "动漫") || strings.Contains(text, "综艺") || strings.Contains(text, "纪录"):
		return "影视"
	case strings.Contains(text, "music") || strings.Contains(text, "音乐") || strings.Contains(text, "mv"):
		return "音乐"
	case strings.Contains(text, "game") || strings.Contains(text, "游戏") || strings.Contains(text, "software") || strings.Contains(text, "软件") || strings.Contains(text, "book") || strings.Contains(text, "书"):
		return "其他"
	default:
		return "原站"
	}
}

func defaultSiteCategories(siteType string) []SiteCategory {
	base := []SiteCategory{
		{ID: "", Name: "全部", Group: "全部"},
		{ID: "401", Name: "电影", Group: "影视"},
		{ID: "402", Name: "剧集", Group: "影视"},
		{ID: "403", Name: "综艺", Group: "影视"},
		{ID: "404", Name: "动漫", Group: "影视"},
		{ID: "405", Name: "纪录片", Group: "影视"},
		{ID: "406", Name: "音乐", Group: "音乐"},
		{ID: "407", Name: "MV", Group: "音乐"},
		{ID: "408", Name: "体育", Group: "其他"},
		{ID: "409", Name: "软件", Group: "其他"},
		{ID: "410", Name: "游戏", Group: "其他"},
		{ID: "411", Name: "电子书", Group: "其他"},
		{ID: "412", Name: "教育", Group: "其他"},
		{ID: "413", Name: "其他", Group: "其他"},
		{ID: "420", Name: "成人视频", Group: "成人", Adult: true},
		{ID: "421", Name: "成人写真", Group: "成人", Adult: true},
		{ID: "422", Name: "成人动漫", Group: "成人", Adult: true},
		{ID: "adult", Name: "成人", Group: "成人", Adult: true},
	}
	switch strings.ToLower(strings.TrimSpace(siteType)) {
	case "unit3d":
		base = append(base,
			SiteCategory{ID: "movie", Name: "电影", Group: "影视"},
			SiteCategory{ID: "tv", Name: "剧集", Group: "影视"},
			SiteCategory{ID: "anime", Name: "动漫", Group: "影视"},
			SiteCategory{ID: "xxx", Name: "成人", Group: "成人", Adult: true},
		)
	case "gazelle":
		base = append(base,
			SiteCategory{ID: "music", Name: "音乐", Group: "音乐"},
			SiteCategory{ID: "applications", Name: "软件", Group: "其他"},
			SiteCategory{ID: "ebooks", Name: "电子书", Group: "其他"},
		)
	case "custom_rss":
		return []SiteCategory{{ID: "", Name: "全部", Group: "全部", SiteType: siteType}}
	}
	for i := range base {
		base[i].SiteType = siteType
	}
	return base
}

func extraSiteCategories(site model.Site) []SiteCategory {
	raw := strings.TrimSpace(site.Extra)
	if raw == "" {
		return nil
	}
	var payload struct {
		Categories []SiteCategory `json:"categories"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || len(payload.Categories) == 0 {
		return nil
	}
	out := make([]SiteCategory, 0, len(payload.Categories))
	for _, cat := range payload.Categories {
		cat.ID = strings.TrimSpace(cat.ID)
		cat.Name = strings.TrimSpace(cat.Name)
		if cat.Name == "" {
			cat.Name = cat.ID
		}
		if cat.Name == "" {
			continue
		}
		if cat.Group == "" {
			if cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID) {
				cat.Group = "成人"
				cat.Adult = true
			} else {
				cat.Group = "自定义"
			}
		}
		cat.SiteType = site.Type
		out = append(out, cat)
	}
	return out
}

func siteCategoryIsAdult(site model.Site, category string) bool {
	category = strings.TrimSpace(category)
	if category == "" {
		return false
	}
	for _, cat := range siteCategoriesForSite(site) {
		if strings.EqualFold(cat.ID, category) || strings.EqualFold(cat.Name, category) {
			return cat.Adult || looksAdultPTResource(cat.Name+" "+cat.ID)
		}
	}
	return looksAdultPTResource(category)
}

func siteCategoryMatches(site model.Site, itemCategory, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return true
	}
	itemCategory = strings.TrimSpace(itemCategory)
	if itemCategory == "" {
		return true
	}
	if strings.EqualFold(itemCategory, selected) {
		return true
	}
	for _, cat := range siteCategoriesForSite(site) {
		if strings.EqualFold(cat.ID, selected) || strings.EqualFold(cat.Name, selected) {
			return strings.EqualFold(itemCategory, cat.ID) || strings.EqualFold(itemCategory, cat.Name)
		}
	}
	return true
}

func looksAdultPTResource(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, needle := range []string{
		"adult", "xxx", "porn", "jav", "av", "18+", "18禁", "成人", "写真", "无码", "有码", "女优", "素人", "裏番",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func SiteSearchURL(keyword, siteID, category string, includeAdult bool) string {
	values := url.Values{}
	if strings.TrimSpace(keyword) != "" {
		values.Set("keyword", strings.TrimSpace(keyword))
	}
	if strings.TrimSpace(siteID) != "" {
		values.Set("site_id", strings.TrimSpace(siteID))
	}
	if strings.TrimSpace(category) != "" {
		values.Set("category", strings.TrimSpace(category))
	}
	if includeAdult {
		values.Set("include_adult", "true")
	}
	return "site-search://resources?" + values.Encode()
}

func siteSearchParamsFromURL(feedURL string) SiteBrowseParams {
	p := SiteBrowseParams{Page: 1}
	u, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil {
		return p
	}
	q := u.Query()
	p.Keyword = strings.TrimSpace(q.Get("keyword"))
	p.SiteID = strings.TrimSpace(q.Get("site_id"))
	p.Category = strings.TrimSpace(q.Get("category"))
	p.IncludeAdult, _ = strconv.ParseBool(q.Get("include_adult"))
	return p
}
