package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestOrganizeDirectoryClassifiesScraperMatchBeforeRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                45745,
					"title":             "寻龙记",
					"original_title":    "Sintel",
					"original_language": "en",
					"genre_ids":         []int{16, 14},
					"release_date":      "2010-09-27",
					"vote_average":      7.4,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Sintel.2010.1080p.CodexVerify.mp4")
	writeOrgFile(t, sourceFile, "movie")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "movie",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电影", "动画电影", "寻龙记 (2010)", "寻龙记 (2010).mp4")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized movie should use metadata category path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "动画电影" {
		t.Fatalf("organize category = %#v, want 动画电影", res.Items)
	}
}

func TestOrganizeDirectoryMetadataCategoryOverridesDownloadFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                12345,
					"name":              "间谍过家家",
					"original_name":     "SPY×FAMILY",
					"original_language": "ja",
					"origin_country":    []string{"JP"},
					"genre_ids":         []int{16, 35},
					"first_air_date":    "2022-04-09",
					"vote_average":      8.6,
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "国产剧", "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "动漫", "日番", "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("metadata category should override wrong source folder at %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "日番" || res.Items[0].MediaType != "anime" {
		t.Fatalf("organize metadata category/type = %#v, want 日番/anime", res.Items)
	}
}

func TestOrganizeDirectoryRejectedMetadataKeepsExplicitWesternSourceCategory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":                107463,
				"name":              "镖人",
				"original_name":     "Biao Ren",
				"original_language": "zh",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{16, 18},
				"first_air_date":    "2023-06-01",
				"vote_average":      8.0,
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "欧美剧", "Blades.of.the.Guardians.S02.1080p.TX.WEB-DL", "Blades.of.the.Guardians.S02E01.1080p.TX.WEB-DL.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "欧美剧", "Blades Of The Guardians", "Season 02", "Blades Of The Guardians - S02E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("rejected metadata should fall back to explicit source category at %q: %v; items=%#v", want, err, res.Items)
	}
	wrong := filepath.Join(dest, "电视剧", "国产剧", "Blades Of The Guardians")
	if _, err := os.Stat(wrong); !os.IsNotExist(err) {
		t.Fatalf("rejected metadata must not fall back to domestic category %q, err=%v", wrong, err)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "欧美剧" || res.Items[0].MediaType != "tv" || res.Items[0].Title != "Blades Of The Guardians" {
		t.Fatalf("organize item = %#v, want Blades Of The Guardians in 欧美剧/tv", res.Items)
	}
}

func TestOrganizeDirectoryDoesNotScrapeByDownloadCategoryFolder(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query().Get("query")
		queries = append(queries, query)
		switch {
		case query == "国产剧":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             843248,
					"name":           "高达 G之复国运动 剧场版III 来自宇宙的遗产",
					"first_air_date": "2021-07-22",
				}},
			})
		case strings.EqualFold(query, "ashes to crown"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                289271,
					"name":              "翘楚",
					"original_name":     "Ashes to Crown",
					"original_language": "zh",
					"origin_country":    []string{"CN"},
					"genre_ids":         []int{18},
					"first_air_date":    "2026-06-01",
				}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(root, "downloads", "国产剧", "Ashes.to.Crown.S01.1080p.YOUKU.WEB-DL.AAC2.0.H.264-MWeb", "Ashes.to.Crown.S01E06.1080p.YOUKU.WEB-DL.AAC2.0.H.264-MWeb.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:           sourceFile,
		DestPath:             dest,
		TransferMode:         TransferCopy,
		MediaType:            "tv",
		MediaCategory:        "国产剧",
		AllowReplaceExisting: false,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "国产剧", "翘楚", "Season 01", "翘楚 - S01E06.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v queries=%#v", res.Organized, res.Items, res.Errors, queries)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use release title metadata, not category query, at %q: %v; items=%#v queries=%#v", want, err, res.Items, queries)
	}
	if len(queries) == 0 || queries[0] == "国产剧" {
		t.Fatalf("first scrape query = %#v, want release title before category folder", queries)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "翘楚" || res.Items[0].Category != "国产剧" {
		t.Fatalf("organize item = %#v, want title 翘楚 in category 国产剧", res.Items)
	}
}

func TestOrganizeDirectoryEpisodeMarkerOverridesMovieSourceFolder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                100088,
					"name":              "The Last of Us",
					"original_language": "en",
					"origin_country":    []string{"US"},
					"genre_ids":         []int{18},
					"first_air_date":    "2023-01-15",
					"vote_average":      8.7,
				}},
			})
		case r.URL.Path == "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":           999,
					"title":        "Wrong Movie",
					"release_date": "2023-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "外语电影", "The.Last.of.Us.S01E01.2023.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(dest, "电视剧", "欧美剧", "The Last of Us", "Season 01", "The Last of Us - S01E01.mkv")
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("episode marker should force TV organize path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Category != "欧美剧" || res.Items[0].MediaType != "tv" {
		t.Fatalf("organize episode category/type = %#v, want 欧美剧/tv", res.Items)
	}
}
