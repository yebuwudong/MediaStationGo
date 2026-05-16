// Package handler — 统一响应格式和错误码。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ─── 错误码定义 ───────────────────────────────────────────────────────────────

// 统一错误码
const (
	ErrOK            = 0
	ErrInvalidParams = 40001
	ErrUnauthorized  = 40101
	ErrForbidden     = 40301
	ErrNotFound      = 40401
	ErrConflict      = 40901
	ErrInternal      = 50001
	ErrExternal      = 50201
	ErrEncryptFailed = 50801
)

// ─── Response Helpers ─────────────────────────────────────────────────────────

// APIResponse 统一 API 响应格式。
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// PaginatedResponse 分页响应格式。
type PaginatedResponse struct {
	Code     int         `json:"code"`
	Message  string      `json:"message"`
	Data     interface{} `json:"data"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// Success 返回成功响应。
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Code: 0, Message: "ok", Data: data})
}

// SuccessWithMessage 返回带消息的成功响应。
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Code: 0, Message: message, Data: data})
}

// Error 返回错误响应。
func Error(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, APIResponse{Code: code, Message: message, Data: nil})
}

// Paginated 返回分页响应。
func Paginated(c *gin.Context, items interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, PaginatedResponse{
		Code:     0,
		Message:  "ok",
		Data:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
