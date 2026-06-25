package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMatchesSubscriptionRulesUserExcludeWords(t *testing.T) {
	sub := &model.Subscription{ExcludeWords: "10bit,dolby vision,杜比"}
	cases := []struct {
		title string
		want  bool
	}{
		{"Movie 2024 1080p WEB-DL", true},
		{"Movie 2024 2160p 10bit HEVC", false},
		{"Movie 2024 2160p Dolby Vision", false},
		{"电影 2024 杜比全景声", false},
	}
	for _, c := range cases {
		if got := matchesSubscriptionRules(sub, c.title); got != c.want {
			t.Errorf("matchesSubscriptionRules(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestMatchesSubscriptionRulesDefaultExcludesJunkReleases(t *testing.T) {
	sub := &model.Subscription{}
	for _, title := range []string{
		"Some Movie 2024 CAM",
		"Some Movie 2024 HDTS",
		"某电影 2024 枪版",
		"Some Movie 2024 TELESYNC",
		"Some Show 预告",
	} {
		if matchesSubscriptionRules(sub, title) {
			t.Errorf("expected default rules to exclude junk release %q", title)
		}
	}
}

func TestMatchesSubscriptionRulesWordBoundaryAvoidsFalsePositives(t *testing.T) {
	sub := &model.Subscription{}
	// "ts" / "cam" / "tc" 作为子串出现在合法标题里时不应被默认排除误伤。
	for _, title := range []string{
		"Tsukihime 2024 1080p WEB-DL",
		"Camp Rock 2024 1080p BluRay",
		"Catch Me 2024 1080p WEB-DL",
	} {
		if !matchesSubscriptionRules(sub, title) {
			t.Errorf("word-boundary match wrongly excluded %q", title)
		}
	}
}

func TestSelectSiteSearchCandidatesSkipsExistingMovieWhenNotWashing(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Inception 2010 2160p 10bit Dolby Vision Atmos", DownloadURL: "https://pt/download/dovi", Seeders: 500},
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/web", Seeders: 90},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none (movie already in library, wash disabled)", got)
	}
}

func TestSelectSiteSearchCandidatesAllowsMovieWashUpgrade(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashEnabled: true, WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "Inception 2010 2160p REMUX", DownloadURL: "https://pt/download/2160", Seeders: 80},
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/1080", Seeders: 200},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/2160" {
		t.Fatalf("selected %#v, want 2160p upgrade allowed when washing", got)
	}
}

func TestSubscriptionItemAlreadyAvailable(t *testing.T) {
	movieSub := &model.Subscription{MediaType: "movie"}
	if !subscriptionItemAlreadyAvailable(movieSub, LocalAvailability{LocalMediaCount: 1}, "Inception 2010 2160p") {
		t.Fatal("movie already in library should be reported available")
	}
	if subscriptionItemAlreadyAvailable(movieSub, LocalAvailability{}, "Inception 2010 2160p") {
		t.Fatal("empty library should not be reported available")
	}
	tvSub := &model.Subscription{MediaType: "tv"}
	avail := LocalAvailability{LocalMediaCount: 1, ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 2): {}}}
	if !subscriptionItemAlreadyAvailable(tvSub, avail, "Show S01E02 1080p") {
		t.Fatal("existing episode should be reported available")
	}
	if subscriptionItemAlreadyAvailable(tvSub, avail, "Show S01E03 1080p") {
		t.Fatal("missing episode should not be reported available")
	}
}
