package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSessionTrackerAppliesRealtimeActivityToUsers(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	old := now.Add(-8 * time.Hour)
	users := []model.User{{Base: model.Base{ID: "u1"}, Username: "admin", LastLoginAt: &old}}

	tracker.RecordLogin(t.Context(), "u1", "admin", "web-1", "Web", "Browser", "127.0.0.1")
	tracker.ApplyToUsers(t.Context(), users)

	if users[0].LastLoginAt == nil || !users[0].LastLoginAt.Equal(now) {
		t.Fatalf("last_login_at = %v, want realtime %v", users[0].LastLoginAt, now)
	}
	if !users[0].RealtimeOnline || users[0].RealtimeDeviceCount != 1 {
		t.Fatalf("realtime flags online=%v devices=%d", users[0].RealtimeOnline, users[0].RealtimeDeviceCount)
	}
}

func TestDeviceListMergesRealtimeSessions(t *testing.T) {
	repos := newSessionTrackerTestRepos(t)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.RecordPlayback(t.Context(), user.ID, user.Username, "dev-1", "Apple TV", "Yamby", "10.0.0.8", "media-1", 123, 456, false)

	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)
	rows, err := device.ListDevices(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("devices = %d, want 1", len(rows))
	}
	if !rows[0].Realtime || !rows[0].Online || !rows[0].Playing {
		t.Fatalf("realtime device flags = realtime:%v online:%v playing:%v", rows[0].Realtime, rows[0].Online, rows[0].Playing)
	}
	if rows[0].DeviceName != "Apple TV" || rows[0].Client != "Yamby" || !rows[0].LastSeenAt.Equal(now) {
		t.Fatalf("device row = %#v", rows[0])
	}
}

func TestRealtimeRecentLoginProtectsCleanupCandidate(t *testing.T) {
	repos := newSessionTrackerTestRepos(t)
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true, LastLoginAt: &old}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), SettingAccountCleanupEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), SettingAccountCleanupRules, `[{"id":"login_7d","name":"最近登录","type":"recent_login","enabled":true,"window_days_max":7}]`); err != nil {
		t.Fatal(err)
	}
	tracker := NewSessionTrackerService(zap.NewNop())
	tracker.now = func() time.Time { return now }
	tracker.RecordLogin(t.Context(), user.ID, user.Username, "web", "Web", "Browser", "127.0.0.1")
	device := NewDeviceService(zap.NewNop(), repos)
	device.SetSessionTracker(tracker)

	candidates, err := device.PreviewAccountCleanup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Fatalf("recent realtime login should protect user, got candidates %#v", candidates)
	}
}

func newSessionTrackerTestRepos(t *testing.T) *repository.Container {
	t.Helper()
	db := newServiceTestDB(t, &model.User{}, &model.Setting{}, &model.UserDevice{}, &model.SignIn{}, &model.PlaybackHistory{})
	return repository.New(db)
}
