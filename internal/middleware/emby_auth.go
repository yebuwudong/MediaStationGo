// Package middleware — Emby API 兼容层认证中间件。
// 支持 X-Emby-Token / Bearer / URL token / Username+Password 四种认证方式。
package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// EmbyCtxUserID 是 Emby 认证中间件设置的用户 ID 上下文键。
const EmbyCtxUserID = "emby_user_id"

// EmbyAuthRequired Emby 认证中间件。
// 按优先级尝试以下认证方式：
// 1. X-Emby-Token 请求头
// 2. Authorization: Bearer <token> 请求头
// 3. ?token=<token> URL 参数
// 4. （仅 AuthenticateByName 端点）POST body 中的 Username+Password
func EmbyAuthRequired(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""

		// 1. X-Emby-Token 头
		if t := c.GetHeader("X-Emby-Token"); t != "" {
			token = t
		}

		// 2. Authorization: Bearer <token> 或 Emby <token>
		if token == "" {
			if authHeader := c.GetHeader("Authorization"); authHeader != "" {
				// Strip "Bearer " or "Emby " prefix
				for _, prefix := range []string{"Bearer ", "Emby "} {
					if len(authHeader) > len(prefix) && authHeader[:len(prefix)] == prefix {
						token = authHeader[len(prefix):]
						break
					}
				}
				if token == "" {
					token = authHeader
				}
			}
		}

		// 3. URL 参数 token
		if token == "" {
			if t := c.Query("token"); t != "" {
				token = t
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"Code":    40101,
				"Message": "Unauthorized",
			})
			c.Abort()
			return
		}

		// 解析 JWT
		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !parsed.Valid || claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"Code":    40101,
				"Message": "Invalid token",
			})
			c.Abort()
			return
		}

		c.Set(EmbyCtxUserID, claims.UserID)
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserRole, claims.Role)
		c.Set(CtxUserTier, claims.Tier)
		c.Next()
	}
}

// GetEmbyUserID 从上下文中获取 Emby 用户 ID。
func GetEmbyUserID(c *gin.Context) string {
	if uid, exists := c.Get(EmbyCtxUserID); exists {
		return uid.(string)
	}
	return ""
}
