package handler

import (
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestDiscoverProviderEnabledHonorsAPIConfigToggle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	apiConfig := service.NewAPIConfigService(zap.NewNop(), repos, service.NewCryptoService("", zap.NewNop()))
	enabled := false
	if _, err := apiConfig.Update(t.Context(), "douban", service.APIConfigPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{APIConfig: apiConfig}

	if discoverProviderEnabled(t.Context(), svc, "douban") {
		t.Fatal("disabled API config should disable discover provider")
	}
	if !discoverProviderEnabled(t.Context(), svc, "missing-provider") {
		t.Fatal("missing API config should keep discover provider available")
	}
}
