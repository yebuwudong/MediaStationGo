package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// embyPlayingReq 播放进度上报请求。
//
// 数值字段使用 FlexInt64：真实客户端会把 PositionTicks / RunTimeTicks
// 写成字符串，把 PlaySessionId 写成数字。严格绑定会让进度上报失败，
// 直接后果是续播位置丢失。参考 emby_flexjson.go 的说明。
type embyPlayingReq struct {
	ItemId        string        `json:"ItemId"`
	MediaSourceId string        `json:"MediaSourceId"`
	PositionTicks model.FlexInt64 `json:"PositionTicks"`
	RunTimeTicks  model.FlexInt64 `json:"RunTimeTicks"`
	PlaySessionID string        `json:"PlaySessionId"`
}

func embyPlayingProgressHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			c.Status(http.StatusUnauthorized)
			return
		}
		var req embyPlayingReq
		// 进度上报只用于记录播放位置，不构成播放准入门槛。类型不匹配由
		// FlexInt64 吸收；此处仅忽略结构性错误，绝不因请求体拒绝上报，
		// 否则客户端每次播放都会从头开始。
		_ = c.ShouldBindJSON(&req)
		if req.ItemId == "" {
			req.ItemId = c.Query("ItemId")
		}
		if req.MediaSourceId == "" {
			req.MediaSourceId = firstQueryValue(c, "MediaSourceId", "MediaSourceID", "mediaSourceId", "media_source_id")
		}
		if req.PositionTicks == 0 {
			if v, err := strconv.ParseInt(c.Query("PositionTicks"), 10, 64); err == nil {
				req.PositionTicks = model.FlexInt64(v)
			}
		}
		if req.RunTimeTicks == 0 {
			if v, err := strconv.ParseInt(c.Query("RunTimeTicks"), 10, 64); err == nil {
				req.RunTimeTicks = model.FlexInt64(v)
			}
		}
		if req.ItemId == "" {
			c.Status(http.StatusOK)
			return
		}
		clientInfo := embyClientInfoFromRequest(c)
		if svc.Device != nil && svc.Device.IsTerminalKicked(c.Request.Context(), uid, clientInfo.DeviceID, clientInfo.DeviceName, clientInfo.Client) {
			c.Status(http.StatusUnauthorized)
			return
		}
		progressAccepted := true
		progress, err := svc.Emby.RecordProgressForMediaSourceResult(
			c.Request.Context(),
			uid,
			req.ItemId,
			req.MediaSourceId,
			req.PositionTicks.Int64(),
			req.RunTimeTicks.Int64(),
		)
		if err != nil {
			if errors.Is(err, service.ErrCloudPlaybackNotResolved) {
				progressAccepted = false
				if svc.Log != nil {
					svc.Log.Warn("ignored playback progress without successful cloud resolve",
						zap.String("user_id", uid),
						zap.String("media_id", req.ItemId),
						zap.String("media_source_id", req.MediaSourceId))
				}
			} else if errors.Is(err, service.ErrPlaybackMediaUnavailable) {
				embyError(c, http.StatusNotFound, "Item not found")
				return
			} else if errors.Is(err, service.ErrInvalidPlaybackProgress) {
				embyError(c, http.StatusBadRequest, "Invalid playback progress")
				return
			} else if errors.Is(err, service.ErrEmbyMediaSourceUnavailable) {
				embyError(c, http.StatusBadRequest, "Invalid MediaSourceId")
				return
			} else {
				if svc.Log != nil {
					svc.Log.Error("record playback progress failed", zap.Error(err))
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "record playback progress failed"})
				return
			}
		}
		stopped := strings.Contains(strings.ToLower(c.FullPath()+" "+c.Request.URL.Path), "stopped")
		if progressAccepted {
			recordPlaybackStats(c, svc, req.ItemId, req.PlaySessionID, clientInfo.DeviceID+clientInfo.DeviceName+clientInfo.Client+c.ClientIP(), clientInfo.Client, progress.PositionMs, progress.DurationMs, stopped)
		}
		if svc.Sessions != nil {
			svc.Sessions.RecordPlayback(c.Request.Context(), uid, "",
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP(),
				req.ItemId,
				req.PositionTicks.Int64(),
				req.RunTimeTicks.Int64(),
				stopped)
		}
		if svc.Device != nil && !stopped {
			svc.Device.RecordPlayback(c.Request.Context(), uid,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client)
		}
		c.Status(http.StatusNoContent)
	}
}

func embyFavoriteHandler(svc *service.Container, fav bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.SetFavorite(c.Request.Context(), uid, mid, fav); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"IsFavorite": fav})
	}
}

func embyMarkPlayedHandler(svc *service.Container, played bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.MarkPlayed(c.Request.Context(), uid, mid, played); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if played && svc.Device != nil {
			clientInfo := embyClientInfoFromRequest(c)
			svc.Device.RecordPlayback(c.Request.Context(), uid, clientInfo.DeviceID, clientInfo.DeviceName, clientInfo.Client)
		}
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"Played": played})
	}
}

func embyHideFromResumeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if mid == "" {
			mid = c.Param("id")
		}
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if len(mid) > 128 {
			embyError(c, http.StatusBadRequest, "Invalid media id")
			return
		}
		// HideFromResume is an action route, so standard clients default to hide.
		// Filmly also sends parameterless calls while synchronizing Resume; those
		// calls are ambiguous and must not mutate the persisted hidden state.
		hide := true
		rawHide := strings.TrimSpace(firstQueryValue(c, "Hide", "hide"))
		isFilmly := embyRequestIsFilmly(c)
		ignoreFilmlyMutation := rawHide == "" && isFilmly
		if rawHide != "" {
			parsed, err := strconv.ParseBool(rawHide)
			if err != nil {
				embyError(c, http.StatusBadRequest, "Invalid Hide value")
				return
			}
			hide = parsed
		}
		if !ignoreFilmlyMutation {
			if err := svc.Emby.SetHiddenFromResume(c.Request.Context(), uid, mid, hide); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		userData, found, err := svc.Emby.MediaUserData(c.Request.Context(), uid, mid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !found {
			embyError(c, http.StatusNotFound, "item not found")
			return
		}
		if isFilmly {
			embyAttachFilmlyUserDataIdentity(userData, mid)
		}
		c.JSON(http.StatusOK, userData)
	}
}
