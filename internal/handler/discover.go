// Package handler — TMDb discovery endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func trendingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Discover.Trending(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func popularHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Discover.Popular(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}
