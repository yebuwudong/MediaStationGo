package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
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

func TestDeleteSubscriptionRemovesDownloaderTaskAndSeenState(t *testing.T) {
	const title = "Delete Subscription Show S01E01 1080p"
	const hash = "abcdef1234567890abcdef1234567890abcdef12"
	var deleteCalls atomic.Int32
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"hash":"` + hash + `","name":"` + title + `","state":"downloading","progress":0.2}]`))
		case "/api/v2/torrents/delete":
			deleteCalls.Add(1)
			if got := r.FormValue("deleteFiles"); got != "false" {
				t.Fatalf("deleteFiles = %q, want false", got)
			}
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	db := newServiceTestDB(t, &model.Subscription{}, &model.DownloadTask{}, &model.DownloadClient{}, &model.Setting{})
	repos := repository.New(db)
	configureTestDefaultQB(t, repos, qb.URL)
	downloads := NewDownloadService(zap.NewNop(), repos, NewHub(zap.NewNop()), nil)
	if err := downloads.ReloadConfig(t.Context()); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, zap.NewNop(), repos, downloads, nil, NewHub(zap.NewNop()))
	sub := &model.Subscription{Name: "Delete Subscription Show 自动订阅", Filter: "Delete Subscription Show", FeedURL: "https://rss.example/feed", UserID: "u1", SavePath: "/downloads/tv"}
	if err := repos.Subscription.Create(t.Context(), sub); err != nil {
		t.Fatal(err)
	}
	task := &model.DownloadTask{
		UserID:         "u1",
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt.example/download?id=1",
		Title:          title,
		SavePath:       "/downloads/tv",
		Status:         "downloading",
		Progress:       0.2,
	}
	if err := repos.Download.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "subscription."+sub.ID+".seen", "guid-1"); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(t.Context(), sub.ID); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	if got := deleteCalls.Load(); got != 1 {
		t.Fatalf("qb delete calls = %d, want 1", got)
	}
	var updated model.DownloadTask
	if err := db.Where("id = ?", task.ID).First(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "deleted" {
		t.Fatalf("download task status = %q, want deleted", updated.Status)
	}
	seen, err := repos.Setting.Get(t.Context(), "subscription."+sub.ID+".seen")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "" {
		t.Fatalf("seen state = %q, want cleared", seen)
	}
	var count int64
	if err := db.Model(&model.Subscription{}).Where("id = ?", sub.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("active subscription count = %d, want 0", count)
	}
}

func TestDeletedDownloadTaskDoesNotBlockSubscriptionReadd(t *testing.T) {
	if downloadTaskBlocksReadd("deleted") {
		t.Fatal("deleted download task must not block subscription re-add")
	}
	if downloadTaskBlocksReadd("removed") {
		t.Fatal("removed download task must not block subscription re-add")
	}
}

func TestSelectSiteSearchCandidatesOnlyQueuesMissingLocalEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家", MediaType: "tv", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     2,
		MissingEpisodes:     []int{3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only missing episode 3", got)
	}
}

func TestSelectSiteSearchCandidatesWithUnknownTotalSkipsExistingEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime"}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "葬送的芙莉莲 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "葬送的芙莉莲 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	availability := LocalAvailability{
		LocalMediaCount:     2,
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-local episode 3", got)
	}
}

func TestSelectSiteSearchCandidatesSingleExistingEpisodeIsSkipped(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{2, 3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because E01 already exists", got)
	}
}

func TestSelectSiteSearchCandidatesFullPackUsedAsFallbackWhenLibraryPartiallyExists(t *testing.T) {
	// 本地缺第 3 集,站点只有整季全集包(无单集种)。剧集完结后站点常只挂全集包,
	// 此时必须用全集包兜底补缺集,否则"补全缺失集"永远匹配为空(用户报告的 bug)。
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家", MediaType: "tv", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     2,
		MissingEpisodes:     []int{3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 {
		t.Fatalf("selected %#v, want the full pack as fallback to cover missing episode 3", got)
	}
	if got[0].Download != "https://pt/download/pack" {
		t.Fatalf("selected %#v, want the Complete pack", got)
	}
}

func TestSelectSiteSearchCandidatesMissingEpisodeCanMatchSubtitleAlias(t *testing.T) {
	sub := &model.Subscription{Name: "躲在超市后门抽烟的两人 自动订阅", Filter: "躲在超市后门抽烟的两人", MediaType: "tv", TotalEpisodes: 12}
	results := []SearchResult{
		{Title: "Smoking Behind the Supermarket with You S01E01 1080p", Subtitle: "躲在超市后门抽烟的两人", DownloadURL: "https://pt/download/1", Seeders: 100},
		{Title: "Smoking Behind the Supermarket with You S01E12 1080p", Subtitle: "躲在超市后门抽烟的两人", DownloadURL: "https://pt/download/12", Seeders: 80},
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 11; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	availability := LocalAvailability{
		TotalEpisodes:       12,
		LocalMediaCount:     11,
		MissingEpisodes:     []int{12},
		ExistingEpisodeKeys: existing,
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 12 || got[0].Download != "https://pt/download/12" {
		t.Fatalf("selected %#v, want subtitle-matched missing episode 12", got)
	}
}

func TestSelectSiteSearchCandidatesRelaxesQueryForExistingSeriesMissingEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "翘楚 S01E06 自动订阅", Filter: "翘楚 S01E06", MediaType: "tv", TotalEpisodes: 24}
	results := []SearchResult{
		{Title: "Qiao Chu 2026 S01E06 2160p WEB-DL", DownloadURL: "https://pt/download/6", Seeders: 10},
		{Title: "Ashes to Crown 2026 S01E21 2160p WEB-DL", DownloadURL: "https://pt/download/21", Seeders: 8},
		{Title: "Ashes to Crown 2026 S01E99 2160p WEB-DL", DownloadURL: "https://pt/download/99", Seeders: 99},
	}
	availability := LocalAvailability{
		TotalEpisodes:       24,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{21},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 6): {}},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 21 || got[0].Download != "https://pt/download/21" {
		t.Fatalf("selected %#v, want relaxed alias-like missing episode 21 only", got)
	}
	if stats.QueryMismatch != 3 || stats.RelaxedQueryMatch != 3 || stats.ExistingEpisodeSkipped != 1 || stats.NotMissingEpisodeSkipped != 1 {
		t.Fatalf("unexpected relaxed stats: %#v", stats)
	}
}

func TestAddSiteSearchCandidateAvailabilityTracksRelaxedAliasCandidate(t *testing.T) {
	sub := &model.Subscription{Name: "翘楚 S01E06 自动订阅", Filter: "翘楚 S01E06", MediaType: "tv", TotalEpisodes: 24}
	availability := LocalAvailability{
		TotalEpisodes:       24,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{21},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 6): {}},
		MissingEpisodeKeys:  map[string]struct{}{episodeKey(1, 21): {}},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Ashes to Crown 2026 S01E21 2160p WEB-DL",
			DownloadURL: "https://pt/download/21",
		},
		Download: "https://pt/download/21",
		GUID:     "site|m-team|ashes-to-crown-21",
		Season:   1,
		Episode:  21,
	}

	addSiteSearchCandidateAvailability(candidate, &availability)
	availability = NewSubscriptionService(nil, nil, nil, nil, nil, nil).finalizePendingAvailability(sub, availability)

	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 21)]; !ok {
		t.Fatalf("missing relaxed alias candidate E21 key: %#v", availability.ExistingEpisodeKeys)
	}
	got := selectSiteSearchCandidates([]SearchResult{candidate.Item}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want relaxed alias candidate skipped after dedup availability update", got)
	}
}

func TestSelectSiteSearchCandidatesDoesNotRelaxQueryForMovies(t *testing.T) {
	sub := &model.Subscription{Name: "玩具总动员 5 自动订阅", Filter: "玩具总动员 5 2026", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Toy Story 4 2019 2160p WEB-DL", DownloadURL: "https://pt/download/wrong", Seeders: 100},
	}
	availability := LocalAvailability{}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want no relaxed movie match", got)
	}
	if stats.QueryMismatch != 1 || stats.RelaxedQueryMatch != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestSelectSiteSearchCandidatesSingleExistingMovieIsSkippedWhenNotWashing(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/web", Seeders: 90},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because movie already exists and wash is disabled", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilitySkipsUnorganizedEpisodes(t *testing.T) {
	root := t.TempDir()
	seasonDir := filepath.Join(root, "间谍过家家", "Season 01")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"间谍过家家 - S01E01.mkv",
		"间谍过家家 - S01E02.mkv.!qB",
	} {
		if err := os.WriteFile(filepath.Join(seasonDir, name), []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sub := &model.Subscription{
		Name:          "间谍过家家 自动订阅",
		Filter:        "间谍过家家",
		MediaType:     "tv",
		SavePath:      root,
		TotalEpisodes: 3,
	}
	svc := NewSubscriptionService(nil, nil, nil, nil, nil, nil)
	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if availability.DownloadedEpisodes != 2 {
		t.Fatalf("downloaded episodes = %d, want 2", availability.DownloadedEpisodes)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 1)]; !ok {
		t.Fatalf("missing pending E01 key: %#v", availability.ExistingEpisodeKeys)
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; !ok {
		t.Fatalf("missing pending E02 key: %#v", availability.ExistingEpisodeKeys)
	}

	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-downloaded episode 3", got)
	}
	if !svc.downloadPathHasCandidate(t.Context(), sub, "间谍过家家 S01E02 1080p", root) {
		t.Fatal("expected existing pending E02 file to be detected")
	}
	if svc.downloadPathHasCandidate(t.Context(), sub, "间谍过家家 S01E03 1080p", root) {
		t.Fatal("did not expect missing E03 to be detected")
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesQueuedTasks(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		Source:   "qbittorrent",
		URL:      "magnet:?xt=urn:btih:2222222222222222222222222222222222222222",
		Title:    "间谍过家家 S01E02 1080p",
		SavePath: "/downloads/tv",
		Status:   "queued",
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
	if availability.DownloadedEpisodes != 1 {
		t.Fatalf("downloaded episodes = %d, want 1", availability.DownloadedEpisodes)
	}
	if availability.InLibrary {
		t.Fatal("queued download should not be reported as already in library")
	}
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 2)]; !ok {
		t.Fatalf("missing queued E02 key: %#v", availability.ExistingEpisodeKeys)
	}

	results := []SearchResult{
		{Title: "间谍过家家 S01E02 1080p WEB-DL", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p WEB-DL", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-downloaded episode 3", got)
	}
}

func TestSubscriptionPendingDownloadAvailabilityIncludesLinkedAliasTask(t *testing.T) {
	db := newServiceTestDB(t, &model.DownloadTask{})
	repos := repository.New(db)
	sub := &model.Subscription{
		Base:          model.Base{ID: "sub-qiao-chu"},
		Name:          "翘楚 S01E06 自动订阅",
		Filter:        "翘楚 S01E06",
		MediaType:     "tv",
		SavePath:      "/downloads/tv",
		TotalEpisodes: 24,
	}
	if err := repos.Download.Create(t.Context(), &model.DownloadTask{
		SubscriptionID: sub.ID,
		Source:         "qbittorrent",
		URL:            "https://pt/download/21",
		Title:          "Ashes to Crown 2026 S01E21 2160p WEB-DL",
		SavePath:       "/downloads/tv",
		Status:         "queued",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(nil, nil, repos, nil, nil, nil)

	availability := svc.pendingDownloadAvailability(t.Context(), sub)
	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 21)]; !ok {
		t.Fatalf("missing linked alias E21 key: %#v", availability.ExistingEpisodeKeys)
	}
	got := selectSiteSearchCandidates([]SearchResult{
		{Title: "Ashes to Crown 2026 S01E21 2160p WEB-DL", DownloadURL: "https://pt/download/21", Seeders: 80},
	}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want linked alias task to satisfy E21", got)
	}
}
