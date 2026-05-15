// Package handler — live tasks board.
//
// /api/tasks aggregates running ffmpeg transcodes, qBittorrent torrents
// and recent scrape progress into a single snapshot suitable for the
// React Tasks panel. The panel can layer this REST snapshot on top of
// live WS events for instant updates.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func tasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		transcodes := svc.Transcoder.Active()
		_, torrents, _ := svc.Downloads.List(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{
			"transcodes": transcodes,
			"torrents":   torrents,
		})
	}
}
