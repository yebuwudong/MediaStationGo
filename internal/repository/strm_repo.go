// Package repository — STRM 文件记录数据访问层。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// STRMRepository persists model.STRMRecord records.
type STRMRepository struct{ db *gorm.DB }

// Create inserts a new STRM record.
func (r *STRMRepository) Create(ctx context.Context, s *model.STRMRecord) error {
	return r.db.WithContext(ctx).Create(s).Error
}

// CreateBatch inserts multiple STRM records.
func (r *STRMRepository) CreateBatch(ctx context.Context, records []model.STRMRecord) error {
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(records, 100).Error
}

// FindByID returns the STRM record by ID, or (nil, nil) when absent.
func (r *STRMRepository) FindByID(ctx context.Context, id string) (*model.STRMRecord, error) {
	var s model.STRMRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns STRM records with optional filters. Supports pagination.
// Filters: media_id, media_type, protocol
func (r *STRMRepository) List(ctx context.Context, filters map[string]string, offset, limit int) ([]model.STRMRecord, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.STRMRecord{})

	if mediaID, ok := filters["media_id"]; ok && mediaID != "" {
		q = q.Where("media_id = ?", mediaID)
	}
	if mediaType, ok := filters["media_type"]; ok && mediaType != "" {
		q = q.Where("media_type = ?", mediaType)
	}
	if protocol, ok := filters["protocol"]; ok && protocol != "" {
		q = q.Where("protocol = ?", protocol)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.STRMRecord
	err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

// Update updates a STRM record.
func (r *STRMRepository) Update(ctx context.Context, s *model.STRMRecord) error {
	return r.db.WithContext(ctx).Save(s).Error
}

// Delete removes a STRM record (soft-delete).
func (r *STRMRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.STRMRecord{}, "id = ?", id).Error
}

// FindByMediaID returns STRM records for a given media ID.
func (r *STRMRepository) FindByMediaID(ctx context.Context, mediaID string) ([]model.STRMRecord, error) {
	var rows []model.STRMRecord
	err := r.db.WithContext(ctx).Where("media_id = ?", mediaID).Find(&rows).Error
	return rows, err
}
