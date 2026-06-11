package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// CloudMountInfo is the canonical identity of a mounted cloud library. ScanDir
// is the provider id/path used for listing. DisplayDir is a hierarchical path
// used to prevent mounting both a parent and its child as separate libraries.
type CloudMountInfo struct {
	Provider   string
	DisplayDir string
	ScanDir    string
	Path       string
}

type CloudMountConflict struct {
	Library            model.Library `json:"library"`
	Exact              bool          `json:"exact"`
	Nested             bool          `json:"nested"`
	ExistingIsAncestor bool          `json:"existing_is_ancestor"`
}

func BuildCloudLibraryPath(provider, scanDir, displayDir string) string {
	provider = strings.TrimSpace(provider)
	scanDir = normalizeCloudMountDir(provider, scanDir)
	displayDir = normalizeCloudMountDir(provider, firstNonEmpty(displayDir, scanDir))
	if provider == "" {
		return ""
	}
	base := "cloud://" + provider
	if displayDir == "" {
		if scanDir != "" {
			return base + "?dir=" + url.QueryEscape(scanDir)
		}
		return base
	}
	path := base + "/" + url.PathEscape(displayDir)
	if scanDir != "" && scanDir != displayDir {
		path += "?dir=" + url.QueryEscape(scanDir)
	}
	return path
}

func ParseCloudLibraryMount(raw string) (CloudMountInfo, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "cloud://") {
		return CloudMountInfo{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || strings.ToLower(u.Scheme) != "cloud" {
		return CloudMountInfo{}, false
	}
	provider := strings.TrimSpace(u.Host)
	if provider == "" {
		return CloudMountInfo{}, false
	}
	displayDir := strings.Trim(strings.TrimSpace(u.Path), "/")
	if decoded, err := url.PathUnescape(displayDir); err == nil {
		displayDir = decoded
	}
	scanDir := displayDir
	if qDir := strings.TrimSpace(u.Query().Get("dir")); qDir != "" {
		if decoded, err := url.QueryUnescape(qDir); err == nil {
			qDir = decoded
		}
		scanDir = qDir
	}
	displayDir = normalizeCloudMountDir(provider, displayDir)
	scanDir = normalizeCloudMountDir(provider, scanDir)
	return CloudMountInfo{
		Provider:   provider,
		DisplayDir: displayDir,
		ScanDir:    scanDir,
		Path:       raw,
	}, true
}

func FindCloudMountConflict(libs []model.Library, provider, scanDir, displayDir string) *CloudMountConflict {
	candidate := CloudMountInfo{
		Provider:   strings.TrimSpace(provider),
		DisplayDir: normalizeCloudMountDir(provider, firstNonEmpty(displayDir, scanDir)),
		ScanDir:    normalizeCloudMountDir(provider, scanDir),
	}
	for _, lib := range libs {
		existing, ok := ParseCloudLibraryMount(lib.Path)
		if !ok || existing.Provider != candidate.Provider {
			continue
		}
		if existing.DisplayDir == candidate.DisplayDir {
			return &CloudMountConflict{Library: lib, Exact: true}
		}
		if existing.ScanDir != "" && candidate.ScanDir != "" && existing.ScanDir == candidate.ScanDir {
			return &CloudMountConflict{Library: lib, Exact: true}
		}
		if cloudMountAncestor(candidate.DisplayDir, existing.DisplayDir) {
			return &CloudMountConflict{Library: lib, Nested: true}
		}
	}
	return nil
}

func CloudLibraryShadowed(libs []model.Library, lib model.Library) *CloudMountConflict {
	current, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return nil
	}
	for _, existing := range libs {
		if existing.ID == lib.ID || !existing.Enabled {
			continue
		}
		info, ok := ParseCloudLibraryMount(existing.Path)
		if !ok || info.Provider != current.Provider {
			continue
		}
		if info.DisplayDir == current.DisplayDir && existing.CreatedAt.Before(lib.CreatedAt) {
			return &CloudMountConflict{Library: existing, Exact: true}
		}
		if cloudMountAncestor(current.DisplayDir, info.DisplayDir) {
			return &CloudMountConflict{Library: existing, Nested: true}
		}
	}
	return nil
}

func FilterShadowedCloudLibraries(libs []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if CloudLibraryShadowed(libs, lib) == nil {
			out = append(out, lib)
		}
	}
	return out
}

func FilterDisplayCloudLibraries(ctx context.Context, repo *repository.Container, libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	counts := cloudLibraryMediaCounts(ctx, repo, libs)
	collapsed := make([]model.Library, 0, len(libs))
	byKey := make(map[string]int, len(libs))
	for _, lib := range libs {
		key, ok := cloudLibraryDisplayKey(lib)
		if !ok {
			collapsed = append(collapsed, lib)
			continue
		}
		if prevIndex, exists := byKey[key]; exists {
			if betterDisplayCloudLibrary(lib, collapsed[prevIndex], counts) {
				collapsed[prevIndex] = lib
			}
			continue
		}
		byKey[key] = len(collapsed)
		collapsed = append(collapsed, lib)
	}
	collapsed = FilterShadowedCloudLibraries(collapsed)
	return mergeDisplayCloudLibraries(collapsed)
}

