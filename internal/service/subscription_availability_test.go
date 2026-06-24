package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionEnrichProgressIncludesPendingDownloads(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Media{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:4444444444444444444444444444444444444444",
		Title:    "Inception 2010 1080p",
		SavePath: "/downloads/movies",
		Status:   "completed",
		Progress: 1,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	items := []model.Subscription{{
		Name:      "Inception 2010",
		Filter:    "Inception 2010",
		MediaType: "movie",
		SavePath:  "/downloads/movies",
	}}

	svc.EnrichProgress(t.Context(), items)
	if items[0].InLibrary {
		t.Fatal("pending download should not be reported as in-library media")
	}
	if items[0].DownloadedEpisodes != 1 || items[0].LocalMediaCount != 1 || items[0].TotalEpisodes != 1 {
		t.Fatalf("unexpected enriched progress: %+v", items[0])
	}
}

func TestSubscriptionLocalAvailabilityMatchesMediaPath(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	if err := db.Create(&model.Media{
		Title:      "Scraped English Title",
		Path:       "/media/电视剧/国产剧/凡人修仙传/Season 01/凡人修仙传 - S01E146.mkv",
		SeasonNum:  1,
		EpisodeNum: 146,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sub := &model.Subscription{
		Name:          "凡人修仙传 年番",
		Filter:        "凡人修仙传",
		MediaType:     "tv",
		TotalEpisodes: 146,
	}

	availability := SubscriptionLocalAvailability(t.Context(), repos, sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 146)]; !ok {
		t.Fatalf("missing path-matched E146 key: %#v", availability.ExistingEpisodeKeys)
	}
	results := []SearchResult{
		{Title: "凡人修仙传 年番 - 146 1080p", DownloadURL: "https://pt/download/146", Seeders: 80},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because path-matched local episode exists", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIgnoresDeletedTasks(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:3333333333333333333333333333333333333333",
		Title:    "间谍过家家 S01E02 1080p",
		SavePath: "/downloads/tv",
		Status:   "deleted",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)
	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 3,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; ok {
		t.Fatalf("deleted E02 task should not count as available: %#v", availability.ExistingEpisodeKeys)
	}
	results := []SearchResult{
		{Title: "间谍过家家 S01E02 1080p WEB-DL", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p WEB-DL", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 2 || got[0].Episode != 2 || got[1].Episode != 3 {
		t.Fatalf("selected %#v, want deleted episode 2 and new episode 3", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesLiveQBTorrents(t *testing.T) {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"abc123","name":"间谍过家家 S01E01 1080p","state":"downloading","progress":0.2}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	downloads.qb.Configure(QBitConfig{BaseURL: qb.URL, Username: "admin", Password: "admin"})
	svc := NewSubscriptionService(nil, nil, repos, downloads, nil, nil)
	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 2,
	}

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("downloaded episodes = %d, want 1", availability.DownloadedEpisodes)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 1)]; !ok {
		t.Fatalf("missing live qB E01 key: %#v", availability.ExistingEpisodeKeys)
	}
}
