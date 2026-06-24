package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEnrichOneUsesExistingTMDbIDForCloudMedia(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "OpenList · 国漫", Path: "cloud://openlist/国漫", Type: "anime", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "dirty release title",
		Path:         "cloud://openlist/国漫/间谍过家家 (2022) {tmdb-12345}/Season 1/间谍过家家.S01E01.2160p.mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.Title != "间谍过家家" || got.TMDbID != 12345 || got.PosterURL == "" {
		t.Fatalf("tmdb id scrape did not apply match: title=%q status=%q tmdb=%d poster=%q", got.Title, got.ScrapeStatus, got.TMDbID, got.PosterURL)
	}
}

func TestEnrichOneWritesTMDbIDColumn(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}

	var media model.Media
	if err := repos.DB.First(&media, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("unexpected scraped media: status=%q tmdb=%d", got.ScrapeStatus, got.TMDbID)
	}
}

func TestEnrichOneTreatsEpisodicMediaInMovieLibraryAsTV(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "混合库", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家 S02E01",
		Path:         filepath.Join(lib.Path, "间谍过家家", "Season 02", "间谍过家家 - S02E01.mkv"),
		SeasonNum:    2,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("episodic media in movie library should use tv scrape: status=%q tmdb=%d", got.ScrapeStatus, got.TMDbID)
	}
}

func TestEnrichOneWritesTMDbEpisodeMetadata(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv")
	existingPoster := "https://image.tmdb.org/t/p/w500/existing-poster.jpg"
	existingBackdrop := "https://image.tmdb.org/t/p/w1280/existing-backdrop.jpg"
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		PosterURL:    existingPoster,
		BackdropURL:  existingBackdrop,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	// 单集专属信息(简介/剧照/评分/时长)应回填到该集行。
	if got.Overview != "单集剧情" {
		t.Fatalf("episode overview not saved: overview=%q", got.Overview)
	}
	if !strings.HasSuffix(got.BackdropURL, "/images/w500/still.jpg") || got.DurationSec != 24*60 {
		t.Fatalf("episode still/runtime not saved: backdrop=%q duration=%d", got.BackdropURL, got.DurationSec)
	}
	if got.Rating < 9.09 || got.Rating > 9.11 {
		t.Fatalf("episode rating = %v, want 9.1", got.Rating)
	}
	if got.EpisodeTitle != "任务代号: 猫" {
		t.Fatalf("episode_title should store per-episode name, got %q", got.EpisodeTitle)
	}
	// original_name 必须保持「整剧原名」,绝不能被单集名(任务代号: 猫)覆盖,
	// 否则同剧每集 original_name 不同会导致合集被拆成多集无法合并。
	if got.OriginalName != "SPY×FAMILY" {
		t.Fatalf("original_name should stay series-level, got %q (episode name must not overwrite it)", got.OriginalName)
	}
}

func TestEnrichOneSkipsTMDbEpisodeStillWhenDisabled(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv")
	existingPoster := "https://image.tmdb.org/t/p/w500/existing-poster.jpg"
	existingBackdrop := "https://image.tmdb.org/t/p/w1280/existing-backdrop.jpg"
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		PosterURL:    existingPoster,
		BackdropURL:  existingBackdrop,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	episodeArtwork := false
	if err := scraper.EnrichOneWithOptions(t.Context(), &media, ScrapeOptions{EpisodeArtwork: &episodeArtwork}); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Overview != "单集剧情" || got.DurationSec != 24*60 {
		t.Fatalf("episode metadata should still be saved: overview=%q duration=%d", got.Overview, got.DurationSec)
	}
	if got.Rating < 9.09 || got.Rating > 9.11 {
		t.Fatalf("episode rating = %v, want 9.1", got.Rating)
	}
	if strings.HasSuffix(got.BackdropURL, "/images/w500/still.jpg") {
		t.Fatalf("episode still should not be saved when disabled: backdrop=%q", got.BackdropURL)
	}
	if !strings.HasSuffix(got.PosterURL, "/images/w500/poster.jpg") {
		t.Fatalf("series poster should still be saved when episode artwork is disabled: got %q", got.PosterURL)
	}
	if !strings.HasSuffix(got.BackdropURL, "/images/w1280/backdrop.jpg") {
		t.Fatalf("series backdrop should still be saved when episode artwork is disabled: got %q", got.BackdropURL)
	}
	if got.PosterURL == existingPoster || got.BackdropURL == existingBackdrop {
		t.Fatalf("main artwork should be refreshed while episode still is skipped: poster=%q backdrop=%q", got.PosterURL, got.BackdropURL)
	}
}

