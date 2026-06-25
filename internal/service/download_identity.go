package service

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var torrentEpisodeToken = regexp.MustCompile(`(?i)e\d{1,3}`)

func localAvailabilityTitleCandidates(title string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 6)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(availabilityQuery(title, ""))
	if cleaned, _ := CleanQuery(title); cleaned != "" {
		for _, candidate := range titleCandidates(cleaned) {
			add(candidate)
			fields := strings.Fields(candidate)
			for i := len(fields) - 1; i >= 1; i-- {
				prefix := strings.Join(fields[:i], " ")
				if containsCJK(prefix) {
					add(prefix)
				}
			}
		}
	}
	return out
}

func downloadTaskBlocksDuplicate(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func downloadTaskBlocksReadd(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "deleted", "removed", "cancelled", "canceled":
		return false
	default:
		return true
	}
}

func downloadTaskIdentityKey(name string) string {
	if key := downloadMediaIdentityKey(name); key != "" {
		return key
	}
	return normalizedDownloadTitleKey(name)
}

func downloadMediaIdentityKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	title, year := CleanQuery(name)
	titleKey := normalizeAvailabilityComparable(title)
	if titleKey == "" {
		titleKey = normalizeAvailabilityComparable(availabilityQuery(name, ""))
	}
	if titleKey == "" {
		return ""
	}
	season, episode := ParseEpisode(name)
	parts := []string{titleKey}
	if year > 0 {
		parts = append(parts, fmt.Sprintf("y%d", year))
	}
	if episode > 0 {
		if season <= 0 {
			season = 1
		}
		parts = append(parts, fmt.Sprintf("s%02de%03d", season, episode))
	}
	return strings.Join(parts, "|")
}

func normalizedDownloadTitleKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func publicDownloadTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "下载任务"
	}
	if u, err := url.Parse(raw); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			if decoded, err := url.QueryUnescape(dn); err == nil && strings.TrimSpace(decoded) != "" {
				return strings.TrimSpace(decoded)
			}
			return dn
		}
		if u.Host != "" {
			base := path.Base(u.Path)
			if base != "." && base != "/" && base != "" {
				base = strings.TrimSuffix(base, path.Ext(base))
				if base != "" {
					return base
				}
			}
			return u.Host
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "磁力下载"
	}
	return "下载任务"
}

func normalizeTorrentName(name string) string {
	name = torrentEpisodeToken.ReplaceAllString(strings.ToLower(name), "")
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
