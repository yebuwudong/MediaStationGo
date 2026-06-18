// Package service — TMDb discovery (trending / popular).
//
// DiscoverService surfaces curated lists from TMDb so the React home
// page can show "Trending" and "Popular" rails alongside the user's own
// library. All methods gracefully no-op when the TMDb provider is
// disabled.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DiscoverService talks to TMDb's /trending and /movie/popular endpoints.
type DiscoverService struct {
	log    *zap.Logger
	tmdb   *TMDbProvider
	client *http.Client
}

// NewDiscoverService is the constructor.
func NewDiscoverService(log *zap.Logger, tmdb *TMDbProvider) *DiscoverService {
	return &DiscoverService{
		log:    log,
		tmdb:   tmdb,
		client: NewExternalHTTPClient(15 * time.Second),
	}
}

// Trending returns the daily trending movies (TMDb /trending/movie/day).
func (d *DiscoverService) Trending(ctx context.Context) ([]Match, error) {
	return d.fetch(ctx, "/trending/movie/day")
}

// Popular returns the popular movies list (TMDb /movie/popular).
func (d *DiscoverService) Popular(ctx context.Context) ([]Match, error) {
	return d.fetch(ctx, "/movie/popular")
}

// TMDbSection returns one TMDb rail converted to the common external
// discovery shape used by the multi-source Discover page.
func (d *DiscoverService) TMDbSection(ctx context.Context, key string) ([]ExternalMediaResult, error) {
	return d.TMDbSectionPage(ctx, key, 1, 40)
}

func (d *DiscoverService) TMDbSectionPage(ctx context.Context, key string, page, limit int) ([]ExternalMediaResult, error) {
	path := tmdbDiscoverPath(key)
	if path == "" {
		return []ExternalMediaResult{}, nil
	}
	matches, err := d.FetchPage(ctx, path, page)
	if err != nil {
		return nil, err
	}
	mediaType := "movie"
	if strings.Contains(path, "/tv/") {
		mediaType = "tv"
	}
	out := make([]ExternalMediaResult, 0, len(matches))
	for _, item := range limitMatches(matches, limit) {
		out = append(out, ExternalMediaResult{
			Source:           "tmdb",
			MediaType:        mediaType,
			Title:            item.Title,
			OriginalTitle:    item.OriginalName,
			OriginalLanguage: strings.Join(item.Languages, ","),
			Overview:         item.Overview,
			PosterURL:        item.PosterURL,
			BackdropURL:      item.BackdropURL,
			Year:             item.Year,
			Rating:           item.Rating,
			Genres:           strings.Join(item.Genres, ","),
			TMDbID:           item.TMDbID,
			SubscribeKeyword: buildSubscribeKeyword(item.Title, item.Year),
		})
	}
	return out, nil
}

// fetch is the shared helper that paginates page=1 only — that's all the
// home page needs and it keeps us under TMDb's 50 rps limit.
func (d *DiscoverService) fetch(ctx context.Context, path string) ([]Match, error) {
	return d.Fetch(ctx, path)
}

// Fetch is the public entry point used by the multi-section handler.
// It paginates page=1 only — that's all the home page needs and it
// keeps us under TMDb's 50 rps limit.
func (d *DiscoverService) Fetch(ctx context.Context, path string) ([]Match, error) {
	return d.FetchPage(ctx, path, 1)
}

