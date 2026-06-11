package service

import (
	"slices"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestFilterDisplayCloudLibrariesPrefersPopulatedCanonicalDuplicate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	now := time.Now()
	oldEmpty := model.Library{
		Base:    model.Base{ID: "old-empty", CreatedAt: now.Add(-time.Hour)},
		Name:    "OpenList · 国产剧",
		Path:    "cloud://openlist/%2F国产剧",
		Type:    "tv",
		Enabled: true,
	}
	newPopulated := model.Library{
		Base:    model.Base{ID: "new-populated", CreatedAt: now},
		Name:    "OpenList · 国产剧",
		Path:    BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &oldEmpty); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &newPopulated); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		LibraryID: newPopulated.ID,
		Title:     "剧集",
		Path:      "cloud://openlist/国产剧/剧集.mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}

	filtered := FilterDisplayCloudLibraries(t.Context(), repos, []model.Library{oldEmpty, newPopulated})
	if len(filtered) != 1 || filtered[0].ID != newPopulated.ID {
		t.Fatalf("filtered = %#v, want only populated canonical duplicate", filtered)
	}

	scanner := NewScannerService(nil, zap.NewNop(), repos, nil, nil, nil)
	if conflict := scanner.shadowedCloudLibrary(t.Context(), &oldEmpty); conflict == nil || conflict.Library.ID != newPopulated.ID {
		t.Fatalf("old duplicate scan conflict = %#v, want populated canonical library", conflict)
	}
}

func TestFilterDisplayCloudLibrariesMergesCloudMountIntoExistingLibrary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	local := model.Library{Name: "国产剧", Path: "/media/国产剧", Type: "tv", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"), Type: "tv", Enabled: true}
	movieCloud := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/电影/国产剧", "/电影/国产剧"), Type: "movie", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud, &movieCloud} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}

	filtered := FilterDisplayCloudLibraries(t.Context(), repos, []model.Library{local, cloud, movieCloud})
	if got := libraryNames(filtered); !slices.Equal(got, []string{"国产剧", "国产剧"}) {
		t.Fatalf("filtered names = %#v, want local tv plus stripped movie cloud", got)
	}
	if filtered[0].ID != local.ID {
		t.Fatalf("first filtered library = %s, want existing local library %s", filtered[0].ID, local.ID)
	}
	if filtered[1].ID != movieCloud.ID {
		t.Fatalf("movie cloud library should stay separate when type differs: %#v", filtered)
	}

	merged := MergedLibraryIDs([]model.Library{local, cloud, movieCloud}, local)
	if !slices.Equal(merged, []string{local.ID, cloud.ID}) {
		t.Fatalf("merged ids = %#v, want local+same-type cloud", merged)
	}
}

func TestListMediaVisibleIncludesMergedCloudLibraryItems(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	local := model.Library{Name: "国产剧", Path: "/media/国产剧", Type: "tv", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"), Type: "tv", Enabled: true}
	other := model.Library{Name: "欧美剧", Path: BuildCloudLibraryPath("openlist", "/欧美剧", "/欧美剧"), Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud, &other} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.DB.Create(&[]model.Media{
		{LibraryID: local.ID, Title: "本地剧", Path: "/media/国产剧/local.mkv"},
		{LibraryID: cloud.ID, Title: "云盘剧", Path: "cloud://openlist/国产剧/cloud.mkv"},
		{LibraryID: other.ID, Title: "其他剧", Path: "cloud://openlist/欧美剧/other.mkv"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	items, total, err := svc.ListMediaVisible(t.Context(), local.ID, 1, 20, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want merged local+cloud items", total)
	}
	if got := mediaTitles(items); !slices.Equal(got, []string{"云盘剧", "本地剧"}) {
		t.Fatalf("items = %#v, want local+cloud only", got)
	}

	items, total, err = svc.ListMediaVisible(t.Context(), local.ID, 1, 20, MediaVisibility{
		IncludeNSFW:       true,
		AllowedLibraryIDs: []string{local.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || !slices.Equal(mediaTitles(items), []string{"云盘剧", "本地剧"}) {
		t.Fatalf("profile-limited merged list total=%d items=%#v", total, mediaTitles(items))
	}
}

func TestStartAllCloudLibraryScansIncludesMergedCloudMounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	local := model.Library{Name: "国产剧", Path: "/media/国产剧", Type: "tv", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产剧", Path: BuildCloudLibraryPath("openlist", "/国产剧", "/国产剧"), Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)

	statuses, err := scanner.StartAllCloudLibraryScans()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].LibraryID != cloud.ID {
		t.Fatalf("scan-all statuses = %#v, want merged cloud library queued", statuses)
	}
}

func libraryNames(libs []model.Library) []string {
	out := make([]string, 0, len(libs))
	for _, lib := range libs {
		out = append(out, lib.Name)
	}
	return out
}

func mediaTitles(items []model.Media) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Title)
	}
	slices.Sort(out)
	return out
}
