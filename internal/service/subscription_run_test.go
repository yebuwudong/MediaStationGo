package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionRunOneArchivesCompletedMovieRSS(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Dune 2021 1080p WEB-DL</title>
    <guid>dune-1080-web</guid>
    <link>magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&amp;dn=Dune+2021+1080p+WEB-DL</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var added bool
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if added {
				_, _ = w.Write([]byte(`[{"hash":"dunehash","name":"Dune 2021 1080p WEB-DL","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			added = true
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:      "Dune 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Dune 2021",
		MediaType: "movie",
		SavePath:  "/downloads/movies",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %d, want 0 after completion", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ArchivedAt == nil {
		t.Fatalf("history = %#v, want one archived subscription", history)
	}
}

func TestSubscriptionArchiveCompletedSingleEpisodeTV(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:      "Some Show S01E01 自动订阅",
		FeedURL:   "site-search://search?keyword=Some%20Show%20S01E01",
		Filter:    "Some Show S01E01",
		MediaType: "tv",
		Enabled:   true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}

	if err := svc.archiveCompletedSubscription(t.Context(), sub, LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
		},
	}); err != nil {
		t.Fatal(err)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active subscriptions = %d, want 0", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ArchiveReason == "" {
		t.Fatalf("history = %#v, want archived single episode", history)
	}
}

func TestSubscriptionArchiveKeepsGenericUnknownTotalSeriesActive(t *testing.T) {
	sub := &model.Subscription{
		Name:      "Some Show 自动订阅",
		Filter:    "Some Show",
		MediaType: "tv",
	}
	availability := LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
		},
	}
	if subscriptionShouldArchive(sub, availability) {
		t.Fatal("generic series with unknown total should stay active for incremental episodes")
	}
}

func TestInferSubscriptionTotalEpisodesFromSearchAndRSS(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p"},
		{Title: "Some Show S01E12 1080p"},
		{Title: "Other Show S01E99 1080p"},
	}
	if got := inferSearchTotalEpisodes(results, sub); got != 12 {
		t.Fatalf("search inferred total = %d, want 12", got)
	}
	subtitleResults := []SearchResult{
		{Title: "Smoking Behind the Supermarket with You", Subtitle: "躲在超市后门抽烟的两人 S01E12"},
	}
	subtitleSub := &model.Subscription{Name: "躲在超市后门抽烟的两人 自动订阅", Filter: "躲在超市后门抽烟的两人", MediaType: "tv"}
	if got := inferSearchTotalEpisodes(subtitleResults, subtitleSub); got != 12 {
		t.Fatalf("subtitle search inferred total = %d, want 12", got)
	}
	items := []rssItem{
		{Title: "Some Show S01E02 WEB-DL"},
		{Title: "Some Show S01E10 WEB-DL"},
	}
	if got := inferRSSTotalEpisodes(items, sub, compileFilter("Some Show")); got != 10 {
		t.Fatalf("rss inferred total = %d, want 10", got)
	}
}

func TestResolveSubscriptionTotalEpisodesPrefersTMDbOverTitleFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Some Show","first_air_date":"2026-01-01"}]}`))
		case "/tv/42":
			_, _ = w.Write([]byte(`{"number_of_episodes":13}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
	svc := NewSubscriptionService(cfg, zap.NewNop(), nil, nil, nil, NewHub(zap.NewNop()))
	svc.SetScraper(NewScraperService(cfg, zap.NewNop(), nil, tmdb, nil, nil, nil, NewHub(zap.NewNop())))

	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	if got := svc.resolveSubscriptionTotalEpisodes(t.Context(), sub, 10); got != 13 {
		t.Fatalf("resolved total = %d, want TMDb total 13", got)
	}
}

func TestSubscriptionArchiveKeepsWashSubscriptionActive(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:        "Dune 自动订阅",
		FeedURL:     "site-search://search?keyword=Dune",
		Filter:      "Dune 2021",
		MediaType:   "movie",
		WashEnabled: true,
		Enabled:     true,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}

	if err := svc.archiveCompletedSubscription(t.Context(), sub, LocalAvailability{
		DownloadedEpisodes: 1,
		LocalMediaCount:    1,
		InLibrary:          true,
	}); err != nil {
		t.Fatal(err)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("active subscriptions = %d, want wash subscription to stay active", len(active))
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history subscriptions = %d, want 0", len(history))
	}
}

func TestRestoreArchivedSubscriptionReturnsToActiveAndClearsSeenState(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		FeedURL:       "https://rss.example/feed",
		Filter:        "南部档案",
		MediaType:     "tv",
		TotalEpisodes: 33,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	archivedAt := time.Now()
	if err := repos.Subscription.Archive(t.Context(), sub.ID, "已下载 1/33 集，缺 33 集", archivedAt); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "subscription."+sub.ID+".seen", "old-guid"); err != nil {
		t.Fatal(err)
	}
	restored, err := svc.Restore(t.Context(), sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ArchivedAt != nil || restored.ArchiveReason != "" || !restored.Enabled {
		t.Fatalf("restored subscription not active: archived=%v reason=%q enabled=%v", restored.ArchivedAt, restored.ArchiveReason, restored.Enabled)
	}
	if restored.TotalEpisodes != 0 {
		t.Fatalf("restored total_episodes = %d, want 0 so it gets recomputed from authoritative metadata", restored.TotalEpisodes)
	}
	active, err := repos.Subscription.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != sub.ID {
		t.Fatalf("active subscriptions = %#v, want restored subscription", active)
	}
	history, err := repos.Subscription.History(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history subscriptions = %d, want 0 after restore", len(history))
	}
	seen, err := repos.Setting.Get(t.Context(), "subscription."+sub.ID+".seen")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("seen state = %q, want cleared", seen)
	}
}

func TestSubscriptionRunOneDeduplicatesDuplicateRSSGUIDInSameFeed(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1</guid>
    <link>magnet:?xt=urn:btih:1111111111111111111111111111111111111111&amp;dn=Some+Show+S01E01</link>
  </item>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1</guid>
    <link>magnet:?xt=urn:btih:1111111111111111111111111111111111111111&amp;dn=Some+Show+S01E01</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Some Show S01E01 1080p","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:      "Some Show 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Some Show",
		MediaType: "tv",
		SavePath:  "/downloads/tv",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("download rows = %d, want 1", len(rows))
	}
}

func TestSubscriptionRunOneSkipsSameEpisodeAddedEarlierInFeed(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Some Show S01E01 1080p</title>
    <guid>episode-1-a</guid>
    <link>magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&amp;dn=Some+Show+S01E01+1080p</link>
  </item>
  <item>
    <title>Some Show S01E01 WEB-DL</title>
    <guid>episode-1-b</guid>
    <link>magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb&amp;dn=Some+Show+S01E01+WEB-DL</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if atomic.LoadInt32(&addCalls) > 0 {
				_, _ = w.Write([]byte(`[{"hash":"abc123","name":"Some Show S01E01 1080p","state":"downloading","progress":0.1}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			atomic.AddInt32(&addCalls, 1)
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:          "Some Show 自动订阅",
		FeedURL:       rss.URL,
		Filter:        "Some Show",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 12,
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("download rows = %d, want 1", len(rows))
	}
}

func TestSubscriptionRunOneRSSWashQueuesOnlyBestMovieVariant(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Dune 2021 1080p WEB-DL</title>
    <guid>dune-1080-web</guid>
    <link>magnet:?xt=urn:btih:dddddddddddddddddddddddddddddddddddddddd&amp;dn=Dune+2021+1080p+WEB-DL</link>
  </item>
  <item>
    <title>Dune 2021 2160p UHD BluRay REMUX HDR</title>
    <guid>dune-2160-remux</guid>
    <link>magnet:?xt=urn:btih:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee&amp;dn=Dune+2021+2160p+REMUX</link>
  </item>
  <item>
    <title>Dune 2021 720p HDTV</title>
    <guid>dune-720-hdtv</guid>
    <link>magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff&amp;dn=Dune+2021+720p+HDTV</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var addCalls int32
	var addedTitles []string
	addedHashes := make([]string, 0, 3)
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			if len(addedHashes) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			var items []string
			for _, hash := range addedHashes {
				items = append(items, `{"hash":"`+hash+`","name":"Dune 2021","state":"downloading","progress":0.1}`)
			}
			_, _ = w.Write([]byte(`[` + strings.Join(items, ",") + `]`))
		case "/api/v2/torrents/add":
			call := atomic.AddInt32(&addCalls, 1)
			_ = r.ParseMultipartForm(10 << 20)
			addedTitles = append(addedTitles, r.FormValue("urls"))
			addedHashes = append(addedHashes, strings.Repeat(fmt.Sprintf("%x", call), 40))
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))

	sub := &model.Subscription{
		Name:         "Dune 自动订阅",
		FeedURL:      rss.URL,
		Filter:       "Dune 2021",
		MediaType:    "movie",
		WashEnabled:  true,
		WashPriority: "resolution",
		SavePath:     "/downloads/movies",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("queued = %d, want 1 best movie variant", queued)
	}
	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("qb add calls = %d, want 1", got)
	}
	if len(addedTitles) != 1 || !strings.Contains(addedTitles[0], "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		t.Fatalf("added %#v, want 2160p REMUX variant only", addedTitles)
	}
}

