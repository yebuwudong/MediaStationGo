// Package service — 权限管理服务。
package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// PermissionService 负责用户细粒度权限管理。
type PermissionService struct {
	cfg  *config.Config
	log  *zap.Logger
	repo *repository.Container
}

// NewPermissionService 创建权限服务实例。
func NewPermissionService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *PermissionService {
	return &PermissionService{cfg: cfg, log: log, repo: repo}
}

// 权限服务错误定义。
var (
	ErrPermissionDenied = errors.New("permission denied")
	ErrPermissionNotFound = errors.New("permission not found")
)

// GetByUserID 获取用户的权限记录，不存在则返回默认权限。
func (s *PermissionService) GetByUserID(ctx context.Context, userID string) (*model.UserPermission, error) {
	perm, err := s.repo.Permission.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if perm == nil {
		// 返回默认权限但不持久化
		return model.NewDefaultPermission(userID), nil
	}
	return perm, nil
}

// EnsureForUser 确保用户拥有权限记录，不存在则创建默认权限。
func (s *PermissionService) EnsureForUser(ctx context.Context, userID string) (*model.UserPermission, error) {
	perm, err := s.repo.Permission.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if perm != nil {
		return perm, nil
	}
	// 创建默认权限
	defaultPerm := model.NewDefaultPermission(userID)
	if err := s.repo.Permission.Upsert(ctx, defaultPerm); err != nil {
		return nil, err
	}
	return defaultPerm, nil
}

// Check 检查用户是否拥有特定权限。
// 权限检查优先级：admin → 全权限 > plus → 全权限 > user → 查表
func (s *PermissionService) Check(ctx context.Context, userID, role, tier, permissionKey string) bool {
	// admin 拥有所有权限
	if role == "admin" {
		return true
	}

	// plus 用户拥有所有权限
	if tier == "plus" {
		return true
	}

	// free 用户查表
	perm, err := s.GetByUserID(ctx, userID)
	if err != nil || perm == nil {
		return false
	}

	permMap := perm.PermissionMap()
	hasPermission, ok := permMap[permissionKey]
	if !ok {
		return false
	}
	return hasPermission
}

// Update 更新用户的权限。
func (s *PermissionService) Update(ctx context.Context, userID string, updates map[string]bool) error {
	// 确保权限记录存在
	if _, err := s.EnsureForUser(ctx, userID); err != nil {
		return err
	}
	return s.repo.Permission.Update(ctx, userID, updates)
}

// ResetToDefault 将用户权限重置为默认值。
func (s *PermissionService) ResetToDefault(ctx context.Context, userID string) error {
	defaultPerm := model.NewDefaultPermission(userID)
	return s.repo.Permission.Upsert(ctx, defaultPerm)
}

// GetPermissionMap 获取用户权限的 map 表示。
func (s *PermissionService) GetPermissionMap(ctx context.Context, userID string) (map[string]bool, error) {
	perm, err := s.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return perm.PermissionMap(), nil
}

// IsSuperUser 检查用户是否为超级用户（admin 或 plus）。
func (s *PermissionService) IsSuperUser(role, tier string) bool {
	return role == "admin" || tier == "plus"
}
