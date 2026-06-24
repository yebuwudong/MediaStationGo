package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) scanCloudLibrary(ctx context.Context, lib *model.Library, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	res := &ScanResult{LibraryID: lib.ID}
	if s.storage == nil {
		return res, fmt.Errorf("cloud storage service unavailable")
	}

	cfg, err := s.repo.StorageConfig.Get(ctx, mount.Provider)
	if err != nil || cfg == nil {
		return res, fmt.Errorf("storage config not found: %s", mount.Provider)
	}
	if !cfg.Enabled {
		return res, fmt.Errorf("storage %s is disabled", mount.Provider)
	}
	typ := mount.Provider
	rootDir := mount.ScanDir
	rootDisplayDir := mount.DisplayDir
	autoCategoryRoot := cloudRootMountNeedsAutoCategory(mount)
	scopeIDs := s.cloudScanLibraryScopeIDs(ctx, lib, mount)
	seen := make(map[string]struct{})
	progress := newCloudScanProgressState()
	progress.publish(s, lib.ID, res, "listing", true)
	candidates, err := s.collectCloudScanCandidates(ctx, lib, cloudScanCandidateRequest{
		provider:         typ,
		rootDir:          rootDir,
		rootDisplayDir:   rootDisplayDir,
		autoCategoryRoot: autoCategoryRoot,
		progress:         progress,
		result:           res,
	})
	if err != nil {
		return res, err
	}
	existingMedia, err := s.existingCloudMediaSnapshotForLibraries(ctx, scopeIDs)
	if err != nil {
		s.log.Warn("load existing cloud media snapshot failed", zap.String("library_id", lib.ID), zap.Error(err))
		existingMedia = nil
	}
	sortCloudCandidatesByRefreshPriority(candidates, existingMedia)
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)
	probeBudget := maxCloudMediaProbeQueuePerScan
	targetLibs := map[string]*model.Library{"": lib}
	touchedLibraryIDs := []string{}
	for _, candidate := range candidates {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		targetLib := lib
		if candidate.categoryDisplayDir != "" {
			if cached, ok := targetLibs[candidate.categoryDisplayDir]; ok {
				targetLib = cached
			} else if categoryLib, err := s.ensureCloudAutoCategoryLibrary(ctx, lib, typ, candidate.categoryDisplayDir); err == nil && categoryLib != nil {
				targetLib = categoryLib
				targetLibs[candidate.categoryDisplayDir] = categoryLib
				scopeIDs = appendUniqueLibraryIDs(scopeIDs, categoryLib.ID)
			} else if err != nil {
				s.log.Warn("ensure cloud auto category library failed",
					zap.String("library_id", lib.ID),
					zap.String("provider", typ),
					zap.String("category", candidate.categoryDisplayDir),
					zap.Error(err))
			}
		}
		touchedLibraryIDs = appendUniqueLibraryIDs(touchedLibraryIDs, targetLib.ID)
		seen[candidate.path] = struct{}{}
		s.ingestCloudFile(ctx, targetLib, typ, candidate.ref, candidate.path, candidate.name, candidate.size, candidate.localMeta, existingMedia, writeBatch, &probeBudget, res)
		progress.publish(s, lib.ID, res, "importing", res.Visited == 1 || res.Visited%100 == 0)
	}
	writeBatch.Flush()
	removed, err := s.pruneMissingCloudMediaForLibraries(ctx, scopeIDs, seen)
	if err != nil {
		s.log.Warn("prune missing cloud media failed", zap.String("library_id", lib.ID), zap.Error(err))
	} else {
		res.Removed = removed
	}
	publishCloudScanFinished(s, lib.ID, res, progress)
	s.invalidateMediaCache(ctx)
	for _, targetID := range appendUniqueLibraryIDs(touchedLibraryIDs, lib.ID) {
		s.maybeGenerateSTRMAfterScan(targetID)
	}
	if scanHasImportChanges(res) && autoScrape && s.scraper != nil && s.scraper.AnyEnabled() && s.autoScrapeEnabled(ctx) {
		for _, targetID := range appendUniqueLibraryIDs(touchedLibraryIDs, lib.ID) {
			s.startAutoScrape(ctx, targetID)
		}
	}
	return res, nil
}

func scanHasImportChanges(res *ScanResult) bool {
	return res != nil && (res.Added > 0 || res.Updated > 0 || res.Removed > 0)
}