func TestApplyManualMatchSkipsTMDbEpisodeStillWhenDisabled(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "待匹配",
		Path:         filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv"),
		SeasonNum:    2,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	episodeArtwork := false
	got, err := scraper.ApplyManualMatch(t.Context(), media.ID, ManualScrapeRequest{
		Source:         "tmdb",
		MediaType:      "tv",
		Title:          "间谍过家家",
		TMDbID:         12345,
		EpisodeArtwork: &episodeArtwork,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("manual match returned nil media")
	}
	if got.Overview != "单集剧情" || got.DurationSec != 24*60 {
		t.Fatalf("episode metadata should still be saved: overview=%q duration=%d", got.Overview, got.DurationSec)
	}
	if strings.HasSuffix(got.BackdropURL, "/images/w500/still.jpg") {
		t.Fatalf("manual episode still should not be saved when disabled: backdrop=%q", got.BackdropURL)
	}
	if !strings.HasSuffix(got.PosterURL, "/images/w500/poster.jpg") {
		t.Fatalf("series poster should still be saved when manual episode artwork is disabled: got %q", got.PosterURL)
	}
	if !strings.HasSuffix(got.BackdropURL, "/images/w1280/backdrop.jpg") {
		t.Fatalf("series backdrop should still be saved when manual episode artwork is disabled: got %q", got.BackdropURL)
	}
}

func TestEnrichOneRejectsWrongYearMatchFromSeriesFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             999,
					"name":           "Parade of Stars Auto Show",
					"first_air_date": "1952-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	root := t.TempDir()
	mediaPath := filepath.Join(root, "Auto Show (2026)", "Season 1", "Auto Show - S01E03 - 第 3 集.mkv")
	lib := model.Library{Name: "剧集", Path: root, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "auto show",
		Path:         mediaPath,
		SeasonNum:    1,
		EpisodeNum:   3,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "no_match" || got.Title != "auto show" || got.Year != 0 || got.TMDbID != 0 {
		t.Fatalf("wrong-year scrape should be rejected, got status=%q title=%q year=%d tmdb=%d", got.ScrapeStatus, got.Title, got.Year, got.TMDbID)
	}
}

func TestEnrichOnePrefersLocalMetadataWithoutProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log))

	root := t.TempDir()
	showDir := filepath.Join(root, "间谍过家家")
	seasonDir := filepath.Join(showDir, "Season 02")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<tvshow>
<title>间谍过家家</title>
<year>2022</year>
<tmdbid>120089</tmdbid>
<genre>Animation</genre>
</tvshow>`), 0o644); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(seasonDir, "间谍过家家 - S02E12.mkv")
	if err := os.WriteFile(nfoPath(mediaPath), []byte(`<episodedetails>
<title>企鹅公园</title>
<showtitle>间谍过家家</showtitle>
<season>2</season>
<episode>12</episode>
<plot>本地剧情</plot>
</episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := model.Library{Name: "番剧", Path: root, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "bad title",
		Path:         mediaPath,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.Title != "间谍过家家" || got.TMDbID != 120089 {
		t.Fatalf("unexpected local scrape: status=%q title=%q tmdb=%d", got.ScrapeStatus, got.Title, got.TMDbID)
	}
	if got.SeasonNum != 2 || got.EpisodeNum != 12 || got.Overview != "本地剧情" {
		t.Fatalf("unexpected local episode data: s=%d e=%d overview=%q", got.SeasonNum, got.EpisodeNum, got.Overview)
	}
	if got.EpisodeTitle != "企鹅公园" {
		t.Fatalf("episode_title = %q, want local episode title", got.EpisodeTitle)
	}
}

func TestManualEnrichLibraryRetriesNoMatchAndCountsRealMatches(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(lib.Path, "间谍过家家 - S02E02.mkv")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "no_match",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if matched, err := scraper.EnrichLibrary(t.Context(), lib.ID); err != nil || matched != 0 {
		t.Fatalf("default EnrichLibrary matched=%d err=%v, want skipped no_match", matched, err)
	}
	if matched, err := scraper.EnrichLibrary(t.Context(), lib.ID, true); err != nil || matched != 1 {
		t.Fatalf("manual EnrichLibrary matched=%d err=%v, want one real match", matched, err)
	}
}

