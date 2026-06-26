package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEnqueueSiteSearchDedupMarksEnglishRangeAvailableForChineseSubscription(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		UserID:   "u1",
		Source:   "qbittorrent",
		URL:      "https://pt.example/download?id=old",
		Title:    "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
		SavePath: "/downloads/国产剧",
		Status:   "queued",
		Progress: 0.1,
	}); err != nil {
		t.Fatal(err)
	}

	site := NewSiteService(zap.NewNop(), repos, "")
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, site, NewHub(zap.NewNop()))
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-nanyang"},
		UserID:        "u1",
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案 2026",
		MediaType:     "tv",
		MediaCategory: "国产剧",
		SavePath:      "/downloads",
		TotalEpisodes: 33,
	}
	state := &siteSearchRunState{
		Keyword: "南部档案 2026",
		SeenSet: map[string]struct{}{},
		Availability: LocalAvailability{
			TotalEpisodes:       33,
			ExistingEpisodeKeys: map[string]struct{}{},
			MissingEpisodeKeys:  map[string]struct{}{},
		},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E07-S01E08 2160p WEB-DL",
			DownloadURL: "https://pt.example/download?id=new",
		},
		Download: "https://pt.example/download?id=new",
		GUID:     "site|m-team|nanyang-7-8",
		Season:   1,
		Episode:  7,
		Episodes: []int{7, 8},
		Pack:     true,
	}

	title, err := svc.enqueueSiteSearchCandidate(t.Context(), sub, candidate, state)
	if err != nil {
		t.Fatalf("enqueueSiteSearchCandidate returned %v, want dedup skip without error", err)
	}
	if title != "" {
		t.Fatalf("title = %q, want empty because candidate was deduped", title)
	}
	for _, episode := range []int{7, 8} {
		if _, ok := state.Availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("availability missing E%d after dedup range: %#v", episode, state.Availability.ExistingEpisodeKeys)
		}
	}
	if _, ok := state.SeenSet[candidate.GUID]; !ok {
		t.Fatalf("seen set missing candidate guid after dedup")
	}
}
