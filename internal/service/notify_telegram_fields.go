package service

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func telegramFirstValue(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := telegramDataString(data, key); value != "" {
			return value
		}
	}
	return ""
}

func telegramMessageFieldValue(message string, keys ...string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	for _, line := range strings.Split(message, "\n") {
		key, value, ok := splitTelegramField(line)
		if !ok {
			continue
		}
		for _, want := range keys {
			if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(want)) {
				return value
			}
		}
	}
	return ""
}

func telegramMediaCategory(data map[string]interface{}) string {
	if category := telegramFirstValue(data, "media_category", "category"); category != "" {
		return category
	}
	switch strings.ToLower(telegramFirstValue(data, "media_type")) {
	case "movie":
		return "电影"
	case "tv", "series", "show":
		return "剧集"
	case "anime":
		return "动漫"
	case "variety":
		return "综艺"
	case "documentary":
		return "纪录片"
	default:
		return telegramFirstValue(data, "media_type")
	}
}

func telegramLanguageName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
	})
	if len(parts) == 0 {
		parts = []string{raw}
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, "[]"))
		if part == "" {
			continue
		}
		lower := strings.ToLower(strings.ReplaceAll(part, "_", "-"))
		name := part
		switch {
		case strings.HasPrefix(lower, "zh") || lower == "cn" || lower == "cmn":
			name = "中文"
		case lower == "en" || strings.HasPrefix(lower, "en-"):
			name = "英语"
		case lower == "ja" || lower == "jp" || strings.HasPrefix(lower, "ja-"):
			name = "日语"
		case lower == "ko" || lower == "kr" || strings.HasPrefix(lower, "ko-"):
			name = "韩语"
		case lower == "fr" || strings.HasPrefix(lower, "fr-"):
			name = "法语"
		case lower == "de" || strings.HasPrefix(lower, "de-"):
			name = "德语"
		case lower == "es" || strings.HasPrefix(lower, "es-"):
			name = "西班牙语"
		case lower == "it" || strings.HasPrefix(lower, "it-"):
			name = "意大利语"
		case lower == "ru" || strings.HasPrefix(lower, "ru-"):
			name = "俄语"
		case lower == "th" || strings.HasPrefix(lower, "th-"):
			name = "泰语"
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return strings.Join(out, "、")
}

func telegramSeasonEpisodeValue(event NotifyEvent) string {
	if value := telegramFirstValue(event.Data, "season_episode", "episode_tag"); value != "" {
		return strings.ToUpper(value)
	}
	for _, raw := range []string{
		telegramFirstValue(event.Data, "resource_title", "torrent_title", "release_title"),
		telegramFirstValue(event.Data, "title", "name"),
		event.Message,
	} {
		if value := telegramExtractSeasonEpisode(raw); value != "" {
			return value
		}
	}
	season := telegramFirstValue(event.Data, "season")
	episode := telegramFirstValue(event.Data, "episode")
	if season != "" && episode != "" {
		return fmt.Sprintf("S%02dE%02d", telegramEpisodeNumber(season), telegramEpisodeNumber(episode))
	}
	return ""
}

func telegramEpisodeNumber(raw string) int {
	raw = strings.TrimSpace(strings.TrimLeft(strings.ToUpper(raw), "SE"))
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return 0
	}
	n, _ := strconv.Atoi(raw)
	return n
}

func telegramExtractSeasonEpisode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.ToUpper(telegramSeasonEpisodePattern.FindString(raw))
}

func telegramSizeValue(data map[string]interface{}) string {
	size := telegramFirstValue(data, "size")
	bitrate := telegramFirstValue(data, "bitrate")
	if size != "" && bitrate != "" {
		return size + " / " + bitrate
	}
	if size != "" {
		return size
	}
	return bitrate
}

func telegramVersionValue(event NotifyEvent, seasonEpisode string) string {
	if version := telegramFirstValue(event.Data, "version", "release_group"); version != "" && !strings.EqualFold(version, "best") {
		return version
	}
	return telegramVersionFromResourceTitle(
		telegramFirstValue(event.Data, "resource_title", "torrent_title", "release_title"),
		seasonEpisode,
		telegramFirstValue(event.Data, "year", "release_year"),
	)
}

