// Package model — PT 站点配置数据模型。
package model

import (
	"time"
)

// Site PT 站点配置。
type Site struct {
	Base
	Name        string     `gorm:"size:128;not null" json:"name"`
	Type        string     `gorm:"size:32;not null" json:"type"`          // nexusphp / gazelle / unit3d / mteam / discuz / custom_rss
	URL         string     `gorm:"size:512;not null" json:"url"`
	AuthType    string     `gorm:"size:32;not null" json:"auth_type"`     // cookie / api_key / auth_header
	Cookie      string     `gorm:"type:text" json:"-"`                    // AES 加密
	APIKey      string     `gorm:"type:text" json:"-"`                    // AES 加密
	AuthHeader  string     `gorm:"type:text" json:"-"`                    // AES 加密
	Enabled     bool       `gorm:"default:true" json:"enabled"`
	IsDefault   bool       `gorm:"default:false" json:"is_default"`
	Extra       string     `gorm:"type:text" json:"-"`                    // JSON 扩展配置, AES 加密
	LastError   string     `gorm:"size:1024" json:"last_error"`
	LastCheckAt *time.Time `json:"last_check_at"`
}

// SiteType 返回支持的站点类型列表。
func SiteTypes() []string {
	return []string{"nexusphp", "gazelle", "unit3d", "mteam", "discuz", "custom_rss"}
}

// AuthTypes 返回支持的认证方式列表。
func AuthTypes() []string {
	return []string{"cookie", "api_key", "auth_header"}
}
