package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func siteCategoriesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Site.Categories(c.Request.Context(), c.Query("site_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func siteBrowseHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		includeAdult, _ := strconv.ParseBool(c.Query("include_adult"))
		out, err := svc.Site.Browse(c.Request.Context(), service.SiteBrowseParams{
			SiteID:       c.Query("site_id"),
			Keyword:      firstNonEmptyString(c.Query("keyword"), c.Query("q")),
			Category:     c.Query("category"),
			Page:         page,
			IncludeAdult: includeAdult,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func siteDetailHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := svc.Site.Detail(c.Request.Context(), c.Query("site_id"), c.Query("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, detail)
	}
}

type siteDownloadReq struct {
	SiteID         string `json:"site_id"`
	ID             string `json:"id"`
	Title          string `json:"title"`
	DownloadURL    string `json:"download_url"`
	TorrentURL     string `json:"torrent_url"`
	PosterURL      string `json:"poster_url"`
	BackdropURL    string `json:"backdrop_url"`
	Overview       string `json:"overview"`
	SavePath       string `json:"save_path"`
	MediaType      string `json:"media_type"`
	MediaCategory  string `json:"media_category"`
	SourceCategory string `json:"source_category"`
}

func enrichSiteTorrentDetailMeta(ctx context.Context, svc *service.Container, siteID, torrentID string, meta service.DownloadTaskMeta) service.DownloadTaskMeta {
	if svc == nil || svc.Site == nil || strings.TrimSpace(siteID) == "" || strings.TrimSpace(torrentID) == "" {
		return meta
	}
	detail, err := svc.Site.Detail(ctx, siteID, torrentID)
	if err != nil || detail == nil {
		return meta
	}
	if strings.TrimSpace(meta.Title) == "" {
		meta.Title = detail.Title
	}
	if strings.TrimSpace(meta.PosterURL) == "" {
		meta.PosterURL = detail.PosterURL
	}
	if strings.TrimSpace(meta.BackdropURL) == "" {
		meta.BackdropURL = detail.BackdropURL
	}
	if strings.TrimSpace(meta.Overview) == "" {
		meta.Overview = detail.Description
	}
	if strings.TrimSpace(meta.IMDBID) == "" {
		meta.IMDBID = detail.ImdbID
	}
	if meta.TMDbID <= 0 {
		meta.TMDbID = parseSiteTMDbID(detail.TMDbID)
	}
	if strings.TrimSpace(meta.DoubanID) == "" {
		meta.DoubanID = detail.DoubanID
	}
	return meta
}

func parseSiteTMDbID(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func siteDownloadHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteDownloadReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		raw := firstNonEmptyString(req.DownloadURL, req.TorrentURL)
		realURL, err := svc.Site.DownloadURL(c.Request.Context(), req.SiteID, req.ID, raw)
		if err != nil || strings.TrimSpace(realURL) == "" {
			if err == nil {
				err = errors.New("download url unavailable")
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		meta := service.DownloadTaskMeta{
			Title:          req.Title,
			PosterURL:      req.PosterURL,
			BackdropURL:    req.BackdropURL,
			Overview:       req.Overview,
			MediaType:      req.MediaType,
			MediaCategory:  req.MediaCategory,
			SourceCategory: req.SourceCategory,
		}
		meta = enrichSiteTorrentDetailMeta(c.Request.Context(), svc, req.SiteID, req.ID, meta)
		meta = enrichDownloadTaskMeta(c.Request.Context(), svc, meta, firstNonEmptyString(req.Title, meta.Title, realURL), req.MediaType)
		task, err := svc.Downloads.AddDownloadWithMeta(c.Request.Context(), uid.(string), realURL, req.SavePath, meta)
		if err != nil {
			if errors.Is(err, service.ErrMediaAlreadyInLibrary) {
				c.JSON(http.StatusConflict, gin.H{"error": "media already exists in library"})
				return
			}
			if errors.Is(err, service.ErrDownloadAlreadyExists) {
				c.JSON(http.StatusOK, task)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Audit.Record(c.Request.Context(), uid.(string), "site.download", redactDownloadURL(realURL), c.ClientIP(), "")
		c.JSON(http.StatusCreated, task)
	}
}

type siteSubscribeReq struct {
	SiteID        string `json:"site_id"`
	ID            string `json:"id"`
	Category      string `json:"category"`
	IncludeAdult  bool   `json:"include_adult"`
	Name          string `json:"name"`
	Keyword       string `json:"keyword"`
	Filter        string `json:"filter"`
	MediaType     string `json:"media_type"`
	MediaCategory string `json:"media_category"`
	PosterURL     string `json:"poster_url"`
	BackdropURL   string `json:"backdrop_url"`
	Overview      string `json:"overview"`
	SavePath      string `json:"save_path"`
	Enabled       *bool  `json:"enabled"`
}

func siteSubscribeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req siteSubscribeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		keyword := strings.TrimSpace(firstNonEmptyString(req.Keyword, req.Filter, req.Name))
		if keyword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = keyword
		}
		uid, _ := c.Get(middleware.CtxUserID)
		sub := &model.Subscription{
			UserID:        uid.(string),
			Name:          name,
			FeedURL:       service.SiteSearchURL(keyword, req.SiteID, req.Category, req.IncludeAdult),
			Filter:        firstNonEmptyString(req.Filter, keyword),
			MediaType:     req.MediaType,
			MediaCategory: req.MediaCategory,
			PosterURL:     req.PosterURL,
			BackdropURL:   req.BackdropURL,
			Overview:      req.Overview,
			SavePath:      req.SavePath,
			SearchMode:    "keyword",
			Source:        "site_search",
			Enabled:       enabled,
		}
		if detailMeta := enrichSiteTorrentDetailMeta(c.Request.Context(), svc, req.SiteID, req.ID, service.DownloadTaskMeta{
			Title:       sub.Name,
			PosterURL:   sub.PosterURL,
			BackdropURL: sub.BackdropURL,
			Overview:    sub.Overview,
		}); detailMeta.Title != "" || detailMeta.PosterURL != "" || detailMeta.BackdropURL != "" || detailMeta.Overview != "" || detailMeta.IMDBID != "" || detailMeta.TMDbID > 0 || detailMeta.DoubanID != "" {
			sub.Name = firstNonEmptyString(sub.Name, detailMeta.Title)
			sub.PosterURL = firstNonEmptyString(detailMeta.PosterURL, sub.PosterURL)
			sub.BackdropURL = firstNonEmptyString(detailMeta.BackdropURL, sub.BackdropURL)
			sub.Overview = firstNonEmptyString(sub.Overview, detailMeta.Overview)
			sub.IMDBID = firstNonEmptyString(sub.IMDBID, detailMeta.IMDBID)
			if sub.TMDbID <= 0 {
				sub.TMDbID = detailMeta.TMDbID
			}
			sub.DoubanID = firstNonEmptyString(sub.DoubanID, detailMeta.DoubanID)
		}
		enrichSubscriptionArtwork(c.Request.Context(), svc, sub)
		if err := svc.Subscription.Create(c.Request.Context(), sub); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		enriched := []model.Subscription{*sub}
		svc.Subscription.EnrichProgress(c.Request.Context(), enriched)
		*sub = enriched[0]
		queued := 0
		if enabled {
			if n, err := svc.Subscription.RunNow(c.Request.Context(), sub.ID); err == nil {
				queued = n
			}
		}
		c.JSON(http.StatusCreated, gin.H{"subscription": sub, "queued": queued})
	}
}