func FilterScannableCloudLibraries(ctx context.Context, repo *repository.Container, libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	counts := cloudLibraryMediaCounts(ctx, repo, libs)
	collapsed := make([]model.Library, 0, len(libs))
	byKey := make(map[string]int, len(libs))
	for _, lib := range libs {
		key, ok := cloudLibraryDisplayKey(lib)
		if !ok {
			collapsed = append(collapsed, lib)
			continue
		}
		if prevIndex, exists := byKey[key]; exists {
			if betterDisplayCloudLibrary(lib, collapsed[prevIndex], counts) {
				collapsed[prevIndex] = lib
			}
			continue
		}
		byKey[key] = len(collapsed)
		collapsed = append(collapsed, lib)
	}
	return FilterShadowedCloudLibraries(collapsed)
}

func NormalizeCloudLibraryDisplayNames(libs []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if displayName, ok := CloudLibraryDisplayName(lib); ok && displayName != "" {
			lib.Name = displayName
		}
		out = append(out, lib)
	}
	return out
}

func cloudLibraryMediaCounts(ctx context.Context, repo *repository.Container, libs []model.Library) map[string]int64 {
	counts := make(map[string]int64, len(libs))
	if repo == nil || repo.DB == nil || len(libs) == 0 {
		return counts
	}
	ids := make([]string, 0, len(libs))
	for _, lib := range libs {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			ids = append(ids, lib.ID)
		}
	}
	if len(ids) == 0 {
		return counts
	}
	var rows []struct {
		LibraryID string
		Count     int64
	}
	if err := repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("library_id, COUNT(*) AS count").
		Where("library_id IN ? AND deleted_at IS NULL", ids).
		Group("library_id").
		Scan(&rows).Error; err != nil {
		return counts
	}
	for _, row := range rows {
		counts[row.LibraryID] = row.Count
	}
	return counts
}

func cloudLibraryDisplayKey(lib model.Library) (string, bool) {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return "", false
	}
	dir := firstNonEmpty(info.DisplayDir, info.ScanDir)
	return info.Provider + "\x00" + dir, true
}

func betterDisplayCloudLibrary(candidate, current model.Library, counts map[string]int64) bool {
	candidateCount := counts[candidate.ID]
	currentCount := counts[current.ID]
	if (candidateCount > 0) != (currentCount > 0) {
		return candidateCount > 0
	}
	if candidate.Enabled != current.Enabled {
		return candidate.Enabled
	}
	candidateCanonical := cloudLibraryPathIsCanonical(candidate)
	currentCanonical := cloudLibraryPathIsCanonical(current)
	if candidateCanonical != currentCanonical {
		return candidateCanonical
	}
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func cloudLibraryPathIsCanonical(lib model.Library) bool {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return false
	}
	return BuildCloudLibraryPath(info.Provider, info.ScanDir, info.DisplayDir) == strings.TrimSpace(lib.Path)
}

func mergeDisplayCloudLibraries(libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	localByKey := make(map[string]struct{}, len(libs))
	for _, lib := range libs {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok || !lib.Enabled {
			continue
		}
		if key, ok := CloudLibraryMergeKey(lib); ok {
			localByKey[key] = struct{}{}
		}
	}
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if displayName, ok := CloudLibraryDisplayName(lib); ok && displayName != "" {
			lib.Name = displayName
			if key, ok := CloudLibraryMergeKey(lib); ok {
				if _, exists := localByKey[key]; exists {
					continue
				}
			}
		}
		out = append(out, lib)
	}
	return out
}

func CloudLibraryDisplayName(lib model.Library) (string, bool) {
	info, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return "", false
	}
	name := stripCloudProviderDisplayPrefix(strings.TrimSpace(lib.Name), info.Provider)
	dir := firstNonEmpty(info.DisplayDir, info.ScanDir)
	if name == "" || strings.EqualFold(name, CloudMountProviderLabel(info.Provider)) {
		if base := cloudMountDirBase(dir); base != "" {
			name = base
		}
	}
	if name == "" {
		name = CloudMountProviderLabel(info.Provider)
	}
	return name, true
}

func CloudLibraryMergeKey(lib model.Library) (string, bool) {
	name := strings.TrimSpace(lib.Name)
	if displayName, ok := CloudLibraryDisplayName(lib); ok {
		name = displayName
	}
	name = normalizeLibraryMergeName(name)
	if name == "" {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(lib.Type)) + "\x00" + name, true
}

func MergedLibraryIDsForLibrary(ctx context.Context, repo *repository.Container, libraryID string) ([]string, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" || repo == nil || repo.Library == nil {
		return []string{libraryID}, nil
	}
	lib, err := repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return []string{libraryID}, nil
	}
	libs, err := repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	return MergedLibraryIDs(libs, *lib), nil
}

