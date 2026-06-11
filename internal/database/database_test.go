package database

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEnforceTelegramBindingOneToOneCleansDuplicatesAndAddsIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.TelegramBinding{}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-time.Hour)
	rows := []model.TelegramBinding{
		{TelegramUserID: 10001, ChatID: 10001, UserID: "user-1"},
		{TelegramUserID: 10002, ChatID: 10002, UserID: "user-1"},
	}
	for i := range rows {
		rows[i].CreatedAt = createdAt.Add(time.Duration(i) * time.Minute)
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := enforceTelegramBindingOneToOne(db); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Model(&model.TelegramBinding{}).Where("user_id = ?", "user-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active bindings for user-1 = %d, want 1", count)
	}
	if err := db.Create(&model.TelegramBinding{TelegramUserID: 10003, ChatID: 10003, UserID: "user-1"}).Error; err == nil {
		t.Fatal("expected unique index to reject another active binding for the same user")
	}
}

func TestEnsurePerformanceIndexesCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	if err := ensurePerformanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"idx_media_created_active",
		"idx_media_episode_created_active",
		"idx_media_library_created_active",
		"idx_media_library_episode_active",
		"idx_favorites_user_media_active",
		"idx_playback_histories_user_media_active",
		"idx_play_profiles_user_created_active",
	} {
		var count int
		if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("index %s count = %d, want 1", name, count)
		}
	}
}