func (d *DiscoverService) FetchPage(ctx context.Context, path string, pageNum int) ([]Match, error) {
	if d.tmdb == nil {
		return nil, nil
	}
	if pageNum <= 0 {
		pageNum = 1
	}
	if pageNum > 500 {
		pageNum = 500
	}

	// Resolve API key from config or database
	apiKey := d.tmdb.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := d.tmdb.resolveBaseURL(ctx)

	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	q.Set("page", strconv.Itoa(pageNum))
	u := base + path + "?" + q.Encode()

	type result struct {
		ID           int     `json:"id"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		VoteAverage  float32 `json:"vote_average"`
	}
	type page struct {
		Results []result `json:"results"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb %s: %d", path, resp.StatusCode)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(p.Results))
	for _, r := range p.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		m := Match{
			TMDbID:   r.ID,
			Title:    title,
			Overview: r.Overview,
			Rating:   r.VoteAverage,
		}
		if r.PosterPath != "" {
			m.PosterURL = d.tmdb.imgCDN + "/w500" + r.PosterPath
		}
		if r.BackdropPath != "" {
			m.BackdropURL = d.tmdb.imgCDN + "/w1280" + r.BackdropPath
		}
		date := r.ReleaseDate
		if date == "" {
			date = r.FirstAirDate
		}
		if len(date) >= 4 {
			_, _ = fmt.Sscanf(date[:4], "%d", &m.Year)
		}
		out = append(out, m)
	}
	return out, nil
}

func (d *DiscoverService) SearchTMDb(ctx context.Context, query, mediaType string, pageNum, limit int) ([]ExternalMediaResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || d == nil || d.tmdb == nil {
		return []ExternalMediaResult{}, nil
	}
	if pageNum <= 0 {
		pageNum = 1
	}
	paths := []struct {
		path string
		typ  string
	}{}
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		paths = append(paths, struct {
			path string
			typ  string
		}{"/search/movie", "movie"})
	case "tv", "anime", "variety":
		paths = append(paths, struct {
			path string
			typ  string
		}{"/search/tv", "tv"})
	default:
		paths = append(paths,
			struct {
				path string
				typ  string
			}{"/search/movie", "movie"},
			struct {
				path string
				typ  string
			}{"/search/tv", "tv"},
		)
	}

	apiKey := d.tmdb.resolveAPIKey(ctx)
	if apiKey == "" {
		return []ExternalMediaResult{}, nil
	}
	base := d.tmdb.resolveBaseURL(ctx)
	out := make([]ExternalMediaResult, 0, clampDiscoverLimit(limit))
	for _, entry := range paths {
		q := url.Values{}
		q.Set("api_key", apiKey)
		q.Set("query", query)
		q.Set("language", "zh-CN")
		q.Set("include_adult", "false")
		q.Set("page", strconv.Itoa(pageNum))
		matches, err := d.FetchSearch(ctx, base+entry.path+"?"+q.Encode())
		if err != nil {
			return out, err
		}
		for _, item := range matches {
			out = append(out, ExternalMediaResult{
				Source:           "tmdb",
				MediaType:        entry.typ,
				Title:            item.Title,
				OriginalTitle:    item.OriginalName,
				OriginalLanguage: strings.Join(item.Languages, ","),
				Overview:         item.Overview,
				PosterURL:        item.PosterURL,
				BackdropURL:      item.BackdropURL,
				Year:             item.Year,
				Rating:           item.Rating,
				Genres:           strings.Join(item.Genres, ","),
				TMDbID:           item.TMDbID,
				SubscribeKeyword: buildSubscribeKeyword(item.Title, item.Year),
			})
		}
	}
	return limitExternalMedia(dedupeExternalMedia(out), limit), nil
}

func (d *DiscoverService) FetchSearch(ctx context.Context, u string) ([]Match, error) {
	type result struct {
		ID           int     `json:"id"`
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		Overview     string  `json:"overview"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		VoteAverage  float32 `json:"vote_average"`
	}
	type page struct {
		Results []result `json:"results"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb search: %d", resp.StatusCode)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(p.Results))
	for _, r := range p.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		if strings.TrimSpace(title) == "" {
			continue
		}
		m := Match{TMDbID: r.ID, Title: title, Overview: r.Overview, Rating: r.VoteAverage}
		if r.PosterPath != "" {
			m.PosterURL = d.tmdb.imgCDN + "/w500" + r.PosterPath
		}
		if r.BackdropPath != "" {
			m.BackdropURL = d.tmdb.imgCDN + "/w1280" + r.BackdropPath
		}
		date := r.ReleaseDate
		if date == "" {
			date = r.FirstAirDate
		}
		if len(date) >= 4 {
			_, _ = fmt.Sscanf(date[:4], "%d", &m.Year)
		}
		out = append(out, m)
	}
	return out, nil
}

