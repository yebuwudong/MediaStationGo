package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryScanAndScrapeAfter(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()
	if err := repos.DB.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    filepath.Join(dest, "电视剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1", res.Organized)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	scans, scrapes := scanner.ScanAndScrapeLibrariesForPath(t.Context(), res.DestPath, "", true)
	if len(scans) != 1 || scans[0].Added != 1 {
		t.Fatalf("scans = %#v, want one scan with added=1", scans)
	}
	if len(scrapes) != 1 || scrapes[0].Matched != 1 || scrapes[0].Error != "" || scrapes[0].Skipped {
		t.Fatalf("scrapes = %#v, want one successful matched scrape", scrapes)
	}

	var media model.Media
	if err := repos.DB.Where("path LIKE ?", "%Spy Family - S01E01.mkv").First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.ScrapeStatus != "matched" || media.TMDbID != 12345 {
		t.Fatalf("media scrape status=%q tmdb=%d, want matched/12345", media.ScrapeStatus, media.TMDbID)
	}
	if _, err := os.Stat(media.Path); err != nil {
		t.Fatalf("organized file missing at %q: %v", media.Path, err)
	}
}

func TestOrganizeDirectoryUsesScraperMatchBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1", res.Organized)
	}
	want := filepath.Join(dest, "电视剧", "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use matched metadata path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Target != want || res.Items[0].Title != "间谍过家家" {
		t.Fatalf("organize preview did not use scraper metadata: %#v", res.Items)
	}
}

func TestOrganizeDirectoryDedupsByExternalIDBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.2160p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	existingPath := filepath.Join(dest, "电视剧", "旧错误名", "Season 01", "旧错误名 - S01E01.mkv")
	writeOrgFile(t, existingPath, "existing")
	lib := model.Library{Name: "剧集", Path: filepath.Join(dest, "电视剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "旧错误名",
		Path:         existingPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 0 || res.Skipped != 1 {
		t.Fatalf("organize result = organized %d skipped %d, want 0/1; items=%#v errors=%#v", res.Organized, res.Skipped, res.Items, res.Errors)
	}
	if len(res.Items) != 1 || res.Items[0].Reason != organizeSkipDuplicateLibrary {
		t.Fatalf("source should be skipped as external-id duplicate: %#v", res.Items)
	}
	if _, err := os.Stat(sourceFile); err != nil {
		t.Fatalf("duplicate source should remain untouched: %v", err)
	}
}

func TestOrganizeDirectoryUsesBangumiForAnimeRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/subject/frieren" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": 1,
			"list": []map[string]any{{
				"id":       889,
				"name":     "Frieren",
				"name_cn":  "葬送的芙莉莲",
				"air_date": "2023-09-29",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	bangumi := NewBangumiProvider(cfg, zap.NewNop())
	bangumi.base = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, nil, bangumi, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Frieren.S01E01.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "anime",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "葬送的芙莉莲", "Season 01", "葬送的芙莉莲 - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use Bangumi metadata path %q: %v", want, err)
	}
}

func TestOrganizeScanAndScrapeRetriesNoMatchRows(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	libRoot := filepath.Join(root, "media", "电视剧")
	mediaPath := filepath.Join(libRoot, "间谍过家家", "Season 02", "间谍过家家 - S02E02.mkv")
	writeOrgFile(t, mediaPath, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    libRoot,
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "no_match",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	_, scrapes := scanner.ScanAndScrapeLibrariesForPath(t.Context(), filepath.Join(root, "media"), "", true)
	if len(scrapes) != 1 || scrapes[0].Matched != 1 || scrapes[0].Error != "" || scrapes[0].Skipped {
		t.Fatalf("scrapes = %#v, want one retried no_match row", scrapes)
	}

	var got model.Media
	if err := repos.DB.First(&got, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("media scrape status=%q tmdb=%d, want matched/12345", got.ScrapeStatus, got.TMDbID)
	}
}

func TestOrganizeResultNeedsVisibilitySyncIgnoresScannedDuplicates(t *testing.T) {
	if OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipDuplicateLibrary}},
	}) {
		t.Fatal("already-scanned duplicate should not trigger another visibility scan")
	}
	if OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipSampleClip}},
	}) {
		t.Fatal("sample clip skip should not trigger visibility scan")
	}
	if !OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipTargetExists}},
	}) {
		t.Fatal("unscanned target file should trigger visibility scan")
	}
	if !OrganizeResultNeedsVisibilitySync(&OrganizeResult{Organized: 1}) {
		t.Fatal("organized files must trigger visibility scan")
	}
}

func TestOrganizeScrapeAfterEnabledDefaultsOn(t *testing.T) {
	if !OrganizeScrapeAfterEnabled(t.Context(), nil) {
		t.Fatalf("organize scrape-after should default on without a repo")
	}
	repos := newOrganizerTestRepo(t)
	if !OrganizeScrapeAfterEnabled(t.Context(), repos) {
		t.Fatalf("organize scrape-after should default on when setting is absent")
	}
	if err := repos.Setting.Set(t.Context(), "organize.scrape_after", "false"); err != nil {
		t.Fatal(err)
	}
	if OrganizeScrapeAfterEnabled(t.Context(), repos) {
		t.Fatalf("explicit organize.scrape_after=false should be respected")
	}
}
