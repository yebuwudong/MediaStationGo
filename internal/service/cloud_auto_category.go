package service

import (
	"context"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const cloudAutoCategoryQueryKey = "auto_category"

func BuildCloudAutoCategoryLibraryPath(provider, displayDir string) string {
	base := BuildCloudLibraryPath(provider, "", displayDir)
	if base == "" || strings.TrimSpace(displayDir) == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + cloudAutoCategoryQueryKey + "=1"
}

func CloudLibraryAutoCategory(lib model.Library) bool {
	u, err := url.Parse(strings.TrimSpace(lib.Path))
	if err != nil || strings.ToLower(u.Scheme) != "cloud" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(u.Query().Get(cloudAutoCategoryQueryKey))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func cloudRootMountNeedsAutoCategory(mount CloudMountInfo) bool {
	return strings.TrimSpace(mount.DisplayDir) == "" && strings.TrimSpace(mount.ScanDir) == ""
}

func cloudAutoCategoryDisplayDirForMediaPath(path string) string {
	info, ok := ParseCloudLibraryMount(path)
	if !ok {
		return ""
	}
	parts := strmSlashParts(info.DisplayDir)
	if len(parts) <= 1 {
		return ""
	}
	parts = parts[:len(parts)-1]
	categoryParts := cloudAutoCategoryParts(parts)
	if len(categoryParts) == 0 {
		return ""
	}
	return strings.Join(categoryParts, "/")
}

func cloudAutoCategoryParts(parts []string) []string {
	for i, part := range parts {
		root := strmCanonicalRoot(part)
		if root != "" {
			if i+1 >= len(parts) {
				return nil
			}
			category := strings.TrimSpace(parts[i+1])
			if cloudAutoCategoryRootMatches(root, category) {
				return []string{root, category}
			}
			return nil
		}
		if root := strmCategoryRoot(part); root != "" {
			return []string{root, strings.TrimSpace(part)}
		}
	}
	return nil
}

func cloudAutoCategoryRootMatches(root, category string) bool {
	category = strings.TrimSpace(category)
	if category == "" {
		return false
	}
	if strmCategoryRoot(category) == root {
		return true
	}
	if root == "电影" {
		return containsAnyText(strings.ToLower(category), "纪录片", "纪录", "documentary")
	}
	return false
}

func (s *ScannerService) ensureCloudAutoCategoryLibrary(ctx context.Context, rootLib *model.Library, provider, displayDir string) (*model.Library, error) {
	displayDir = normalizeCloudMountDir(provider, displayDir)
	if s == nil || s.repo == nil || s.repo.DB == nil || rootLib == nil || provider == "" || displayDir == "" {
		return rootLib, nil
	}
	if existing := s.findCloudLibraryByDisplayDir(ctx, provider, displayDir); existing != nil {
		return existing, nil
	}
	path := BuildCloudAutoCategoryLibraryPath(provider, displayDir)
	if path == "" {
		return rootLib, nil
	}
	name := cloudMountDirBase(displayDir)
	if name == "" {
		name = displayDir
	}
	lib := &model.Library{
		Name:    name,
		Path:    path,
		Type:    InferCloudMountMediaType(displayDir, name),
		Enabled: true,
	}
	if err := s.repo.Library.Create(ctx, lib); err != nil {
		if existing := s.findCloudLibraryByDisplayDir(ctx, provider, displayDir); existing != nil {
			return existing, nil
		}
		return nil, err
	}
	if s.log != nil {
		s.log.Info("created cloud auto category library",
			zap.String("root_library_id", rootLib.ID),
			zap.String("library_id", lib.ID),
			zap.String("provider", provider),
			zap.String("display_dir", displayDir))
	}
	return lib, nil
}

func (s *ScannerService) findCloudLibraryByDisplayDir(ctx context.Context, provider, displayDir string) *model.Library {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("list libraries for cloud auto category failed", zap.Error(err))
		}
		return nil
	}
	displayDir = normalizeCloudMountDir(provider, displayDir)
	for _, lib := range libs {
		info, ok := ParseCloudLibraryMount(lib.Path)
		if !ok || info.Provider != provider || normalizeCloudMountDir(provider, info.DisplayDir) != displayDir {
			continue
		}
		return &lib
	}
	return nil
}

func (s *ScannerService) cloudScanLibraryScopeIDs(ctx context.Context, lib *model.Library, mount CloudMountInfo) []string {
	if lib == nil {
		return nil
	}
	ids := []string{lib.ID}
	if !cloudRootMountNeedsAutoCategory(mount) || s == nil || s.repo == nil || s.repo.Library == nil {
		return ids
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("list libraries for cloud scan scope failed", zap.String("library_id", lib.ID), zap.Error(err))
		}
		return ids
	}
	for _, candidate := range libs {
		if candidate.ID == lib.ID || !CloudLibraryAutoCategory(candidate) {
			continue
		}
		info, ok := ParseCloudLibraryMount(candidate.Path)
		if ok && info.Provider == mount.Provider {
			ids = appendUniqueLibraryIDs(ids, candidate.ID)
		}
	}
	return ids
}
