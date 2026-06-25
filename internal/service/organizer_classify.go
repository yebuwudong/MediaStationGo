package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// isSmartClassifyEnabled checks database settings first, then config.yaml.
func (o *OrganizerService) isSmartClassifyEnabled(ctx context.Context) bool {
	if o.repo != nil && o.repo.Setting != nil {
		val, err := o.repo.Setting.Get(ctx, "organizer.smart_classify")
		if err == nil && val != "" {
			return val == "true" || val == "1" || val == "on"
		}
	}
	if o == nil || o.cfg == nil {
		return false
	}
	return o.cfg.Organizer.SmartClassify
}

// SmartClassify determines the subcategory folder based on media metadata.
// It returns values such as "华语电影", "欧美剧", or "日番".
func (o *OrganizerService) SmartClassify(ctx context.Context, m *model.Media) string {
	if !o.isSmartClassifyEnabled(ctx) {
		return ""
	}
	lib, err := o.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return ""
	}
	return o.classifyMedia(ctx, m, lib.Type)
}

// parseCommaList splits a comma-separated string into trimmed non-empty values.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
