package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type localMediaWriteBatch struct {
	scanner *ScannerService
	ctx     context.Context
	res     *ScanResult
	limit   int
	items   []localMediaWriteItem
}

type localMediaWriteItem struct {
	path  string
	media *model.Media
	after func()
}

func newLocalMediaWriteBatch(scanner *ScannerService, ctx context.Context, res *ScanResult, limit int) *localMediaWriteBatch {
	if limit <= 0 {
		limit = 100
	}
	return &localMediaWriteBatch{scanner: scanner, ctx: ctx, res: res, limit: limit}
}

func (b *localMediaWriteBatch) Add(path string, media *model.Media) {
	b.AddWithAfter(path, media, nil)
}

func (b *localMediaWriteBatch) AddWithAfter(path string, media *model.Media, after func()) {
	if b == nil || b.scanner == nil || media == nil {
		return
	}
	if media.ScrapeStatus == "" {
		media.ScrapeStatus = "pending"
	}
	b.items = append(b.items, localMediaWriteItem{path: path, media: media, after: after})
	if len(b.items) >= b.limit {
		b.Flush()
	}
}

func (b *localMediaWriteBatch) Flush() {
	if b == nil || len(b.items) == 0 || b.scanner == nil || b.scanner.repo == nil || b.scanner.repo.DB == nil {
		return
	}
	items := b.items
	b.items = nil
	media := make([]model.Media, 0, len(items))
	for _, item := range items {
		if item.media != nil {
			media = append(media, *item.media)
		}
	}
	if len(media) == 0 {
		return
	}
	if err := b.scanner.repo.DB.WithContext(b.ctx).CreateInBatches(&media, b.limit).Error; err == nil {
		b.res.Added += len(media)
		for _, item := range items {
			if item.after != nil {
				item.after()
			}
		}
		b.publish()
		return
	}
	for _, item := range items {
		if item.media == nil {
			continue
		}
		if err := b.scanner.repo.Media.Upsert(b.ctx, item.media); err != nil {
			addScanError(b.res, item.path, err)
			b.scanner.log.Warn("upsert media failed", zap.String("path", item.path), zap.Error(err))
			continue
		}
		b.res.Added++
		if item.after != nil {
			item.after()
		}
	}
	b.publish()
}

func (b *localMediaWriteBatch) publish() {
	if b == nil || b.scanner == nil || b.scanner.hub == nil || b.res == nil {
		return
	}
	b.scanner.hub.Publish("scan", map[string]any{
		"library_id": b.res.LibraryID,
		"visited":    b.res.Visited,
		"added":      b.res.Added,
		"updated":    b.res.Updated,
		"probed":     b.res.Probed,
		"local_meta": b.res.LocalMetadata,
		"batched":    true,
	})
}
