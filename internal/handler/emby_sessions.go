package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embySessionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Sessions == nil {
			c.JSON(http.StatusOK, []any{})
			return
		}
		out := make([]gin.H, 0)
		for _, sess := range svc.Sessions.List(c.Request.Context()) {
			last := sess.LastActivityAt
			itemID := sess.ItemID
			playState := gin.H{
				"PositionTicks": sess.PositionTicks,
				"IsPaused":      sess.IsPaused,
				"PlayMethod":    "DirectStream",
				"CanSeek":       true,
			}
			row := gin.H{
				"Id":                    sess.ID,
				"ServerId":              "mediastation-go-001",
				"Client":                sess.Client,
				"DeviceId":              sess.DeviceID,
				"DeviceName":            sess.DeviceName,
				"UserId":                sess.UserID,
				"UserName":              sess.UserName,
				"LastActivityDate":      last,
				"RemoteEndPoint":        sess.RemoteEndPoint,
				"PlayState":             playState,
				"SupportsRemoteControl": true,
			}
			if itemID != "" && sess.IsPlaying {
				row["NowPlayingItem"] = gin.H{"Id": itemID}
			}
			out = append(out, row)
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, out)
	}
}

func embySessionLogoutHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Sessions != nil {
			uid := embyUserID(c)
			clientInfo := embyClientInfoFromRequest(c)
			svc.Sessions.Logout(c.Request.Context(), uid, clientInfo.DeviceID, c.ClientIP())
		}
		c.Status(http.StatusNoContent)
	}
}
