package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func TestImageProxyServesLocalImagePath(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "episode-thumb.png")
	if err := os.WriteFile(imagePath, transparent1x1PNG, 0o644); err != nil {
		t.Fatal(err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(dir, "cache")}}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/api/img", nil)
	rec := httptest.NewRecorder()

	if err := proxy.Serve(t.Context(), rec, req, imagePath); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got == "" {
		t.Fatal("missing content-type")
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("body length = %d, want %d", rec.Body.Len(), len(transparent1x1PNG))
	}
}

// TestImageProxyServesPosterUnderLibraryRoot verifies that sidecar posters
// stored under an arbitrary media library root (not the configured
// data/cache/movies dirs) are served rather than dropped to the placeholder.
// This is the regression that made web/Emby posters disappear.
func TestImageProxyServesPosterUnderLibraryRoot(t *testing.T) {
	libDir := t.TempDir()
	posterPath := filepath.Join(libDir, "Inception (2010)", "poster.png")
	if err := os.MkdirAll(filepath.Dir(posterPath), 0o755); err != nil {
		t.Fatal(err)
	}
	realPoster := []byte("THIS-IS-A-REAL-POSTER-NOT-THE-PLACEHOLDER")
	if err := os.WriteFile(posterPath, realPoster, 0o644); err != nil {
		t.Fatal(err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())

	// Without a library-roots provider the poster lives outside every allowed
	// root, so it must fall back to the transparent placeholder.
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), posterPath); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("expected placeholder before provider set, got %d bytes", rec.Body.Len())
	}

	// Once the library root is known, the real poster bytes are served.
	proxy.SetLibraryRootsProvider(func() []string { return []string{libDir} })
	rec = httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), posterPath); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.Bytes(); string(got) != string(realPoster) {
		t.Fatalf("served %q, want real poster bytes", string(got))
	}
}

func TestImageProxyCachesFailedRemoteImageFetch(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
			Request:    req,
		}, nil
	})}
	raw := "https://image.tmdb.org/t/p/w500/poster.jpg"
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), raw); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if rec.Body.Len() != len(transparent1x1PNG) {
			t.Fatalf("body length = %d, want placeholder %d", rec.Body.Len(), len(transparent1x1PNG))
		}
		if got := rec.Header().Get("Cache-Control"); got != imagePlaceholderCacheControl {
			t.Fatalf("Cache-Control = %q, want %q", got, imagePlaceholderCacheControl)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 due to negative cache", got)
	}
}

func TestImageProxyCachesCloudResolvedImage(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader(string(transparent1x1PNG))),
			Request:    req,
		}, nil
	})}

	link := &cloud.DirectLink{URL: "http://cloud-provider.invalid/poster.png"}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		if err := proxy.ServeCloudResolved(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=poster.png", nil), "openlist:poster.png", link); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != imageBrowserCacheControl {
			t.Fatalf("Cache-Control = %q, want %q", got, imageBrowserCacheControl)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 due to cloud image cache", got)
	}
}

type imageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f imageRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestIsPrivateHost(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.5", "192.168.1.10", "169.254.169.254", "0.0.0.0", "::1", ""}
	for _, h := range blocked {
		if !isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = false, want true (literal private/loopback IP)", h)
		}
	}
	// Hostnames must NOT be blocked even though GFW DNS poisoning may resolve
	// them to private/loopback IPs — blocking them broke legitimate posters.
	allowed := []string{"image.tmdb.org", "lain.bgm.tv", "example.com", "8.8.8.8"}
	for _, h := range allowed {
		if isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = true, want false", h)
		}
	}
}
