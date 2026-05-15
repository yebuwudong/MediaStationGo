// Package handler — server-side file browser used by the React
// "select library path" dialog and the Storage tab.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func browseFilesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Query("path")
		max, _ := strconv.Atoi(c.DefaultQuery("max", "1000"))
		listing, err := svc.FileManager.List(path, max)
		if err != nil {
			if errors.Is(err, service.ErrPathOutOfBounds) {
				c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, listing)
	}
}
