package service

import (
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectSiteSearchCandidatesPrefersSeriesPack(t *testing.T) {
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家 2022", MediaType: "tv"}
	results := []SearchResult{
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 80},
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 50},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 70},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 {
		t.Fatalf("selected %d candidates, want 1", len(got))
	}
	if got[0].Download != "https://pt/download/pack" || !got[0].Pack {
		t.Fatalf("selected %#v, want complete pack", got[0])
	}
}

func TestSelectSiteSearchCandidatesQueuesDistinctEpisodesWhenNoPack(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime", WashEnabled: true, WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1a", Seeders: 90},
		{Title: "葬送的芙莉莲 S01E01 2160p", DownloadURL: "https://pt/download/1b", Seeders: 80},
		{Title: "葬送的芙莉莲 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 70},
		{Title: "葬送的芙莉莲 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 60},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 3 {
		t.Fatalf("selected %d candidates, want 3", len(got))
	}
	if got[0].Episode != 1 || got[1].Episode != 2 || got[2].Episode != 3 {
		t.Fatalf("episodes = %d,%d,%d; want 1,2,3", got[0].Episode, got[1].Episode, got[2].Episode)
	}
	if got[0].Download != "https://pt/download/1b" {
		t.Fatalf("duplicate episode should keep wash-priority best result, got %q", got[0].Download)
	}
}

func TestSelectSiteSearchCandidatesKeepsMovieSingleBest(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p", DownloadURL: "https://pt/download/1080", Seeders: 90},
		{Title: "Inception 2010 2160p", DownloadURL: "https://pt/download/2160", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/1080" {
		t.Fatalf("selected %#v, want movie best only", got)
	}
}