func MergedLibraryIDs(libs []model.Library, lib model.Library) []string {
	ids := []string{}
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	add(lib.ID)
	key, ok := CloudLibraryMergeKey(lib)
	if !ok {
		return ids
	}
	_, libIsCloud := ParseCloudLibraryMount(lib.Path)
	for _, candidate := range libs {
		if candidate.ID == lib.ID || !candidate.Enabled {
			continue
		}
		candidateKey, ok := CloudLibraryMergeKey(candidate)
		if !ok || candidateKey != key {
			continue
		}
		_, candidateIsCloud := ParseCloudLibraryMount(candidate.Path)
		if !libIsCloud && !candidateIsCloud {
			continue
		}
		add(candidate.ID)
	}
	return ids
}

func ExpandMediaVisibilityForMergedCloudLibraries(ctx context.Context, repo *repository.Container, visibility MediaVisibility) MediaVisibility {
	if repo == nil || repo.Library == nil {
		return visibility
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		visibility.AllowedLibraryIDs = expandMergedLibraryIDs(ctx, repo, visibility.AllowedLibraryIDs)
	}
	if len(visibility.HiddenLibraryIDs) > 0 {
		visibility.HiddenLibraryIDs = expandMergedLibraryIDs(ctx, repo, visibility.HiddenLibraryIDs)
	}
	return visibility
}

func expandMergedLibraryIDs(ctx context.Context, repo *repository.Container, ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	libs, err := repo.Library.List(ctx)
	if err != nil {
		return ids
	}
	byID := make(map[string]model.Library, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = lib
	}
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range ids {
		lib, ok := byID[id]
		if !ok {
			add(id)
			continue
		}
		for _, mergedID := range MergedLibraryIDs(libs, lib) {
			add(mergedID)
		}
	}
	return out
}

func CloudMountProviderLabel(provider string) string {
	switch strings.TrimSpace(provider) {
	case cloud.TypeQuark:
		return "夸克网盘"
	case cloud.Type115:
		return "115 网盘"
	case cloud.TypeCloudDrive2:
		return "CloudDrive2"
	case cloud.TypeOpenList:
		return "OpenList"
	default:
		if strings.TrimSpace(provider) == "" {
			return "网盘"
		}
		return strings.TrimSpace(provider)
	}
}

func stripCloudProviderDisplayPrefix(name, provider string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, label := range []string{CloudMountProviderLabel(provider), strings.TrimSpace(provider)} {
		label = strings.TrimSpace(label)
		if label == "" || len(name) < len(label) || !strings.EqualFold(name[:len(label)], label) {
			continue
		}
		rest := strings.TrimSpace(name[len(label):])
		rest = strings.TrimLeft(rest, " \t\r\n·・-—–|｜:/\\")
		if rest != "" {
			return strings.TrimSpace(rest)
		}
		if strings.EqualFold(name, label) {
			return ""
		}
	}
	return name
}

func cloudMountDirBase(dir string) string {
	dir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")), "/")
	if dir == "" {
		return ""
	}
	parts := strings.Split(dir, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if part := strings.TrimSpace(parts[i]); part != "" {
			return part
		}
	}
	return ""
}

func normalizeLibraryMergeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	return strings.Join(strings.Fields(name), " ")
}

func ShadowedCloudLibraryIDSet(libs []model.Library) map[string]bool {
	out := make(map[string]bool)
	for _, lib := range libs {
		if CloudLibraryShadowed(libs, lib) != nil {
			out[lib.ID] = true
		}
	}
	return out
}

func InferCloudMountMediaType(dir, name string) string {
	text := strings.ToLower(dir + " " + name)
	switch {
	case strings.Contains(text, "成人") || strings.Contains(text, "adult") || strings.Contains(text, "jav") || strings.Contains(text, "9kg"):
		return "adult"
	case containsAny(text, "动画电影", "华语电影", "外语电影", "欧美电影", "日韩电影", "韩国电影", "日本电影", "港台电影", "香港电影", "台湾电影", "大陆电影", "国产电影", "纪录片", "演唱会", "电影", "movie", "movies", "film", "films", "documentary", "concert"):
		return "movie"
	case containsAny(text, "综艺", "真人秀", "脱口秀", "晚会", "variety"):
		return "variety"
	case containsAny(text, "国漫", "日漫", "日番", "番剧", "动漫", "欧美动漫", "动画剧集", "anime"):
		return "anime"
	case containsAny(text, "国产剧", "大陆剧", "华语剧", "欧美剧", "日韩剧", "韩剧", "日剧", "港剧", "台剧", "泰剧", "英剧", "美剧", "短剧", "电视剧", "剧集", "连续剧", "series", "tv", "shows"):
		return "tv"
	default:
		return "movie"
	}
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func cloudMountAncestor(parent, child string) bool {
	parent = strings.Trim(parent, "/")
	child = strings.Trim(child, "/")
	if parent == child {
		return false
	}
	if parent == "" {
		return child != ""
	}
	return strings.HasPrefix(child, parent+"/")
}

func normalizeCloudMountDir(provider, value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.PathUnescape(value); err == nil {
		value = decoded
	}
	if decoded, err := url.QueryUnescape(value); err == nil {
		value = decoded
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "." || ((provider == cloud.Type115 || provider == cloud.TypeQuark) && value == "0") {
		return ""
	}
	return value
}
