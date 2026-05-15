// Package handler — disk usage breakdown for the Storage tab.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func storageHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		bd, err := svc.Storage.Compute(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, bd)
	}
}
