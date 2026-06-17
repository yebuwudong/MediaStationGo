// Package handler — multi-section discover endpoints.
//
// The Vue DiscoverView paginates a configurable list of "sections"
// (trending day/week, popular movies, top rated, etc.) and asks the
// backend for a feed keyed by section name. We mirror that surface so
// the React DiscoverPage can render the same rails without a rewrite.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// discoverSectionsHandler returns the catalog of sections the UI can
// pick from. The names match the upstream Vue UI so existing settings
// keep working.
func discoverSectionsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"sections": []gin.H{
				{"key": "tmdb_trending_day", "label": "TMDb 今日趋势", "provider": "tmdb"},
				{"key": "tmdb_trending_week", "label": "TMDb 本周热门", "provider": "tmdb"},
				{"key": "tmdb_popular_movie", "label": "TMDb 热门电影", "provider": "tmdb"},
				{"key": "tmdb_popular_tv", "label": "TMDb 热门剧集", "provider": "tmdb"},
				{"key": "tmdb_top_rated_movie", "label": "TMDb 高分电影", "provider": "tmdb"},
				{"key": "douban_hot_movie", "label": "豆瓣热门电影", "provider": "douban"},
				{"key": "douban_hot_tv", "label": "豆瓣热门剧集", "provider": "douban"},
				{"key": "douban_top_movie", "label": "豆瓣高分电影", "provider": "douban"},
				{"key": "bangumi_calendar", "label": "Bangumi 每日放送", "provider": "bangumi"},
			},
		})
	}
}

// discoverFeedHandler resolves one or more section keys (?sections=a,b)
// to TMDb / Douban / Bangumi rails and returns the joined results keyed by
// section name. Unknown keys are silently dropped so URL typos don't break
// the page.
func discoverFeedHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		keys := strings.Split(c.DefaultQuery("sections", "tmdb_trending_day,tmdb_popular_movie,douban_hot_movie,bangumi_calendar"), ",")
		page := parsePositiveQueryInt(c, "page", 1, 50)
		limit := parsePositiveQueryInt(c, "limit", 40, 100)
		out := gin.H{}
		for _, raw := range keys {
			k := strings.TrimSpace(raw)
			items, err := discoverSectionItems(c.Request.Context(), svc, k, page, limit)
			if err != nil {
				svc.Log.Debug("discover fetch failed")
				items = nil
			}
			out[k] = items
		}
		c.JSON(http.StatusOK, out)
	}
}

func discoverSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if query == "" {
			c.JSON(http.StatusOK, gin.H{"items": []service.ExternalMediaResult{}})
			return
		}
		source := strings.ToLower(strings.TrimSpace(c.DefaultQuery("source", "all")))
		mediaType := strings.ToLower(strings.TrimSpace(c.Query("type")))
		page := parsePositiveQueryInt(c, "page", 1, 50)
		limit := parsePositiveQueryInt(c, "limit", 40, 100)
		items, err := service.SearchSubscriptionCatalog(c.Request.Context(), query, mediaType, source, page, limit, svc.Discover, svc.Douban, svc.Bangumi)
		if err != nil {
			svc.Log.Debug("discover search failed")
			c.JSON(http.StatusOK, gin.H{"items": []service.ExternalMediaResult{}, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func discoverSectionItems(ctx context.Context, svc *service.Container, k string, page, limit int) ([]service.ExternalMediaResult, error) {
	switch k {
	case "tmdb_trending_day", "tmdb_trending_week", "tmdb_popular_movie", "tmdb_popular_tv", "tmdb_top_rated_movie",
		"trending_day", "trending_week", "popular_movie", "popular_tv", "top_rated_movie", "upcoming_movie":
		return svc.Discover.TMDbSectionPage(ctx, k, page, limit)
	case "douban_hot_movie", "douban_hot_tv", "douban_top_movie":
		if svc.Douban == nil {
			return []service.ExternalMediaResult{}, nil
		}
		return svc.Douban.DiscoverPage(ctx, k, page, limit)
	case "bangumi_calendar":
		if svc.Bangumi == nil {
			return []service.ExternalMediaResult{}, nil
		}
		return svc.Bangumi.CalendarPage(ctx, page, limit)
	default:
		return []service.ExternalMediaResult{}, nil
	}
}

func parsePositiveQueryInt(c *gin.Context, key string, fallback, maxValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}
