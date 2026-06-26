package service

import (
	"fmt"
	"strconv"
	"strings"
)

func mergeManualRequestIntoMatch(match *Match, req ManualScrapeRequest) *Match {
	if match == nil {
		match = &Match{}
	}
	if req.Title != "" {
		match.Title = req.Title
	}
	if mediaType := normalizeOrganizeMediaType(req.MediaType); mediaType != "" {
		match.MediaType = mediaType
	}
	if req.OriginalName != "" {
		match.OriginalName = req.OriginalName
	}
	if req.Overview != "" {
		match.Overview = req.Overview
	}
	if req.PosterURL != "" {
		match.PosterURL = req.PosterURL
	}
	if req.BackdropURL != "" {
		match.BackdropURL = req.BackdropURL
	}
	if req.Year > 0 {
		match.Year = req.Year
	}
	if req.Rating > 0 {
		match.Rating = req.Rating
	}
	if req.TMDbID > 0 {
		match.TMDbID = req.TMDbID
	}
	if req.BangumiID > 0 {
		match.BangumiID = req.BangumiID
	}
	if req.DoubanID != "" {
		match.DoubanID = req.DoubanID
	}
	if req.TheTVDBID != "" {
		match.TheTVDBID = req.TheTVDBID
	}
	if len(req.Genres) > 0 {
		match.Genres = req.Genres
	}
	if len(req.Countries) > 0 {
		match.Countries = req.Countries
	}
	if len(req.Languages) > 0 {
		match.Languages = req.Languages
	}
	if req.NSFW {
		match.NSFW = true
	}
	return match
}

func isTVLikeTMDbMatch(match *Match, mediaType string) bool {
	return mediaType == "tv" || mediaType == "anime" || mediaType == "variety"
}

func parsePositiveInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ":") {
		value = value[strings.LastIndex(value, ":")+1:]
	}
	id, err := strconv.Atoi(strings.TrimSpace(value))
	return id, err == nil && id > 0
}

func parsePositiveIDString(value string) (string, bool) {
	id, ok := parsePositiveInt(value)
	if !ok {
		return "", false
	}
	return strconv.Itoa(id), true
}

func manualScrapeBatchName(ids []string) string {
	if len(ids) == 1 {
		return ids[0]
	}
	return fmt.Sprintf("%d 个媒体", len(ids))
}
