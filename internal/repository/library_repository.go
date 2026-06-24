package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// LibraryRepository persists model.Library records.
type LibraryRepository struct{ db *gorm.DB }

// Create persists a new library row.
func (r *LibraryRepository) Create(ctx context.Context, l *model.Library) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// List returns all enabled+disabled libraries.
func (r *LibraryRepository) List(ctx context.Context) ([]model.Library, error) {
	var ls []model.Library
	err := r.db.WithContext(ctx).Order("created_at asc").Find(&ls).Error
	return ls, err
}

// FindByID returns the library, or (nil, nil) when missing.
func (r *LibraryRepository) FindByID(ctx context.Context, id string) (*model.Library, error) {
	var l model.Library
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Delete removes a library and (soft) cascades to its media via repository
// callers; we do not run CASCADE here to keep this method narrow.
func (r *LibraryRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Library{}, "id = ?", id).Error
}
