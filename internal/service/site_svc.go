// Package service — PT 站点管理服务。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// 站点管理错误码。
var (
	ErrSiteNotFound     = errors.New("site not found")
	ErrSiteAuthFailed   = errors.New("site authentication failed")
	ErrSiteTypeInvalid  = errors.New("invalid site type")
	ErrSiteAuthInvalid  = errors.New("invalid auth type")
)

// SiteService 站点管理服务。
type SiteService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
}

// NewSiteService 创建站点管理服务。
func NewSiteService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *SiteService {
	return &SiteService{log: log, repo: repo, crypto: crypto}
}

// Create 创建站点，加密敏感字段。
func (s *SiteService) Create(ctx context.Context, site *model.Site) (*model.Site, error) {
	if !isValidSiteType(site.Type) {
		return nil, ErrSiteTypeInvalid
	}
	if !isValidAuthType(site.AuthType) {
		return nil, ErrSiteAuthInvalid
	}

	// 加密敏感字段
	s.encryptSite(site)

	if err := s.repo.Site.Create(ctx, site); err != nil {
		s.log.Error("create site failed", zap.Error(err))
		return nil, err
	}

	return site, nil
}

// GetByID 获取站点（敏感字段解密）。
func (s *SiteService) GetByID(ctx context.Context, id string) (*model.Site, error) {
	site, err := s.repo.Site.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, ErrSiteNotFound
	}

	s.decryptSite(site)
	return site, nil
}

// List 获取所有站点（不含敏感字段）。
func (s *SiteService) List(ctx context.Context) ([]model.Site, error) {
	sites, err := s.repo.Site.List(ctx)
	if err != nil {
		return nil, err
	}
	return sites, nil
}

// Update 更新站点。
func (s *SiteService) Update(ctx context.Context, site *model.Site) (*model.Site, error) {
	existing, err := s.repo.Site.FindByID(ctx, site.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrSiteNotFound
	}

	if !isValidSiteType(site.Type) {
		return nil, ErrSiteTypeInvalid
	}
	if !isValidAuthType(site.AuthType) {
		return nil, ErrSiteAuthInvalid
	}

	s.encryptSite(site)

	if err := s.repo.Site.Update(ctx, site); err != nil {
		s.log.Error("update site failed", zap.Error(err))
		return nil, err
	}

	return site, nil
}

// Delete 删除站点。
func (s *SiteService) Delete(ctx context.Context, id string) error {
	existing, err := s.repo.Site.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrSiteNotFound
	}
	return s.repo.Site.Delete(ctx, id)
}

// Authenticate 测试站点认证。
func (s *SiteService) Authenticate(ctx context.Context, id string) error {
	site, err := s.repo.Site.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if site == nil {
		return ErrSiteNotFound
	}

	cfg, err := s.toSiteConfig(site)
	if err != nil {
		return err
	}

	adapter := GetAdapterForType(site.Type)
	if err := adapter.Authenticate(ctx, *cfg); err != nil {
		// 更新错误状态
		now := time.Now()
		site.LastError = err.Error()
		site.LastCheckAt = &now
		_ = s.repo.Site.Update(ctx, site)
		return ErrSiteAuthFailed
	}

	// 清除错误状态
	now := time.Now()
	site.LastError = ""
	site.LastCheckAt = &now
	_ = s.repo.Site.Update(ctx, site)
	return nil
}

// GetSiteConfig 获取解密后的站点配置（供内部使用）。
func (s *SiteService) GetSiteConfig(ctx context.Context, id string) (*SiteConfig, error) {
	site, err := s.repo.Site.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, ErrSiteNotFound
	}
	return s.toSiteConfig(site)
}

// encryptSite 加密站点敏感字段。
func (s *SiteService) encryptSite(site *model.Site) {
	if site.Cookie != "" {
		site.Cookie = s.crypto.Encrypt(site.Cookie)
	}
	if site.APIKey != "" {
		site.APIKey = s.crypto.Encrypt(site.APIKey)
	}
	if site.AuthHeader != "" {
		site.AuthHeader = s.crypto.Encrypt(site.AuthHeader)
	}
	if site.Extra != "" {
		site.Extra = s.crypto.Encrypt(site.Extra)
	}
}

// decryptSite 解密站点敏感字段。
func (s *SiteService) decryptSite(site *model.Site) {
	if site.Cookie != "" {
		site.Cookie = s.crypto.Decrypt(site.Cookie)
	}
	if site.APIKey != "" {
		site.APIKey = s.crypto.Decrypt(site.APIKey)
	}
	if site.AuthHeader != "" {
		site.AuthHeader = s.crypto.Decrypt(site.AuthHeader)
	}
	if site.Extra != "" {
		site.Extra = s.crypto.Decrypt(site.Extra)
	}
}

// toSiteConfig 将 model.Site 转换为 SiteConfig（解密后）。
func (s *SiteService) toSiteConfig(site *model.Site) (*SiteConfig, error) {
	cfg := &SiteConfig{
		Name:     site.Name,
		Type:     site.Type,
		URL:      strings.TrimRight(site.URL, "/"),
		AuthType: site.AuthType,
		Extra:    map[string]string{},
	}

	// 解密
	if site.Cookie != "" {
		cfg.Cookie = s.crypto.Decrypt(site.Cookie)
	}
	if site.APIKey != "" {
		cfg.APIKey = s.crypto.Decrypt(site.APIKey)
	}
	if site.AuthHeader != "" {
		cfg.AuthHeader = s.crypto.Decrypt(site.AuthHeader)
	}
	if site.Extra != "" {
		dec := s.crypto.Decrypt(site.Extra)
		if dec != "" {
			if err := json.Unmarshal([]byte(dec), &cfg.Extra); err != nil {
				s.log.Warn("parse site extra config failed", zap.Error(err))
			}
		}
	}

	return cfg, nil
}

// isValidSiteType 检查站点类型是否有效。
func isValidSiteType(siteType string) bool {
	for _, t := range model.SiteTypes() {
		if t == siteType {
			return true
		}
	}
	return false
}

// isValidAuthType 检查认证方式是否有效。
func isValidAuthType(authType string) bool {
	for _, t := range model.AuthTypes() {
		if t == authType {
			return true
		}
	}
	return false
}
