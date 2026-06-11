package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMediaSearchFilteredSupportsChineseFuzzyTerms(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "国产剧", Path: "/media/国产剧", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:         model.Base{ID: "m-ferry"},
			LibraryID:    lib.ID,
			Title:        "灵魂摆渡·十年",
			OriginalName: "The Ferry Man 10th Anniversary",
			Path:         "/media/国产剧/灵魂摆渡·十年/S01E01.mkv",
			Genres:       "悬疑,奇幻",
		},
		{
			Base:         model.Base{ID: "m-ashes"},
			LibraryID:    lib.ID,
			Title:        "翘楚",
			OriginalName: "Ashes to Crown",
			Path:         "/media/国产剧/翘楚/S01E01.mkv",
			Genres:       "剧情",
		},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatalf("upsert media: %v", err)
		}
	}

	items, err := repos.Media.SearchFiltered(t.Context(), "灵魂 十年", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search chinese terms: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("chinese fuzzy search missed target: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "Ferry", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search original name: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("original-name search missed target: %#v", items)
	}

	items, err = repos.Media.SearchFiltered(t.Context(), "悬疑", 10, MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatalf("search genre: %v", err)
	}
	if len(items) == 0 || items[0].ID != "m-ferry" {
		t.Fatalf("genre search missed target: %#v", items)
	}
}

func TestMediaSearchIndexBackfillRunsInBatches(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := New(db)
	lib := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:      model.Base{ID: "m-backfill"},
		LibraryID: lib.ID,
		Title:     "后台索引",
		Path:      "/media/movie/后台索引.mkv",
	}).Error; err != nil {
		t.Fatal(err)
	}
	var before int64
	if err := repos.DB.Raw(`SELECT COUNT(*) FROM media_search_fts`).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("startup migrate should not synchronously backfill FTS, got %d rows", before)
	}
	n, err := repos.Media.BackfillSearchIndex(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("backfilled rows = %d, want 1", n)
	}
	var after int64
	if err := repos.DB.Raw(`SELECT COUNT(*) FROM media_search_fts`).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != 1 {
		t.Fatalf("fts rows = %d, want 1", after)
	}
}
