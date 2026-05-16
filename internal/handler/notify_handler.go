// Package handler — 通知渠道管理 HTTP 端点。
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// NotifyHandler 处理通知渠道的 CRUD 操作。
type NotifyHandler struct {
	svc *service.Container
	log *zap.Logger
}

// NewNotifyHandler 创建通知渠道处理器。
func NewNotifyHandler(svc *service.Container, log *zap.Logger) *NotifyHandler {
	return &NotifyHandler{svc: svc, log: log}
}

// notifyCreateRequest 创建通知渠道请求体。
type notifyCreateRequest struct {
	Name    string            `json:"name" binding:"required"`
	Type    string            `json:"type" binding:"required,oneof=telegram wechat bark webhook email"`
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config" binding:"required"`
	Events  []string          `json:"events"`
}

// notifyUpdateRequest 更新通知渠道请求体。
type notifyUpdateRequest struct {
	Name    string            `json:"name"`
	Enabled *bool             `json:"enabled"`
	Config  map[string]string `json:"config"`
	Events  []string          `json:"events"`
}

// Create 创建新的通知渠道。
func (h *NotifyHandler) Create(c *gin.Context) {
	var req notifyCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}

	ctx := c.Request.Context()

	// 验证配置
	if err := h.svc.Notify.ValidateChannelConfig(req.Type, req.Config); err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, "配置验证失败: "+err.Error())
		return
	}

	// 加密配置
	configJSON, _ := json.Marshal(req.Config)
	configStr := string(configJSON)
	if h.svc.Crypto != nil {
		configStr = h.svc.Crypto.Encrypt(configStr)
	}

	// 序列化事件列表
	eventsJSON, _ := json.Marshal(req.Events)
	eventsStr := string(eventsJSON)

	channel := &model.NotifyChannel{
		Name:    req.Name,
		Type:    req.Type,
		Enabled: req.Enabled,
		Config:  configStr,
		Events:  eventsStr,
	}

	if err := h.svc.Repo.NotifyChannel.Create(ctx, channel); err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "创建失败: "+err.Error())
		return
	}

	Success(c, channel)
}

// List 返回所有通知渠道。
func (h *NotifyHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	channels, err := h.svc.Repo.NotifyChannel.List(ctx)
	if err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "查询失败")
		return
	}
	Success(c, channels)
}

// Get 返回指定通知渠道详情。
func (h *NotifyHandler) Get(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()
	channel, err := h.svc.Repo.NotifyChannel.FindByID(ctx, id)
	if err != nil || channel == nil {
		Error(c, http.StatusNotFound, ErrNotFound, "通知渠道不存在")
		return
	}
	Success(c, channel)
}

// Update 更新通知渠道。
func (h *NotifyHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req notifyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
		return
	}

	ctx := c.Request.Context()
	channel, err := h.svc.Repo.NotifyChannel.FindByID(ctx, id)
	if err != nil || channel == nil {
		Error(c, http.StatusNotFound, ErrNotFound, "通知渠道不存在")
		return
	}

	if req.Name != "" {
		channel.Name = req.Name
	}
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}

	// 更新配置
	if len(req.Config) > 0 {
		if err := h.svc.Notify.ValidateChannelConfig(channel.Type, req.Config); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "配置验证失败: "+err.Error())
			return
		}
		configJSON, _ := json.Marshal(req.Config)
		configStr := string(configJSON)
		if h.svc.Crypto != nil {
			configStr = h.svc.Crypto.Encrypt(configStr)
		}
		channel.Config = configStr
	}

	// 更新事件列表
	if req.Events != nil {
		eventsJSON, _ := json.Marshal(req.Events)
		channel.Events = string(eventsJSON)
	}

	if err := h.svc.Repo.NotifyChannel.Update(ctx, channel); err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "更新失败")
		return
	}

	Success(c, channel)
}

// Delete 删除通知渠道。
func (h *NotifyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	_, err := h.svc.Repo.NotifyChannel.FindByID(ctx, id)
	if err != nil {
		Error(c, http.StatusNotFound, ErrNotFound, "通知渠道不存在")
		return
	}

	if delErr := h.svc.Repo.NotifyChannel.Delete(ctx, id); delErr != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "删除失败")
		return
	}

	SuccessWithMessage(c, "已删除", nil)
}

// Test 发送测试通知。
func (h *NotifyHandler) Test(c *gin.Context) {
	id := c.Param("id")
	ctx := c.Request.Context()

	if err := h.svc.Notify.SendTest(ctx, id); err != nil {
		Error(c, http.StatusBadRequest, ErrExternal, "测试通知发送失败: "+err.Error())
		return
	}

	SuccessWithMessage(c, "测试通知已发送", nil)
}

// GetTypes 返回支持的通知渠道类型列表。
func (h *NotifyHandler) GetTypes(c *gin.Context) {
	types := h.svc.Notify.GetProviderTypes()
	Success(c, types)
}