func tmdbDiscoverPath(key string) string {
	switch key {
	case "tmdb_trending_day", "trending_day":
		return "/trending/movie/day"
	case "tmdb_trending_week", "trending_week":
		return "/trending/movie/week"
	case "tmdb_popular_movie", "popular_movie":
		return "/movie/popular"
	case "tmdb_popular_tv", "popular_tv":
		return "/tv/popular"
	case "tmdb_top_rated_movie", "top_rated_movie":
		return "/movie/top_rated"
	case "tmdb_upcoming_movie", "upcoming_movie":
		return "/movie/upcoming"
	default:
		return ""
	}
}

// Discover returns public Douban movie/TV rails. Douban does not require a
// formal API key here; these are the same public web endpoints the site uses.
func (d *DoubanProvider) Discover(ctx context.Context, key string) ([]ExternalMediaResult, error) {
	return d.DiscoverPage(ctx, key, 1, 24)
}

func (d *DoubanProvider) DiscoverPage(ctx context.Context, key string, pageNum, limit int) ([]ExternalMediaResult, error) {
	doubanType := "movie"
	tag := "热门"
	switch key {
	case "douban_hot_movie":
		doubanType = "movie"
		tag = "热门"
	case "douban_top_movie":
		doubanType = "movie"
		tag = "高分"
	case "douban_hot_tv":
		doubanType = "tv"
		tag = "热门"
	default:
		return []ExternalMediaResult{}, nil
	}
	q := url.Values{}
	q.Set("type", doubanType)
	q.Set("tag", tag)
	q.Set("sort", "recommend")
	q.Set("page_limit", strconv.Itoa(clampDiscoverLimit(limit)))
	q.Set("page_start", strconv.Itoa((maxInt(pageNum, 1)-1)*clampDiscoverLimit(limit)))
	u := "https://movie.douban.com/j/search_subjects?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("douban discover: %d", resp.StatusCode)
	}
	var page struct {
		Subjects []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Rate  string `json:"rate"`
			Cover string `json:"cover"`
			URL   string `json:"url"`
		} `json:"subjects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	out := make([]ExternalMediaResult, 0, len(page.Subjects))
	mediaType := "movie"
	if doubanType == "tv" {
		mediaType = "tv"
	}
	for _, subject := range page.Subjects {
		if strings.TrimSpace(subject.Title) == "" {
			continue
		}
		rating, _ := strconv.ParseFloat(subject.Rate, 32)
		out = append(out, ExternalMediaResult{
			Source:           "douban",
			MediaType:        mediaType,
			Title:            subject.Title,
			PosterURL:        subject.Cover,
			Rating:           float32(rating),
			DoubanID:         subject.ID,
			SubscribeKeyword: subject.Title,
		})
	}
	return out, nil
}

// Calendar returns Bangumi's public on-air anime calendar as a recommendation
// rail. It needs no token, but NewBangumiProvider still attaches one when set.
func (b *BangumiProvider) Calendar(ctx context.Context) ([]ExternalMediaResult, error) {
	return b.CalendarPage(ctx, 1, 40)
}

func (b *BangumiProvider) CalendarPage(ctx context.Context, pageNum, limit int) ([]ExternalMediaResult, error) {
	type subject struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		NameCN  string `json:"name_cn"`
		Summary string `json:"summary"`
		AirDate string `json:"air_date"`
		Images  struct {
			Large  string `json:"large"`
			Common string `json:"common"`
		} `json:"images"`
		Rating struct {
			Score float32 `json:"score"`
		} `json:"rating"`
	}
	type day struct {
		Items []subject `json:"items"`
	}
	var days []day
	if err := b.getJSON(ctx, b.base+"/calendar", &days); err != nil {
		return nil, err
	}
	all := make([]ExternalMediaResult, 0, 80)
	seen := map[int]struct{}{}
	for _, day := range days {
		for _, item := range day.Items {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			title := strings.TrimSpace(item.NameCN)
			if title == "" {
				title = strings.TrimSpace(item.Name)
			}
			if title == "" {
				continue
			}
			poster := item.Images.Large
			if poster == "" {
				poster = item.Images.Common
			}
			year := 0
			if len(item.AirDate) >= 4 {
				year, _ = strconv.Atoi(item.AirDate[:4])
			}
			all = append(all, ExternalMediaResult{
				Source:           "bangumi",
				MediaType:        "anime",
				Title:            title,
				Overview:         item.Summary,
				PosterURL:        poster,
				Year:             year,
				Rating:           item.Rating.Score,
				BangumiID:        item.ID,
				SubscribeKeyword: buildSubscribeKeyword(title, year),
			})
		}
	}
	start := (maxInt(pageNum, 1) - 1) * clampDiscoverLimit(limit)
	return pageExternalMedia(all, start, clampDiscoverLimit(limit)), nil
}

func SearchSubscriptionCatalog(ctx context.Context, query, mediaType, source string, pageNum, limit int, discover *DiscoverService, douban *DoubanProvider, bangumi *BangumiProvider) ([]ExternalMediaResult, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "all"
	}
	limit = clampDiscoverLimit(limit)
	out := make([]ExternalMediaResult, 0, limit)
	if (source == "all" || source == "bangumi") && bangumi != nil && (mediaType == "" || mediaType == "anime") {
		if m, err := bangumi.Search(ctx, query); err == nil && m != nil {
			out = append(out, ExternalMediaResult{
				Source:           "bangumi",
				MediaType:        "anime",
				Title:            m.Title,
				Overview:         m.Overview,
				PosterURL:        m.PosterURL,
				Year:             m.Year,
				Rating:           m.Rating,
				BangumiID:        m.BangumiID,
				SubscribeKeyword: buildSubscribeKeyword(m.Title, m.Year),
			})
		}
	}
	if (source == "all" || source == "douban") && douban != nil {
		if m, err := douban.Search(ctx, query); err == nil && m != nil {
			yearValue, _ := strconv.Atoi(m.Year)
			typ := normalizeDoubanType(m.Type, mediaType)
			out = append(out, ExternalMediaResult{
				Source:           "douban",
				MediaType:        typ,
				Title:            m.Title,
				PosterURL:        m.Img,
				Year:             yearValue,
				Rating:           m.Rating,
				DoubanID:         m.DoubanID,
				SubscribeKeyword: buildSubscribeKeyword(m.Title, yearValue),
			})
		}
	}
	if (source == "all" || source == "tmdb") && discover != nil {
		items, err := discover.SearchTMDb(ctx, query, mediaType, pageNum, limit)
		if err != nil {
			return out, err
		}
		out = append(out, items...)
	}
	return limitExternalMedia(dedupeExternalMedia(out), limit), nil
}

func clampDiscoverLimit(limit int) int {
	if limit <= 0 {
		return 40
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func limitMatches(items []Match, limit int) []Match {
	limit = clampDiscoverLimit(limit)
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func limitExternalMedia(items []ExternalMediaResult, limit int) []ExternalMediaResult {
	limit = clampDiscoverLimit(limit)
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func pageExternalMedia(items []ExternalMediaResult, start, limit int) []ExternalMediaResult {
	if start < 0 {
		start = 0
	}
	limit = clampDiscoverLimit(limit)
	if start >= len(items) {
		return []ExternalMediaResult{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
