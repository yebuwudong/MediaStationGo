// Package repository — PT 站点数据访问层。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SiteRepository persists model.Site records.
type SiteRepository struct{ db *gorm.DB }

// Create inserts a new site.
func (r *SiteRepository) Create(ctx context.Context, s *model.Site) error {
	return r.db.WithContext(ctx).Create(s).Error
}

// FindByID returns the site by ID, or (nil, nil) when absent.
func (r *SiteRepository) FindByID(ctx context.Context, id string) (*model.Site, error) {
	var s model.Site
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns all sites ordered by name.
func (r *SiteRepository) List(ctx context.Context) ([]model.Site, error) {
	var rows []model.Site
	err := r.db.WithContext(ctx).Order("name asc").Find(&rows).Error
	return rows, err
}

// ListEnabled returns all enabled sites.
func (r *SiteRepository) ListEnabled(ctx context.Context) ([]model.Site, error) {
	var rows []model.Site
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("name asc").Find(&rows).Error
	return rows, err
}

// Update updates site fields.
func (r *SiteRepository) Update(ctx context.Context, s *model.Site) error {
	return r.db.WithContext(ctx).Save(s).Error
}

// Delete removes a site (soft-delete).
func (r *SiteRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Site{}, "id = ?", id).Error
}
