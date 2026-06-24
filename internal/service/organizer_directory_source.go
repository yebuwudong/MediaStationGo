package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type organizeDirectoryLayout struct {
	MediaType string
	Category  string
}

type organizeSourceFileRequest struct {
	Source                string
	SourceRoot            string
	DestRoot              string
	Mode                  TransferMode
	MediaTypeOverride     string
	MediaCategoryOverride string
	DryRun                bool
	AllowReplaceExisting  bool
	MetadataCache         map[string]*Match
	Result                *OrganizeResult
}

// organizeSourceFile organizes a single video file from the source directory
// into destRoot, applying dedup + 洗版.
func (o *OrganizerService) organizeSourceFile(ctx context.Context, req organizeSourceFileRequest) error {
	src := req.Source
	ext := filepath.Ext(src)
	season, episode := ParseEpisode(src)
	title, year := CleanQuery(src)
	if organizeWeakFileTitle(title) {
		if folderTitle, folderYear := organizeTitleFromParentFolder(src, req.SourceRoot, season > 0 || episode > 0); folderTitle != "" {
			title = folderTitle
			if year <= 0 {
				year = folderYear
			}
		} else {
			title = strings.TrimSuffix(filepath.Base(src), ext)
		}
	}
	parsedTitle := title
	sourceMedia := o.lookupOrganizeSourceMedia(ctx, src)
	title = sanitizeFilename(titleCaseWords(title))
	if sourceMedia != nil {
		if mediaTitle := sanitizeFilename(strings.TrimSpace(sourceMedia.Title)); mediaTitle != "" {
			title = mediaTitle
			parsedTitle = strings.TrimSpace(sourceMedia.Title)
		}
		if sourceMedia.Year > 0 {
			year = sourceMedia.Year
		}
		if sourceMedia.SeasonNum > 0 {
			season = sourceMedia.SeasonNum
		}
		if sourceMedia.EpisodeNum > 0 {
			episode = sourceMedia.EpisodeNum
		}
	}
	if title == "" {
		title = "Unknown"
	}
	pathLayout := o.inferOrganizeDirectoryLayout(src, req.SourceRoot)
	layout := pathLayout
	forcedType := normalizeOrganizeMediaType(req.MediaTypeOverride)
	inferredType := o.inferMediaTypeForSourceFile(src, title, season, episode)
	if forcedType != "" {
		if layout.Category != "" && layout.MediaType != "" && layout.MediaType != forcedType {
			layout.Category = ""
		}
		layout.MediaType = forcedType
	} else if inferredType != "" {
		if inferredType == "tv" && layout.MediaType == "movie" {
			layout = organizeDirectoryLayout{MediaType: inferredType}
		} else if layout.MediaType == "" {
			layout.MediaType = inferredType
		}
	}
	var metadataMatch *Match
	if match := o.lookupOrganizeMetadata(ctx, src, req.SourceRoot, layout.MediaType, title, year, season, episode, req.MetadataCache); match != nil {
		metadataMatch = match
		applyOrganizeMetadataMatch(metadataMatch, &title, &parsedTitle, &year)
	} else if sourceMedia != nil {
		metadataMatch = organizeMatchFromMedia(sourceMedia)
		applyOrganizeMetadataMatch(metadataMatch, &title, &parsedTitle, &year)
	}
	if category := strings.TrimSpace(req.MediaCategoryOverride); category != "" {
		layout.Category = sanitizeFilename(category)
	} else if category := o.smartClassifySourceFile(ctx, src, req.SourceRoot, layout.MediaType, title, parsedTitle, metadataMatch); category != "" {
		layout.Category = category
	}
	if forcedType == "" {
		if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(layout.Category); impliedType != "" {
			layout.Category = normalizedCategory
			if layout.MediaType == "" || layout.MediaType == "tv" || layout.MediaType == "anime" || pathLayout.Category != layout.Category {
				layout.MediaType = impliedType
			}
		}
	}
	targetLibrary, matchedLibrary := o.organizeLibraryForLayout(ctx, req.DestRoot, layout.MediaType, layout.Category)
	layoutRoot := targetLibrary.Path
	targetLibraryID := targetLibrary.ID
	if !matchedLibrary && layout.MediaType != "" {
		layoutRoot = o.organizeRoot(req.DestRoot, layout.MediaType, layout.Category)
	}
	if !matchedLibrary && layout.Category != "" {
		layoutRoot = categoryRoot(layoutRoot, sanitizeFilename(layout.Category))
	}
	if !matchedLibrary && !req.DryRun {
		o.ensureOrganizeLibraryForRoot(ctx, layoutRoot, layout.MediaType, layout.Category)
	}

	isSeries := season > 0 || episode > 0
	if layout.MediaType != "" {
		isSeries = isSeriesLibraryType(layout.MediaType) && (season > 0 || episode > 0)
	}
	target, err := o.buildOrganizeTargetPath(ctx, organizeTargetInput{
		Root:      layoutRoot,
		MediaType: layout.MediaType,
		Category:  layout.Category,
		Title:     title,
		Source:    src,
		Ext:       ext,
		Year:      year,
		Season:    season,
		Episode:   episode,
		Series:    isSeries,
	})
	if err != nil {
		return err
	}
	destDir, dst, episodeTag := target.Dir, target.Path, target.EpisodeTag
	if filepath.Clean(src) == filepath.Clean(dst) {
		req.Result.Skipped++
		req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
			Source: src, Target: dst, Action: "skip", Reason: organizeSkipAlreadyOrganized,
			MediaType: layout.MediaType, Category: layout.Category, Title: title,
		})
		return nil
	}

	externalExisting := o.existingByExternalIdentity(ctx, req.DestRoot, metadataMatch, season, episode)
	identityExisting := o.existingByIdentity(ctx, req.DestRoot, parsedTitle, year, season, episode)
	folderExisting := o.existingByFolder(destDir, episodeTag)
	existing := mergeExistingVersionPaths(externalExisting, identityExisting, folderExisting)
	if len(existing) > 0 {
		reclassified, err := o.reclassifyExistingMedia(ctx, organizeExistingReclassifyRequest{
			Source:          src,
			Target:          dst,
			DestRoot:        req.DestRoot,
			TargetLibraryID: targetLibraryID,
			Existing:        existing,
			DryRun:          req.DryRun,
			MediaType:       layout.MediaType,
			Category:        layout.Category,
			Title:           title,
			Year:            year,
			Season:          season,
			Episode:         episode,
			Result:          req.Result,
		})
		if err != nil {
			return err
		}
		if reclassified {
			return nil
		}
		srcArea := o.resolutionArea(ctx, src)
		bestArea := 0
		for _, e := range existing {
			if a := o.resolutionArea(ctx, e); a > bestArea {
				bestArea = a
			}
		}
		if req.AllowReplaceExisting && srcArea > 0 && bestArea > 0 && srcArea > bestArea {
			req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
				Source: src, Target: dst, Action: "replace", Reason: "higher resolution",
				MediaType: layout.MediaType, Category: layout.Category, Title: title,
			})
			if req.DryRun {
				req.Result.Replaced++
				return nil
			}
			if err := o.replaceVersions(ctx, src, existing, dst, req.Mode); err != nil {
				return err
			}
			o.log.Info("organize replaced lower-resolution media",
				zap.String("from", src),
				zap.String("to", dst),
				zap.Int("src_area", srcArea),
				zap.Int("existing_area", bestArea),
			)
			req.Result.Replaced++
			return nil
		}
		reason := organizeSkipTargetExists
		if len(externalExisting) > 0 || len(identityExisting) > 0 || o.allExistingPathsInDB(ctx, existing) {
			reason = organizeSkipDuplicateLibrary
		}
		o.log.Debug("organize skip duplicate",
			zap.String("src", src), zap.String("dest_dir", destDir), zap.String("reason", reason))
		req.Result.Skipped++
		req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
			Source: src, Target: dst, Action: "skip", Reason: reason,
			MediaType: layout.MediaType, Category: layout.Category, Title: title,
		})
		return nil
	}

	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: src, Target: dst, Action: "organize",
		MediaType: layout.MediaType, Category: layout.Category, Title: title,
	})
	if req.DryRun {
		req.Result.Organized++
		return nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		req.Result.Skipped++
		if len(req.Result.Items) > 0 {
			req.Result.Items[len(req.Result.Items)-1].Action = "skip"
			req.Result.Items[len(req.Result.Items)-1].Reason = organizeSkipTargetExists
		}
		return nil
	}
	if err := transferFile(src, dst, req.Mode); err != nil {
		return err
	}
	if err := transferSidecarNFO(src, dst, req.Mode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("from", src), zap.String("to", dst), zap.Error(err))
	}
	req.Result.Organized++
	return nil
}

