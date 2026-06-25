package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLicenseStatusMaxUsersUsesLicensedLimit(t *testing.T) {
	maxUsers := 25
	state := service.LicenseActivationState{Valid: true, MaxUsers: &maxUsers}

	if got := licenseStatusMaxUsers(state); got != maxUsers {
		t.Fatalf("expected licensed max users %d, got %#v", maxUsers, got)
	}
}

func TestLicenseStatusMaxUsersAllowsUnlimited(t *testing.T) {
	state := service.LicenseActivationState{Valid: true, UnlimitedUsers: true}

	if got := licenseStatusMaxUsers(state); got != nil {
		t.Fatalf("expected unlimited max users to be nil, got %#v", got)
	}
}

func TestLicenseStatusMaxUsersFallsBackToOpenSourceLimit(t *testing.T) {
	state := service.LicenseActivationState{}

	if got := licenseStatusMaxUsers(state); got != service.OpenSourceUserLimit {
		t.Fatalf("expected open-source max users %d, got %#v", service.OpenSourceUserLimit, got)
	}
}

func TestApplyLicenseStatusReflectsEditedLimitAndClearsExpiry(t *testing.T) {
	maxUsers := 60
	licenseType := "subscription"
	state := service.LicenseActivationState{
		Valid:          true,
		LicenseType:    "enterprise",
		ExpiryDate:     "2026-01-01",
		MaxDevices:     2,
		UnlimitedUsers: true,
	}

	applyLicenseStatus(&state, licenseServerStatusResp{
		Valid:          true,
		LicenseType:    &licenseType,
		ExpiryDate:     nil,
		MaxDevices:     5,
		MaxUsers:       &maxUsers,
		UnlimitedUsers: false,
		DeviceName:     "Edited Device",
	}, "device-1")

	if !state.Valid || state.LicenseType != "subscription" || state.ExpiryDate != "" || state.MaxDevices != 5 {
		t.Fatalf("status fields were not fully refreshed: %+v", state)
	}
	if state.MaxUsers == nil || *state.MaxUsers != 60 || state.UnlimitedUsers {
		t.Fatalf("user limit was not refreshed from status: %+v", state)
	}
	if state.DeviceID != "device-1" || state.DeviceName != "Edited Device" {
		t.Fatalf("device fields were not refreshed: %+v", state)
	}
}

func TestApplyLicenseStatusReflectsUnlimitedUsers(t *testing.T) {
	maxUsers := 30
	state := service.LicenseActivationState{Valid: true, MaxUsers: &maxUsers}

	applyLicenseStatus(&state, licenseServerStatusResp{
		Valid:          true,
		MaxUsers:       nil,
		UnlimitedUsers: true,
	}, "device-1")

	if state.MaxUsers != nil || !state.UnlimitedUsers {
		t.Fatalf("unlimited status should clear previous finite user limit: %+v", state)
	}
}

