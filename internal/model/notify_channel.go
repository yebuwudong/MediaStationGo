// Package model 定义通知渠道配置数据模型。
package model

// NotifyChannel 通知渠道配置。
// 支持多种通知渠道：telegram / wechat / bark / webhook / email。
// Events 字段存储 JSON array，表示该渠道订阅的事件类型。
type NotifyChannel struct {
	Base
	Name    string `gorm:"size:128;not null" json:"name"`
	Type    string `gorm:"size:32;not null" json:"type"`     // telegram / wechat / bark / webhook / email
	Enabled bool   `gorm:"default:true" json:"enabled"`
	Config  string `gorm:"type:text" json:"-"`               // JSON配置, AES加密
	Events  string `gorm:"type:text" json:"events"`           // 订阅的事件列表, JSON array
}