func organizeTitleFromParentFolder(src, sourceRoot string, seriesLike bool) (string, int) {
	if !seriesLike {
		return "", 0
	}
	raw := seriesFolderTitle(src, sourceRoot)
	if strings.TrimSpace(raw) == "" {
		return "", 0
	}
	title, year := CleanQuery(raw)
	if title == "" {
		title = strings.TrimSpace(raw)
	}
	return title, year
}

func shouldSkipOrganizeSourceVideo(path, sourceRoot string) (bool, string) {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(sourceRoot)
	if rel, err := filepath.Rel(cleanRoot, cleanPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		dir := filepath.Dir(rel)
		if dir != "." {
			for _, part := range strings.Split(dir, string(os.PathSeparator)) {
				switch normalizeOrganizeCategoryKey(part) {
				case "sample", "samples", "trailer", "trailers", "preview", "previews", "teaser", "teasers":
					return true, organizeSkipSampleClip
				}
			}
		}
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath)))
	normalized := strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(base)
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return false, ""
	}
	if len(fields) == 1 && strings.HasPrefix(fields[0], "sample") {
		return true, organizeSkipSampleClip
	}
	last := fields[len(fields)-1]
	switch last {
	case "sample", "trailer", "preview", "teaser":
		return true, organizeSkipSampleClip
	}
	return false, ""
}

func normalizeOrganizeMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie", "film":
		return "movie"
	case "tv", "series", "show", "drama":
		return "tv"
	case "anime", "animation":
		return "anime"
	case "variety":
		return "variety"
	case "adult", "nsfw":
		return "adult"
	default:
		return ""
	}
}

func organizeWeakFileTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	fields := strings.Fields(strings.ToLower(title))
	if len(fields) == 0 {
		return true
	}
	meaningful := 0
	for _, field := range fields {
		if _, ok := noiseTokenSet[field]; ok {
			continue
		}
		if _, ok := releaseBoundaryTokenSet[field]; ok {
			continue
		}
		if len(field) == 4 && strings.HasPrefix(field, "20") {
			continue
		}
		meaningful++
	}
	return meaningful == 0
}

func (o *OrganizerService) inferMediaTypeForSourceFile(src, title string, season, episode int) string {
	if season > 0 || episode > 0 {
		return "tv"
	}
	return normalizeMediaType("", title, src)
}
