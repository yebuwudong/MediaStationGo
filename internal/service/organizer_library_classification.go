package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func organizeLibraryModelType(mediaType string) string {
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv", "anime", "variety":
		return "tv"
	case "adult", "movie":
		return "movie"
	default:
		return "movie"
	}
}

func organizeLibraryTypeName(mediaType string) string {
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv":
		return "电视剧"
	case "anime":
		return "动漫"
	case "variety":
		return "综艺"
	case "adult":
		return "成人"
	default:
		return "电影"
	}
}

func (o *OrganizerService) organizeCategoryAliases(mediaType, category string) map[string]struct{} {
	aliases := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			key := normalizeOrganizeCategoryKey(value)
			if key != "" {
				aliases[key] = struct{}{}
			}
		}
	}
	categories := o.categoryMap()
	add(category)
	switch normalizeOrganizeCategoryKey(category) {
	case normalizeOrganizeCategoryKey(categoryName(categories, "jp_anime", "日番")), "日番", "日漫", "日本动漫", "日本動畫", "日本动画":
		add("日番", "日漫", "日本动漫", "日本动画")
	case normalizeOrganizeCategoryKey(categoryName(categories, "cn_anime", "国漫")), "国漫", "国产动漫", "國漫":
		add("国漫", "国产动漫")
	case normalizeOrganizeCategoryKey(categoryName(categories, "euus_anime", "欧美动漫")), "欧美动漫", "欧美动画", "西方动画":
		add("欧美动漫", "欧美动画", "西方动画")
	case normalizeOrganizeCategoryKey(categoryName(categories, "domestic_tv", "国产剧")), "国产剧", "国剧", "大陆剧", "国产电视剧":
		add("国产剧", "国剧", "大陆剧", "国产电视剧")
	case normalizeOrganizeCategoryKey(categoryName(categories, "euus_tv", "欧美剧")), "欧美剧", "欧美电视剧":
		add("欧美剧", "欧美电视剧")
	case normalizeOrganizeCategoryKey(categoryName(categories, "jk_tv", "日韩剧")), "日韩剧", "日剧", "韩剧":
		add("日韩剧", "日剧", "韩剧")
	case normalizeOrganizeCategoryKey(categoryName(categories, "variety", "综艺")), "综艺", "真人秀":
		add("综艺", "真人秀")
	case normalizeOrganizeCategoryKey(categoryName(categories, "documentary", "纪录片")), "纪录片", "纪录":
		add("纪录片", "纪录")
	case normalizeOrganizeCategoryKey(categoryName(categories, "children", "儿童")), "儿童", "少儿":
		add("儿童", "少儿")
	case normalizeOrganizeCategoryKey(categoryName(categories, "chinese_movie", "华语电影")), "华语电影", "国产电影", "大陆电影":
		add("华语电影", "国产电影", "大陆电影")
	case normalizeOrganizeCategoryKey(categoryName(categories, "foreign_movie", "外语电影")), "外语电影":
		add("外语电影")
	case normalizeOrganizeCategoryKey(categoryName(categories, "animation_movie", "动画电影")), "动画电影", "动漫电影":
		add("动画电影", "动漫电影")
	case normalizeOrganizeCategoryKey(categoryName(categories, "adult", "成人")), "成人":
		add("成人")
	case normalizeOrganizeCategoryKey(categoryName(categories, "adult_9kg", "9KG")), "9kg":
		add("9KG")
	case normalizeOrganizeCategoryKey(categoryName(categories, "adult_jav", "番号")), "番号", "jav":
		add("番号", "JAV")
	}
	return aliases
}

func libraryMatchesOrganizeCategory(lib model.Library, aliases map[string]struct{}) bool {
	for _, value := range []string{lib.Name, filepath.Base(filepath.Clean(lib.Path))} {
		if _, ok := aliases[normalizeOrganizeCategoryKey(value)]; ok {
			return true
		}
	}
	return false
}

func organizeLibraryTypeScore(mediaType, libraryType string) int {
	libraryType = normalizeOrganizeMediaType(libraryType)
	if mediaType == "" || libraryType == "" {
		return 1
	}
	if mediaType == libraryType {
		return 8
	}
	if mediaType == "anime" && libraryType == "tv" {
		return 5
	}
	if mediaType == "variety" && libraryType == "tv" {
		return 5
	}
	return 0
}

func normalizeOrganizeCategoryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}

func pathDepth(path string) int {
	path = filepath.Clean(path)
	if path == "." || path == string(os.PathSeparator) {
		return 0
	}
	return len(strings.Split(path, string(os.PathSeparator)))
}
