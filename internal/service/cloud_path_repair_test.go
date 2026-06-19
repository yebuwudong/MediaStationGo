package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// TestResetEpisodicMatchedForRescrape 验证「修复+重刮」会把脏的 matched 剧集行
// 重置为 pending(让 EnrichLibrary 能重新刮削),而电影行与其它库不受影响。
func TestResetEpisodicMatchedForRescrape(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	container := &Container{Repo: repository.New(db), Log: zap.NewNop()}

	rows := []model.Media{
		// 目标库的剧集行(matched, 有季集号)→ 应被重置。
		{Base: model.Base{ID: "ep1"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", Path: "/a/show/S01/ep1.mkv"},
		{Base: model.Base{ID: "ep2"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 2, ScrapeStatus: "matched", Path: "/a/show/S01/ep2.mkv"},
		// 目标库的电影行(无季集号)→ 不应被重置。
		{Base: model.Base{ID: "movie1"}, LibraryID: "lib-a", ScrapeStatus: "matched", Path: "/a/movie.mkv"},
		// 目标库已是 pending 的剧集行 → 不计入重置数。
		{Base: model.Base{ID: "ep3"}, LibraryID: "lib-a", SeasonNum: 1, EpisodeNum: 3, ScrapeStatus: "pending", Path: "/a/show/S01/ep3.mkv"},
		// 其它库的剧集行(matched)→ 单库重置时不应受影响。
		{Base: model.Base{ID: "ep-other"}, LibraryID: "lib-b", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", Path: "/b/show/S01/ep1.mkv"},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media %s: %v", rows[i].ID, err)
		}
	}

	reset, err := container.resetEpisodicMatchedForRescrape(t.Context(), "lib-a")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if reset != 2 {
		t.Fatalf("reset = %d, want 2 (only matched episodic rows in lib-a)", reset)
	}

	status := func(id string) string {
		var m model.Media
		if err := db.First(&m, "id = ?", id).Error; err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		return m.ScrapeStatus
	}
	if status("ep1") != "pending" || status("ep2") != "pending" {
		t.Fatalf("episodic matched rows should be pending, got ep1=%q ep2=%q", status("ep1"), status("ep2"))
	}
	if status("movie1") != "matched" {
		t.Fatalf("movie row should stay matched, got %q", status("movie1"))
	}
	if status("ep-other") != "matched" {
		t.Fatalf("other library row should stay matched, got %q", status("ep-other"))
	}
}