func TestSubscriptionRunOneDoesNotUseDeletedDownloader(t *testing.T) {
	rss := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss><channel>
  <item>
    <title>Deleted Downloader Show S01E01 1080p</title>
    <guid>deleted-downloader-episode-1</guid>
    <link>magnet:?xt=urn:btih:cccccccccccccccccccccccccccccccccccccccc&amp;dn=Deleted+Downloader+Show+S01E01</link>
  </item>
</channel></rss>`))
	}))
	defer rss.Close()

	var qbCalls int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&qbCalls, 1)
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.Setting{}, &model.DownloadTask{}, &model.Media{}, &model.DownloadClient{})
	repos := repository.New(db)
	client := &model.DownloadClient{Name: "qB deleted", Type: "qbittorrent", Host: qb.URL, Username: "admin", Password: "admin", IsDefault: true, Enabled: true}
	if err := repos.DownloadClient.Create(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), settingDownloadClientsManaged, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.DownloadClient.Delete(t.Context(), client.ID); err != nil {
		t.Fatal(err)
	}

	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Name:      "Deleted Downloader Show 自动订阅",
		FeedURL:   rss.URL,
		Filter:    "Deleted Downloader Show",
		MediaType: "tv",
		SavePath:  "/downloads/tv",
	}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}

	queued, err := svc.runOne(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("queued = %d, want 0 when default downloader was deleted", queued)
	}
	if got := atomic.LoadInt32(&qbCalls); got != 0 {
		t.Fatalf("qB calls = %d, want 0 after downloader deletion", got)
	}
	rows, err := repos.Download.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("download rows = %d, want 0", len(rows))
	}
}
