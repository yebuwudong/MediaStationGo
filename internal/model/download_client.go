// Package model 定义下载客户端配置数据模型。
package model

// DownloadClient 下载客户端配置（qBittorrent / Transmission / Aria2）。
// 支持多客户端并行运行，一个标记为默认客户端。
type DownloadClient struct {
	Base
	Name      string `gorm:"size:128;not null" json:"name"`
	Type      string `gorm:"size:32;not null" json:"type"`       // qbittorrent / transmission / aria2
	Host      string `gorm:"size:512;not null" json:"host"`       // http://host:port
	Username  string `gorm:"size:256" json:"username"`
	Password  string `gorm:"size:1024" json:"-"`                 // AES加密存储
	IsDefault bool   `gorm:"default:false" json:"is_default"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	Extra     string `gorm:"type:text" json:"-"`                 // JSON配置, AES加密
}
