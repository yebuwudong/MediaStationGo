// Package handler — AI integration endpoints.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type smartSearchReq struct {
	Query string `json:"query" binding:"required"`
}

func smartSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req smartSearchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		intent, err := svc.AI.SmartSearch(c.Request.Context(), req.Query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Run the actual library search using the cleaned query so the
		// caller can render results in one round-trip.
		items, _ := svc.Media.SearchMedia(c.Request.Context(), intent.Query, 60)
		c.JSON(http.StatusOK, gin.H{
			"intent": intent,
			"items":  items,
		})
	}
}

func aiRecommendHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		hist, err := svc.Playback.RecentHistory(c.Request.Context(), toString(uid), 10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		titles := make([]string, 0, len(hist))
		for _, h := range hist {
			if h.Media != nil && strings.TrimSpace(h.Media.Title) != "" {
				titles = append(titles, h.Media.Title)
			}
		}
		out, err := svc.AI.Recommend(c.Request.Context(), titles, 8)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"titles": out})
	}
}

func aiStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"enabled":  svc.AI.Enabled(),
			"provider": svc.Cfg.AI.Provider,
			"model":    svc.Cfg.AI.Model,
		})
	}
}