func TestManualEnrichLibraryCanRefreshAlreadyMatchedRows(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         filepath.Join(lib.Path, "间谍过家家 - S02E02.mkv"),
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	defaultResult, err := scraper.EnrichLibraryDetailedWithOptions(t.Context(), lib.ID, ScrapeOptions{RetryNoMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if defaultResult.Processed != 0 || defaultResult.Candidates != 0 {
		t.Fatalf("default manual scrape result=%+v, want matched rows skipped without IncludeMatched", defaultResult)
	}

	refreshResult, err := scraper.EnrichLibraryDetailedWithOptions(t.Context(), lib.ID, ScrapeOptions{
		RetryNoMatch:   true,
		IncludeMatched: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshResult.Processed != 1 || refreshResult.Matched != 1 || refreshResult.Candidates != 1 {
		t.Fatalf("refresh result=%+v, want already matched row reprocessed", refreshResult)
	}
}

func TestScrapeCandidateRowsPrioritizeLibraryArtworkBeforeEpisodes(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{
			Base:         model.Base{ID: "001-episode"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家 第 1 集",
			Path:         filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv"),
			SeasonNum:    2,
			EpisodeNum:   1,
			ScrapeStatus: "pending",
		},
		{
			Base:         model.Base{ID: "999-series"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			Path:         filepath.Join(lib.Path, "间谍过家家.mkv"),
			ScrapeStatus: "pending",
		},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := scraper.scrapeCandidateRows(t.Context(), lib.ID, ScrapeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("candidate rows = %d, want 2", len(got))
	}
	if got[0].ID != "999-series" || got[1].ID != "001-episode" {
		t.Fatalf("scrape order = [%s, %s], want series-level row before episode row", got[0].ID, got[1].ID)
	}
}

func TestEnrichLibraryIncludesMergedCloudLibraryMedia(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	local := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	cloud := model.Library{
		Name:    "OpenList · 番剧",
		Path:    BuildCloudLibraryPath("openlist", "/番剧", "/番剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&cloud).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		LibraryID:    cloud.ID,
		Title:        "间谍过家家",
		Path:         "cloud://openlist/番剧/间谍过家家 - S02E02.mkv",
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := scraper.EnrichLibraryDetailed(t.Context(), local.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Processed != 1 || result.Candidates != 1 || result.Failed != 0 {
		t.Fatalf("result=%+v, want merged cloud media to be scraped once", result)
	}
	var got model.Media
	if err := repos.DB.First(&got, "library_id = ?", cloud.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("merged cloud media was not enriched: status=%q tmdb=%d", got.ScrapeStatus, got.TMDbID)
	}
}

func newTestScraper(t *testing.T) (*ScraperService, *repository.Container, func()) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             12345,
					"name":           "间谍过家家",
					"original_name":  "SPY×FAMILY",
					"overview":       "测试简介",
					"poster_path":    "/poster.jpg",
					"backdrop_path":  "/backdrop.jpg",
					"first_air_date": "2022-04-09",
					"vote_average":   8.6,
				}},
			})
		case r.URL.Path == "/tv/12345/season/2/episode/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":         "任务代号: 猫",
				"overview":     "单集剧情",
				"still_path":   "/still.jpg",
				"air_date":     "2023-10-07",
				"vote_average": 9.1,
				"runtime":      24,
			})
		case strings.HasPrefix(r.URL.Path, "/tv/12345"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             12345,
				"name":           "间谍过家家",
				"overview":       "测试简介",
				"poster_path":    "/poster.jpg",
				"backdrop_path":  "/backdrop.jpg",
				"first_air_date": "2022-04-09",
				"vote_average":   8.6,
				"origin_country": []string{"JP"},
				"spoken_languages": []map[string]any{{
					"iso_639_1": "ja",
				}},
				"genres": []map[string]any{{
					"name": "Animation",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = upstream.URL + "/images"
	log := zap.NewNop()
	tmdb := NewTMDbProvider(cfg, log, nil)
	scraper := NewScraperService(cfg, log, repos, tmdb, nil, nil, nil, NewHub(log))

	return scraper, repos, upstream.Close
}