func TestRefreshLicenseServerStatusReflectsEditedLimitAndHeartbeatRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status/device-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"valid": true,
			"license_type": "subscription",
			"max_devices": 5,
			"max_users": 60,
			"unlimited_users": false,
			"device_name": "NAS",
			"heartbeat_requested": true,
			"is_active": true
		}`))
	}))
	defer upstream.Close()

	state := service.LicenseActivationState{Valid: true, UnlimitedUsers: true}
	client := &licenseClient{baseURL: upstream.URL, httpClient: upstream.Client()}

	refreshed, ok, requested, err := refreshLicenseServerStatus(t.Context(), client, state, "device-1")
	if err != nil {
		t.Fatalf("refresh status: %v", err)
	}
	if !ok || !requested {
		t.Fatalf("expected valid status with requested heartbeat, ok=%v requested=%v", ok, requested)
	}
	if refreshed.MaxUsers == nil || *refreshed.MaxUsers != 60 || refreshed.UnlimitedUsers {
		t.Fatalf("edited user limit was not reflected: %+v", refreshed)
	}
	if refreshed.MaxDevices != 5 || refreshed.DeviceName != "NAS" {
		t.Fatalf("server status fields were not applied: %+v", refreshed)
	}
}

func TestLicenseHeartbeatPayloadIncludesStoredLicenseKey(t *testing.T) {
	payload := licenseHeartbeatPayload(service.LicenseActivationState{
		LicenseKey: "MS-ABCD-EFGH-JKLM-NPQR",
	}, "device-1", "NAS")

	if payload["fingerprint"] != "device-1" || payload["instance_id"] != "device-1" || payload["device_name"] != "NAS" {
		t.Fatalf("heartbeat identity payload is wrong: %#v", payload)
	}
	if payload["key"] != "MS-ABCD-EFGH-JKLM-NPQR" {
		t.Fatalf("heartbeat should include stored license key for server-side backfill: %#v", payload)
	}
}

func TestLicenseHeartbeatDueUsesTwelveHourWindow(t *testing.T) {
	state := service.LicenseActivationState{
		Valid:     true,
		UpdatedAt: time.Now().Add(-11 * time.Hour).Format(time.RFC3339),
	}
	if licenseHeartbeatDue(state, 12*time.Hour) {
		t.Fatalf("heartbeat should not be due before interval")
	}

	state.UpdatedAt = time.Now().Add(-13 * time.Hour).Format(time.RFC3339)
	if !licenseHeartbeatDue(state, 12*time.Hour) {
		t.Fatalf("heartbeat should be due after interval")
	}
}

func TestLicenseHeartbeatEligibleRequiresActivationState(t *testing.T) {
	if licenseHeartbeatEligible(service.LicenseActivationState{DeviceID: "device-only"}) {
		t.Fatalf("device id alone should not trigger automatic license heartbeat")
	}
	if !licenseHeartbeatEligible(service.LicenseActivationState{LicenseKey: "MS-KEY"}) {
		t.Fatalf("stored license key should trigger automatic license heartbeat")
	}
	if !licenseHeartbeatEligible(service.LicenseActivationState{Valid: true}) {
		t.Fatalf("valid license state should trigger automatic license heartbeat")
	}
}

func TestLicenseActivateBindsServerInstanceNotBrowserFingerprint(t *testing.T) {
	var upstreamFingerprint string
	maxUsers := 60
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		upstreamFingerprint, _ = payload["fingerprint"].(string)
		resp := licenseServerSignedResp{
			Valid:         true,
			LicenseType:   "subscription",
			MaxDevices:    3,
			MaxUsers:      &maxUsers,
			NextHeartbeat: time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		resp.Signature = signLicenseTestPayload("test-secret", resp)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.Setting.Set(t.Context(), licenseHMACSecretSetting, "test-secret"); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/license/activate", licenseActivateHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/license/activate", strings.NewReader(`{
		"key": "MS-ABCD-EFGH-JKLM-NPQR",
		"device_id": "browser-fingerprint",
		"device_name": ""
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("activate status = %d body=%s", w.Code, w.Body.String())
	}
	if upstreamFingerprint == "" || upstreamFingerprint == "browser-fingerprint" || !strings.HasPrefix(upstreamFingerprint, "msgo-") {
		t.Fatalf("activation should use server-generated msgo id, got %q", upstreamFingerprint)
	}
	stored, err := svc.Repo.Setting.Get(t.Context(), licenseDeviceIDSetting)
	if err != nil {
		t.Fatal(err)
	}
	if stored != upstreamFingerprint {
		t.Fatalf("stored device id = %q, upstream fingerprint = %q", stored, upstreamFingerprint)
	}
}

func TestNewLicenseClientRequiresHMACSecret(t *testing.T) {
	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, "http://127.0.0.1:8001"); err != nil {
		t.Fatal(err)
	}
	if _, err := newLicenseClient(t.Context(), svc); err == nil || !strings.Contains(err.Error(), "hmac secret") {
		t.Fatalf("expected missing hmac secret error, got %v", err)
	}
}

func newLicenseHandlerTestService(t *testing.T) *service.Container {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	return &service.Container{
		Cfg:  &config.Config{},
		Repo: repository.New(db),
	}
}

func signLicenseTestPayload(secret string, resp licenseServerSignedResp) string {
	unsigned := struct {
		Valid         bool    `json:"valid"`
		LicenseType   string  `json:"license_type"`
		ExpiryDate    *string `json:"expiry_date"`
		MaxDevices    int     `json:"max_devices"`
		MaxUsers      *int    `json:"max_users"`
		DaysRemaining *int    `json:"days_remaining"`
		NextHeartbeat string  `json:"next_heartbeat"`
	}{
		Valid:         resp.Valid,
		LicenseType:   resp.LicenseType,
		ExpiryDate:    resp.ExpiryDate,
		MaxDevices:    resp.MaxDevices,
		MaxUsers:      resp.MaxUsers,
		DaysRemaining: resp.DaysRemaining,
		NextHeartbeat: resp.NextHeartbeat,
	}
	payload, _ := json.Marshal(unsigned)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