func TestSelectSiteSearchCandidatesRejectsUnrelatedHighSeederResult(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Unrelated Movie 2026 2160p", DownloadURL: "https://pt/download/wrong", Seeders: 999},
		{Title: "Inception 2010 1080p", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want title-matched result only", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesTranslatedSubtitle(t *testing.T) {
	sub := &model.Subscription{Name: "真人快打2 自动订阅", Filter: "真人快打2 2026", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Unrelated Movie 2026 2160p", DownloadURL: "https://pt/download/wrong", Seeders: 999},
		{Title: "Mortal Kombat II 2026 1080p WEB-DL", Subtitle: "真人快打2", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want translated subtitle match", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesFeedAlias(t *testing.T) {
	sub := &model.Subscription{
		Name:      "真人快打2 自动订阅",
		FeedURL:   "site-search://search?keyword=%E7%9C%9F%E4%BA%BA%E5%BF%AB%E6%89%932%202026&alias=Mortal%20Kombat%20II%202026",
		Filter:    "真人快打2 2026",
		MediaType: "movie",
	}
	results := []SearchResult{
		{Title: "Mortal Kombat II 2026 1080p WEB-DL", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want alias-matched result", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesSubscriptionOriginalNameAlias(t *testing.T) {
	sub := &model.Subscription{
		Name:         "玩具总动员 5 自动订阅",
		Filter:       "玩具总动员 5 2026",
		OriginalName: "Toy Story 5",
		Year:         2026,
		MediaType:    "movie",
	}
	results := []SearchResult{
		{Title: "Toy Story 5 2026 1080p WEB-DL", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want original-name alias match", got)
	}
}

func TestSelectSiteSearchCandidatesDoesNotWashByDefault(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p", DownloadURL: "https://pt/download/1080", Seeders: 90},
		{Title: "Inception 2010 2160p", DownloadURL: "https://pt/download/2160", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/1080" {
		t.Fatalf("selected %#v, want seeders best when wash disabled", got)
	}
}

func TestSelectSiteSearchCandidatesAppliesQualityRules(t *testing.T) {
	sub := &model.Subscription{
		Name:         "Dune 自动订阅",
		Filter:       "Dune 2021",
		MediaType:    "movie",
		Resolution:   "2160p",
		Quality:      "remux",
		Effects:      "hdr",
		ExcludeWords: "cam,ts",
	}
	results := []SearchResult{
		{Title: "Dune 2021 2160p WEB-DL HDR", DownloadURL: "https://pt/download/web", Seeders: 100},
		{Title: "Dune 2021 2160p UHD BluRay REMUX HDR", DownloadURL: "https://pt/download/remux", Seeders: 30},
		{Title: "Dune 2021 2160p REMUX HDR CAM", DownloadURL: "https://pt/download/cam", Seeders: 200},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/remux" {
		t.Fatalf("selected %#v, want filtered remux", got)
	}
}

func TestSiteSearchKeywordCanUseIMDB(t *testing.T) {
	sub := &model.Subscription{Name: "沙丘 自动订阅", Filter: "Dune 2021", SearchMode: "imdb", IMDBID: "tt1160419"}
	if got := siteSearchKeyword(sub); got != "tt1160419" {
		t.Fatalf("keyword = %q, want imdb id", got)
	}
}

func TestSiteSearchKeywordsIncludeAliasesAndCleanedKeywords(t *testing.T) {
	sub := &model.Subscription{
		Name:    "真人快打2 自动订阅",
		FeedURL: "site-search://search?keyword=%E7%9C%9F%E4%BA%BA%E5%BF%AB%E6%89%932%202026&alias=Mortal%20Kombat%20II%202026",
		Filter:  "真人快打2 2026",
	}

	got := siteSearchKeywords(sub)
	for _, want := range []string{"真人快打2 2026", "Mortal Kombat II 2026", "真人快打2", "Mortal Kombat II"} {
		if !containsString(got, want) {
			t.Fatalf("keywords = %#v, missing %q", got, want)
		}
	}
	if got[0] != "真人快打2 2026" {
		t.Fatalf("primary keyword = %q, want feed keyword first", got[0])
	}
}

func TestSiteSearchKeywordsUseCleanMetadataAliases(t *testing.T) {
	sub := &model.Subscription{
		Name:         "玩具总动员 4 自动订阅",
		Filter:       "玩具总动员 4 2019",
		OriginalName: "Toy Story 4",
		Year:         2019,
	}

	got := siteSearchKeywords(sub)
	for _, want := range []string{"玩具总动员 4 2019", "Toy Story 4", "Toy Story 4 2019", "玩具总动员 4"} {
		if !containsString(got, want) {
			t.Fatalf("keywords = %#v, missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"玩具总动员 4 自动订阅", "玩具总动员 4 自动订阅 2019", "玩具总动员 4 2019 2019"} {
		if containsString(got, unwanted) {
			t.Fatalf("keywords = %#v, should not contain %q", got, unwanted)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestStableSiteSearchGUIDIgnoresPrivateTokenChanges(t *testing.T) {
	item := SearchResult{
		SiteID:   "mteam",
		Title:    "Some Show S01E01 1080p",
		Category: "TV",
		Size:     1024,
	}
	first := stableSiteSearchGUID(item, "https://pt.example/download?id=123&passkey=old")
	second := stableSiteSearchGUID(item, "https://pt.example/download?id=123&passkey=new")
	if first != second {
		t.Fatalf("stableSiteSearchGUID changed with token: %q != %q", first, second)
	}
	if strings.Contains(first, "passkey") || strings.Contains(first, "old") || strings.Contains(first, "new") {
		t.Fatalf("stableSiteSearchGUID leaked private token: %q", first)
	}
}

func TestSelectSiteSearchCandidatesWithStatsExplainsFiltering(t *testing.T) {
	sub := &model.Subscription{Name: "Stats Show 自动订阅", Filter: "Stats Show", MediaType: "tv"}
	seenItem := SearchResult{Title: "Stats Show S01E02 1080p", DownloadURL: "https://pt/download/seen", Seeders: 50}
	seenGUID := stableSiteSearchGUID(seenItem, seenItem.DownloadURL)
	results := []SearchResult{
		{Title: "Different Show S01E01 1080p", DownloadURL: "https://pt/download/wrong", Seeders: 90},
		{Title: "Stats Show S01E01 CAM", DownloadURL: "https://pt/download/cam", Seeders: 80},
		{Title: "Stats Show S01E02 1080p", Seeders: 70},
		seenItem,
		{Title: "Stats Show S01E03 1080p", DownloadURL: "https://pt/download/right", Seeders: 60},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{seenGUID: {}}, LocalAvailability{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want only unfiltered candidate", got)
	}
	if stats.Total != 5 ||
		stats.QueryMismatch != 1 ||
		stats.RuleMismatch != 1 ||
		stats.MissingDownload != 1 ||
		stats.Seen != 1 ||
		stats.Prepared != 1 ||
		stats.Selected != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if len(stats.QueryMismatchExamples) != 1 || stats.QueryMismatchExamples[0] != "Different Show S01E01 1080p" {
		t.Fatalf("query mismatch examples = %#v", stats.QueryMismatchExamples)
	}
}
