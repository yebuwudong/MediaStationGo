// Package handler — Emby/Jellyfin compatibility shim.
//
// Routes are mounted under /emby/* so existing Emby-aware clients
// (Infuse / VidHub / Kodi) point at MediaStationGo and discover the
// library through their familiar API. We do not implement write paths;
// the React UI stays the canonical control plane.
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embySystemInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, svc.Emby.SystemInfo())
	}
}

func embyListUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := svc.Emby.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, users)
	}
}

func embyViewsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.Views(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libraryID := c.Query("ParentId")
		limit, _ := strconv.Atoi(c.DefaultQuery("Limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("StartIndex", "0"))
		out, err := svc.Emby.Items(c.Request.Context(), libraryID, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}

func embyPlaybackInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.PlaybackInfo(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if out == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}