func telegramVersionFromResourceTitle(raw, seasonEpisode, year string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	tail := ""
	if seasonEpisode != "" {
		upperRaw := strings.ToUpper(raw)
		upperEpisode := strings.ToUpper(seasonEpisode)
		if idx := strings.Index(upperRaw, upperEpisode); idx >= 0 {
			tail = raw[idx+len(seasonEpisode):]
		}
	}
	if tail == "" && year != "" {
		if idx := strings.LastIndex(raw, year); idx >= 0 {
			tail = raw[idx+len(year):]
		}
	}
	tail = strings.Trim(tail, " \t\r\n._-[]()【】")
	if tail == "" {
		return ""
	}
	tail = strings.TrimSuffix(tail, ".torrent")
	tail = strings.TrimSuffix(tail, ".mkv")
	tail = strings.TrimSuffix(tail, ".mp4")
	tail = strings.Join(strings.Fields(tail), ".")
	if len([]rune(tail)) > 72 {
		tail = string([]rune(tail)[:72]) + "..."
	}
	return tail
}

func telegramGenresValue(raw string) string {
	raw = strings.TrimSpace(strings.Trim(raw, "[]"))
	if raw == "" {
		return ""
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '/' || r == '|' || r == '、'
	})
	if len(parts) <= 1 {
		return raw
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, "、")
}

type telegramDataField struct {
	key   string
	value string
}

func telegramDisplayData(data map[string]interface{}) []telegramDataField {
	if len(data) == 0 {
		return nil
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		if telegramHiddenDataKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]telegramDataField, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(data[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		fields = append(fields, telegramDataField{key: key, value: value})
	}
	return fields
}

func telegramHiddenDataKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "photo_url", "poster_url", "poster", "image_url", "backdrop_url",
		"tmdb_url", "imdb_url", "douban_url", "detail_url", "external_url",
		"resource_title", "torrent_title", "release_title":
		return true
	default:
		return false
	}
}

func telegramFieldLabel(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "title", "name":
		return "标题"
	case "original_title":
		return "原始片名"
	case "original_language":
		return "原始语言"
	case "year", "release_year":
		return "发行年份"
	case "save_path":
		return "保存路径"
	case "hash":
		return "Hash"
	case "media_type":
		return "媒体类型"
	case "media_category":
		return "类别"
	case "season_episode":
		return "季集"
	case "size", "bitrate":
		return "大小"
	case "version", "release_group":
		return "版本"
	case "rating":
		return "评分"
	case "genres":
		return "类型"
	case "overview":
		return "简介"
	case "subscription":
		return "订阅"
	case "queued":
		return "新增资源"
	default:
		return strings.TrimSpace(key)
	}
}

func telegramExternalLinks(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}
	links := []string{}
	for _, item := range []struct {
		key  string
		name string
	}{
		{key: "tmdb_url", name: "TMDB"},
		{key: "imdb_url", name: "IMDB"},
		{key: "douban_url", name: "豆瓣"},
	} {
		value := telegramDataString(data, item.key)
		if isTelegramRemotePhotoURL(value) {
			links = append(links, fmt.Sprintf(`<a href="%s">%s</a>`, escapeHTML(value), escapeHTML(item.name)))
		}
	}
	if len(links) == 0 {
		return ""
	}
	return "🔗 外链：" + strings.Join(links, " / ")
}

func telegramEventPhotoURL(event NotifyEvent) string {
	for _, key := range []string{"photo_url", "poster_url", "poster", "image_url", "backdrop_url"} {
		value := telegramDataString(event.Data, key)
		if isTelegramRemotePhotoURL(value) {
			return value
		}
	}
	return ""
}

func telegramDataString(data map[string]interface{}, key string) string {
	if len(data) == 0 {
		return ""
	}
	for k, value := range data {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return telegramValueString(value)
		}
	}
	return ""
}

func telegramValueString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []string:
		return strings.TrimSpace(strings.Join(v, ","))
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := telegramValueString(item); s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, ",")
	case float32:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "<nil>" {
			return ""
		}
		return text
	}
}

func isTelegramRemotePhotoURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
