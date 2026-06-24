package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// MediaCategoryReclassifyOptions controls the metadata-based category audit.
// Empty LibraryIDs means all enabled local libraries.
type MediaCategoryReclassifyOptions struct {
	LibraryIDs []string
	DryRun     bool
}

// ReclassifyMisclassifiedMedia corrects already-scanned local media whose
// stored metadata clearly disagrees with the current library/category.
func (o *OrganizerService) ReclassifyMisclassifiedMedia(ctx context.Context, opts MediaCategoryReclassifyOptions) (*OrganizeResult, error) {
	res := &OrganizeResult{DryRun: opts.DryRun}
	if o == nil || o.repo == nil || o.repo.DB == nil || o.repo.Library == nil {
		return res, nil
	}
	libraries, err := o.repo.Library.List(ctx)
	if err != nil {
		return res, err
	}
	filterIDs := compactLibraryIDs(opts.LibraryIDs...)
	filter := map[string]struct{}{}
	for _, id := range filterIDs {
		filter[id] = struct{}{}
	}
	libByID := make(map[string]model.Library, len(libraries))
	for _, lib := range libraries {
		if !lib.Enabled || strings.TrimSpace(lib.ID) == "" {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[lib.ID]; !ok {
				continue
			}
		}
		libByID[lib.ID] = lib
	}
	if len(libByID) == 0 {
		return res, nil
	}

	query := o.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	if len(filter) > 0 {
		query = query.Where("library_id IN ?", filterIDs)
	}
	var rows []model.Media
	err = query.FindInBatches(&rows, 500, func(_ *gorm.DB, _ int) error {
		for i := range rows {
			lib, ok := libByID[rows[i].LibraryID]
			if !ok {
				continue
			}
			changed, err := o.reclassifyScannedMedia(ctx, rows[i], lib, opts.DryRun, res)
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", rows[i].Title, err.Error()))
				if o.log != nil {
					o.log.Warn("metadata category reclassify failed",
						zap.String("media", rows[i].ID),
						zap.String("path", rows[i].Path),
						zap.Error(err))
				}
				continue
			}
			if changed && o.log != nil {
				o.log.Debug("metadata category reclassify applied",
					zap.String("media", rows[i].ID),
					zap.String("title", rows[i].Title))
			}
		}
		return nil
	}).Error
	return res, err
}

func (o *OrganizerService) reclassifyScannedMedia(ctx context.Context, media model.Media, lib model.Library, dryRun bool, res *OrganizeResult) (bool, error) {
	if res == nil || !lib.Enabled || strings.TrimSpace(media.Path) == "" {
		return false, nil
	}
	if _, ok := ParseCloudLibraryMount(lib.Path); ok {
		return false, nil
	}
	if !organizeFileExists(media.Path) || !mediaHasReliableCategoryMetadata(media) {
		return false, nil
	}

	mediaType := normalizeOrganizeMediaType(lib.Type)
	category := o.classifyMedia(ctx, &media, mediaType)
	if category == "" {
		return false, nil
	}
	if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(category); impliedType != "" {
		mediaType = impliedType
		category = normalizedCategory
	}
	if mediaType == "" {
		mediaType = normalizeOrganizeMediaType(lib.Type)
	}
	if mediaType == "" {
		return false, nil
	}

	baseRoot := redirectOrganizeStagingRoot(o.resolveBaseRoot(ctx, &lib, ""))
	targetLibrary, matched := o.organizeLibraryForLayout(ctx, baseRoot, mediaType, category)
	if !matched || strings.TrimSpace(targetLibrary.ID) == "" || strings.TrimSpace(targetLibrary.Path) == "" {
		return false, nil
	}
	if strings.EqualFold(targetLibrary.ID, lib.ID) && pathWithin(media.Path, targetLibrary.Path) {
		return false, nil
	}
	if pathWithin(media.Path, targetLibrary.Path) {
		return o.reclassifyScannedMediaLibraryOnly(ctx, media, lib, targetLibrary, category, mediaType, dryRun, res)
	}

	title := sanitizeFilename(strings.TrimSpace(media.Title))
	if title == "" {
		title = "Unknown"
	}
	target, err := o.buildOrganizeTargetPath(ctx, organizeTargetInput{
		Root:      targetLibrary.Path,
		MediaType: mediaType,
		Category:  category,
		Title:     title,
		Source:    media.Path,
		Ext:       filepath.Ext(media.Path),
		Year:      media.Year,
		Season:    media.SeasonNum,
		Episode:   media.EpisodeNum,
		Series:    isSeriesLibraryType(mediaType),
	})
	if err != nil {
		return false, err
	}
	return o.reclassifyExistingMedia(ctx, organizeExistingReclassifyRequest{
		Source:          media.Path,
		Target:          target.Path,
		DestRoot:        firstNonEmpty(baseRoot, lib.Path),
		TargetLibraryID: targetLibrary.ID,
		Existing:        []string{media.Path},
		DryRun:          dryRun,
		MediaType:       mediaType,
		Category:        category,
		Title:           title,
		Year:            media.Year,
		Season:          media.SeasonNum,
		Episode:         media.EpisodeNum,
		Result:          res,
	})
}

func (o *OrganizerService) reclassifyScannedMediaLibraryOnly(ctx context.Context, media model.Media, oldLib, targetLib model.Library, category, mediaType string, dryRun bool, res *OrganizeResult) (bool, error) {
	res.Items = append(res.Items, OrganizePreviewItem{
		Source:    media.Path,
		Target:    media.Path,
		Action:    "reclassify",
		Reason:    "metadata category library changed",
		MediaType: mediaType,
		Category:  category,
		Title:     media.Title,
	})
	if dryRun {
		res.Reclassified++
		return true, nil
	}
	if err := o.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Where("id = ?", media.ID).
		Update("library_id", targetLib.ID).Error; err != nil {
		return false, err
	}
	if o.log != nil {
		o.log.Info("media library reclassified by metadata",
			zap.String("media", media.ID),
			zap.String("path", media.Path),
			zap.String("from_library", oldLib.ID),
			zap.String("to_library", targetLib.ID),
			zap.String("category", category),
			zap.String("media_type", mediaType))
	}
	res.Reclassified++
	return true, nil
}

func mediaHasReliableCategoryMetadata(media model.Media) bool {
	return media.NSFW ||
		strings.TrimSpace(media.Languages) != "" ||
		strings.TrimSpace(media.Countries) != "" ||
		strings.TrimSpace(media.Genres) != ""
}
