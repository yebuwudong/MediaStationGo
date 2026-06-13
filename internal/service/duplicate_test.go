package service

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDuplicateDetectMarksExternalIdentityDuplicates(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	root := t.TempDir()
	firstPath := filepath.Join(root, "show-a.mkv")
	secondPath := filepath.Join(root, "show-b.mkv")
	writeOrgFile(t, firstPath, "first-release")
	writeOrgFile(t, secondPath, "second-release")

	lib := model.Library{Name: "剧集", Path: root, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	first := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         firstPath,
		SizeBytes:    13,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "matched",
	}
	second := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy Family",
		Path:         secondPath,
		SizeBytes:    14,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&second).Error; err != nil {
		t.Fatal(err)
	}

	report, err := NewDuplicateService(zap.NewNop(), repos, nil).Detect(t.Context(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.ItemsMarked != 1 || report.GroupsFound != 1 {
		t.Fatalf("report = %#v, want one external identity duplicate", report)
	}
	var rows []model.Media
	if err := repos.DB.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, row := range rows {
		if row.IsDuplicate && row.DuplicateOf != "" {
			marked++
		}
	}
	if marked != 1 {
		t.Fatalf("marked duplicate rows = %d, want 1; rows=%#v", marked, rows)
	}
}
