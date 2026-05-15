// Package handler — scheduled jobs admin page.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func schedulerStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"jobs": svc.Scheduler.Status()})
	}
}

func schedulerRunHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if err := svc.Scheduler.RunNow(c.Request.Context(), name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
