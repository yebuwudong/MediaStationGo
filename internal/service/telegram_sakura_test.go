package service

import (
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSakuraUserManagementAndAuditCommands(t *testing.T) {
	ctx := t.Context()
	repos, bot := newBotTestService(t)
	if err := repos.User.Create(ctx, &model.User{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9401"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9401, Username: "admin"}, Chat: TelegramChat{ID: 9401, Type: "private"}}

	reply, err := bot.executeCommand(ctx, channel, msg, "/ucr viewer secret-pass 30")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已创建用户") {
		t.Fatalf("expected user creation, got %q", reply.Text)
	}
	viewer, err := repos.User.FindByUsername(ctx, "viewer")
	if err != nil || viewer == nil {
		t.Fatalf("viewer should exist: %v", err)
	}
	if err := repos.UserDevice.Create(ctx, &model.UserDevice{
		UserID:      viewer.ID,
		DeviceID:    "dev-abc",
		DeviceName:  "Windows PC",
		Client:      "Infuse",
		LastIP:      "1.2.3.4",
		FirstSeenAt: time.Now(),
		LastSeenAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		command string
		want    string
	}{
		{"/uinfo viewer", "用户信息"},
		{"/userip viewer", "1.2.3.4"},
		{"/auditip 1.2.3", "viewer"},
		{"/auditdevice Windows", "viewer"},
		{"/auditclient Infuse", "viewer"},
		{"/udeviceid dev-abc", "viewer"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			reply, err := bot.executeCommand(ctx, channel, msg, tc.command)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(reply.Text, tc.want) {
				t.Fatalf("%s expected %q in %q", tc.command, tc.want, reply.Text)
			}
		})
	}
}

func TestSakuraBatchAndPermissionCommands(t *testing.T) {
	ctx := t.Context()
	repos, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9501"}`}
	msg := &TelegramMessage{From: TelegramUser{ID: 9501, Username: "admin"}, Chat: TelegramChat{ID: 9501, Type: "private"}}
	users := []*model.User{
		{Username: "root", PasswordHash: "x", Role: "admin", IsActive: true},
		{Username: "viewer1", PasswordHash: "x", Role: "user", IsActive: true},
		{Username: "viewer2", PasswordHash: "x", Role: "user", IsActive: true},
		{Username: "viewer3", PasswordHash: "x", Role: "user", IsActive: true},
	}
	for _, user := range users {
		if err := repos.User.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
		if user.Role != "admin" && user.Username != "viewer3" {
			if err := repos.Permission.Create(ctx, DefaultPermissions(user.ID)); err != nil {
				t.Fatal(err)
			}
		}
	}

	reply, err := bot.executeCommand(ctx, channel, msg, "/renewall 7 confirm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "批量续期完成") {
		t.Fatalf("expected renewall success, got %q", reply.Text)
	}
	renewed, _ := repos.User.FindByUsername(ctx, "viewer1")
	if renewed.ExpiredAt == nil {
		t.Fatal("renewall should set expiry for normal users")
	}

	if reply, err = bot.executeCommand(ctx, channel, msg, "/embylibs_blockall"); err != nil || !strings.Contains(reply.Text, "关闭媒体播放权限") {
		t.Fatalf("expected blockall success, reply=%q err=%v", reply.Text, err)
	}
	perm, _ := repos.Permission.FindByUserID(ctx, users[1].ID)
	if perm == nil || perm.CanPlayMedia {
		t.Fatal("embylibs_blockall should disable media playback for normal users")
	}
	perm, _ = repos.Permission.FindByUserID(ctx, users[3].ID)
	if perm == nil || perm.CanPlayMedia {
		t.Fatal("embylibs_blockall should create disabled media playback permissions when missing")
	}
	if reply, err = bot.executeCommand(ctx, channel, msg, "/embylibs_unblockall"); err != nil || !strings.Contains(reply.Text, "开启媒体播放权限") {
		t.Fatalf("expected unblockall success, reply=%q err=%v", reply.Text, err)
	}
	perm, _ = repos.Permission.FindByUserID(ctx, users[1].ID)
	if perm == nil || !perm.CanPlayMedia {
		t.Fatal("embylibs_unblockall should enable media playback for normal users")
	}

	if reply, err = bot.executeCommand(ctx, channel, msg, "/banall confirm"); err != nil || !strings.Contains(reply.Text, "已禁用普通用户") {
		t.Fatalf("expected banall success, reply=%q err=%v", reply.Text, err)
	}
	banned, _ := repos.User.FindByUsername(ctx, "viewer2")
	if banned.IsActive {
		t.Fatal("banall should disable normal users")
	}
	if reply, err = bot.executeCommand(ctx, channel, msg, "/unbanall confirm"); err != nil || !strings.Contains(reply.Text, "已解禁普通用户") {
		t.Fatalf("expected unbanall success, reply=%q err=%v", reply.Text, err)
	}
	unbanned, _ := repos.User.FindByUsername(ctx, "viewer2")
	if !unbanned.IsActive {
		t.Fatal("unbanall should re-enable normal users")
	}
}

func TestSakuraSyncExpiryAndBotAdminCommands(t *testing.T) {
	ctx := t.Context()
	repos, bot := newBotTestService(t)
	channel := &model.NotifyChannel{Name: "Telegram", Type: "telegram", Enabled: true, Config: `{"admin_user_ids":"9601"}`}
	if err := repos.NotifyChannel.Create(ctx, channel); err != nil {
		t.Fatal(err)
	}
	msg := &TelegramMessage{From: TelegramUser{ID: 9601, Username: "admin"}, Chat: TelegramChat{ID: 9601, Type: "private"}}
	past := time.Now().Add(-24 * time.Hour)
	if err := repos.User.Create(ctx, &model.User{Username: "expired", PasswordHash: "x", Role: "user", IsActive: true, ExpiredAt: &past}); err != nil {
		t.Fatal(err)
	}

	reply, err := bot.executeCommand(ctx, channel, msg, "/syncunbound")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "expired") {
		t.Fatalf("syncunbound should list unbound users, got %q", reply.Text)
	}
	reply, err = bot.executeCommand(ctx, channel, msg, "/check_ex")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "expired") {
		t.Fatalf("check_ex should list expired users, got %q", reply.Text)
	}
	reply, err = bot.executeCommand(ctx, channel, msg, "/proadmin 9602")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Text, "已添加") {
		t.Fatalf("proadmin should update channel config, got %q", reply.Text)
	}
	updated, _ := repos.NotifyChannel.FindByID(ctx, channel.ID)
	cfg := bot.telegramChannelConfig(updated)
	if !strings.Contains(cfg["admin_user_ids"], "9602") {
		t.Fatalf("expected admin ids to include 9602, got %#v", cfg)
	}
}
