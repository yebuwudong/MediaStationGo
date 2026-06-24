package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyMarkPlayedRefreshesPlaybackDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Watched Movie",
		Path:      `/media/movies/Watched Movie.mkv`,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
		Device: service.NewDeviceService(zap.NewNop(), repos),
	})

	token := signedTestToken(t, secret)
	req := httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/PlayedItems/media-1", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="played-device", Token="`+token+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	devices, err := repos.UserDevice.ListByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 || devices[0].LastPlayAt == nil {
		t.Fatalf("mark played should refresh playback device, got %#v", devices)
	}
	if devices[0].DeviceID != "played-device" || devices[0].DeviceName != "iPhone" || devices[0].Client != "Infuse" {
		t.Fatalf("playback device info not parsed: %#v", devices[0])
	}
}

func TestEmbyCompatSessionAllowsSameClientRequestsWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	permissions := service.NewPermissionService(log, repos)
	auth := service.NewAuthService(cfg, log, repos, service.NewTokenService(cfg, log, repos), permissions)
	user, _, err := auth.Register(context.Background(), "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.Library.Create(t.Context(), &model.Library{Name: "Movies", Path: "D:\\media", Type: "movie", Enabled: true}); err != nil {
		t.Fatalf("create library: %v", err)
	}

	embyCompatSessions.Lock()
	embyCompatSessions.items = map[string]embyCompatSession{}
	embyCompatSessions.Unlock()

	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo:  repos,
		Auth:  auth,
		Emby:  service.NewEmbyService(cfg, log, repos),
		Audit: service.NewAuditService(log, repos),
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/authenticatebyname", strings.NewReader(`{"Username":"viewer","Pw":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Emby Theater")
	req.Header.Set("X-Emby-Device-Id", "pc-device")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login status: %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/emby/Users/"+user.ID+"/Views", nil)
	req.Header.Set("User-Agent", "Emby Theater")
	req.Header.Set("X-Emby-Device-Id", "pc-device")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("views status: %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if _, ok := payload["Items"]; !ok {
		t.Fatalf("missing Items: %#v", payload)
	}
}

func TestEmbyWithRequestAddressUsesHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://192.168.1.4:18080/System/Info/Public", nil)

	payload := embyWithRequestAddress(c, map[string]any{"Id": "mediastation-go-001"})

	if payload["LocalAddress"] != "http://192.168.1.4:18080" {
		t.Fatalf("unexpected LocalAddress: %#v", payload["LocalAddress"])
	}
	if payload["WanAddress"] != "http://192.168.1.4:18080" {
		t.Fatalf("unexpected WanAddress: %#v", payload["WanAddress"])
	}
}

func TestEmbyWithRequestAddressHonorsForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/System/Info/Public", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "media.example.test")

	payload := embyWithRequestAddress(c, map[string]any{"Id": "mediastation-go-001"})

	if payload["LocalAddress"] != "https://media.example.test" {
		t.Fatalf("unexpected LocalAddress: %#v", payload["LocalAddress"])
	}
}

func TestEmbyPublicSystemInfoLooksLikeModernEmbyServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodGet, "/System/Info/Public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode system info: %v", err)
	}
	if payload["ProductName"] != "Emby Server" {
		t.Fatalf("ProductName = %#v, want Emby Server", payload["ProductName"])
	}
	version, _ := payload["Version"].(string)
	if !strings.HasPrefix(version, "4.") {
		t.Fatalf("Version = %q, want Emby-compatible 4.x", version)
	}
}

