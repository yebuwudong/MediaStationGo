package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServeSPANoCachesIndexAndServesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html><div id=\"root\"></div></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "favicon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	serveSPA(router, webDir)

	for _, path := range []string{"/", "/login", "/media/abc"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, w.Code)
		}
		if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Fatalf("%s Cache-Control = %q, want no-store", path, got)
		}
		if !strings.Contains(w.Body.String(), "root") {
			t.Fatalf("%s did not serve index.html: %q", path, w.Body.String())
		}
	}
}

func TestServeSPAServesAssetsImmutableAndBypassesAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	serveSPA(router, webDir)

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetResp := httptest.NewRecorder()
	router.ServeHTTP(assetResp, assetReq)
	if assetResp.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200", assetResp.Code)
	}
	if got := assetResp.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset Cache-Control = %q, want immutable", got)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	apiResp := httptest.NewRecorder()
	router.ServeHTTP(apiResp, apiReq)
	if apiResp.Code != http.StatusNotFound {
		t.Fatalf("api fallback status = %d, want 404", apiResp.Code)
	}
	if strings.Contains(apiResp.Body.String(), "index") {
		t.Fatalf("api route should not serve SPA index: %q", apiResp.Body.String())
	}
}

func TestServeSPAMissingIndexReportsExplicit404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	serveSPA(router, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "web UI not found") {
		t.Fatalf("body = %q, want explicit missing UI message", w.Body.String())
	}
}