func TestEmbyMobileCompatibilityRoutesAvoidPlaybackBlocking404s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "viewer",
		PasswordHash: "hash",
		Role:         "admin",
		IsActive:     true,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(cfg, zap.NewNop(), repos),
	})

	token := signedTestToken(t, cfg.Secrets.JWTSecret)
	tests := []struct {
		path     string
		auth     bool
		wantCode int
	}{
		{path: "/emby/System/Ext/ServerDomains", wantCode: http.StatusOK},
		{path: "/emby/Items/msgo-series-demo/Similar", auth: true, wantCode: http.StatusOK},
		{path: "/emby/api/danmu/media-demo/raw", auth: true, wantCode: http.StatusOK},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.auth {
			req.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.wantCode {
			t.Fatalf("%s status = %d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestEmbyOfficialClientProbeRoutesAvoidHomepageBlocking404s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})
	token := signedTestToken(t, secret)
	tests := []struct {
		method string
		path   string
		auth   bool
	}{
		{method: http.MethodGet, path: "/emby/CustomCssJS/Scripts"},
		{method: http.MethodGet, path: "/emby/Localization/cultures"},
		{method: http.MethodPost, path: "/emby/Sessions/Logout"},
		{method: http.MethodGet, path: "/emby/System/WakeOnLanInfo", auth: true},
		{method: http.MethodGet, path: "/emby/ScheduledTasks", auth: true},
		{method: http.MethodGet, path: "/emby/LiveTv/Recordings", auth: true},
		{method: http.MethodGet, path: "/emby/System/ActivityLog/Entries", auth: true},
		{method: http.MethodGet, path: "/emby/web/configurationpages", auth: true},
		{method: http.MethodPost, path: "/emby/Users/user-1/Configuration", auth: true},
		{method: http.MethodGet, path: "/emby/Items/Latest?UserId=user-1", auth: true},
		{method: http.MethodGet, path: "/emby/Items/Resume?UserId=user-1", auth: true},
		{method: http.MethodGet, path: "/emby/Genres", auth: true},
		{method: http.MethodGet, path: "/emby/Shows/Upcoming", auth: true},
		{method: http.MethodGet, path: "/emby/Items/item-1/ThumbnailSet", auth: true},
		{method: http.MethodGet, path: "/emby/Items/item-1/ThemeMedia", auth: true},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1/SpecialFeatures", auth: true},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1/Intros", auth: true},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.auth {
				req.Header.Set("X-Emby-Token", token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Fatalf("route returned 404 body=%s", w.Body.String())
			}
			if w.Code >= 500 {
				t.Fatalf("route returned %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestEmbySenPlayerDiscoveryRoutesReturnProtocolResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{}
	cfg.App.Port = 9011
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(cfg, zap.NewNop(), repos),
	})

	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/emby", contentType: "application/json", contains: "Emby Server"},
		{path: "/emby/", contentType: "application/json", contains: "Emby Server"},
		{path: "/Startup/Configuration", contentType: "application/json", contains: "StartupWizardCompleted"},
		{path: "/emby/Startup/Configuration", contentType: "application/json", contains: "StartupWizardCompleted"},
		{path: "/System/Configuration/Public", contentType: "application/json", contains: "IsStartupWizardCompleted"},
		{path: "/emby/System/Configuration/Public", contentType: "application/json", contains: "IsStartupWizardCompleted"},
		{path: "/QuickConnect/Enabled", contentType: "application/json", contains: "false"},
		{path: "/emby/QuickConnect/Enabled", contentType: "application/json", contains: "false"},
		{path: "/Branding/Css", contentType: "text/css", contains: ""},
		{path: "/emby/Branding/Css", contentType: "text/css", contains: ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, tt.contentType) {
				t.Fatalf("Content-Type = %q, want %q", contentType, tt.contentType)
			}
			if tt.contains != "" && !strings.Contains(w.Body.String(), tt.contains) {
				t.Fatalf("body = %q, want contains %q", w.Body.String(), tt.contains)
			}
			if strings.Contains(w.Body.String(), "<html") {
				t.Fatalf("protocol discovery route served SPA HTML: %q", w.Body.String())
			}
		})
	}
}

func TestEmbyUppercaseSessionCapabilitiesRouteNoContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos})

	req := httptest.NewRequest(http.MethodPost, "/Sessions/Capabilities/Full", strings.NewReader(`{}`))
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestEmbySessionCapabilitiesRouteAllowsPreAuthProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{})

	for _, path := range []string{"/Sessions/Capabilities", "/Sessions/Capabilities/Full", "/emby/Sessions/Capabilities", "/emby/Sessions/Capabilities/Full"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestEmbyVirtualFoldersRouteReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, lib := range []model.Library{
		{Name: "电影", Path: "D:\\media\\movies", Type: "movie", Enabled: true},
		{Name: "剧集", Path: "D:\\media\\tv", Type: "tv", Enabled: true},
		{Name: "综艺", Path: "D:\\media\\variety", Type: "variety", Enabled: true},
	} {
		if err := repos.Library.Create(t.Context(), &lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos})

	req := httptest.NewRequest(http.MethodGet, "/Library/VirtualFolders", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q body=%s", contentType, w.Body.String())
	}
	if strings.HasPrefix(strings.TrimSpace(w.Body.String()), "<!doctype html>") {
		t.Fatalf("route returned frontend HTML instead of JSON")
	}

	var folders []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("decode folders: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("expected 3 folders, got %d: %#v", len(folders), folders)
	}
	if folders[1]["CollectionType"] != "tvshows" || folders[2]["CollectionType"] != "tvshows" {
		t.Fatalf("episodic libraries should expose tvshows collection type: %#v", folders)
	}
}

func TestEmbyItemsCountsRouteReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos})

	for _, path := range []string{"/Items/Counts", "/Users/user-1/Items/Counts", "/items/counts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s decode response: %v", path, err)
		}
		if _, ok := body["MovieCount"]; !ok {
			t.Fatalf("%s missing MovieCount: %#v", path, body)
		}
	}
}

func TestEmbyDisplayPreferencesAllowsAnonymousCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "secret", &service.Container{})

	req := httptest.NewRequest(http.MethodGet, "/emby/DisplayPreferences/usersettings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected GET status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode display preferences: %v", err)
	}
	if body["Id"] != "usersettings" {
		t.Fatalf("unexpected preferences payload: %#v", body)
	}
	customPrefs, ok := body["CustomPrefs"].(map[string]any)
	if !ok {
		t.Fatalf("missing CustomPrefs: %#v", body)
	}
	if customPrefs["homesection0"] != "smalllibrarytiles" || customPrefs["homesection2"] != "latestmedia" {
		t.Fatalf("homepage sections should expose library tiles and latest media: %#v", customPrefs)
	}

	req = httptest.NewRequest(http.MethodPost, "/emby/displaypreferences/usersettings", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected POST status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestEmbyWebSocketRouteUpgradesForOfficialClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "secret", &service.Container{})
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/embywebsocket?api_key=test-token&deviceId=device-1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("websocket dial failed status=%d err=%v", status, err)
	}
	defer conn.Close()
	if resp == nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected websocket upgrade, got resp=%#v", resp)
	}
}

func signedTestToken(t *testing.T, secret string) string {
	t.Helper()
	claims := middleware.Claims{
		UserID: "user-1",
		Role:   "admin",
		Tier:   "plus",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "mediastationgo-test",
			Subject:   "user-1",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}
