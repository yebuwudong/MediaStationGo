package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestStorageConfigUploadLocalToAlist(t *testing.T) {
	var uploaded []string
	var authHeaders []string
	alist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/mkdir":
			_ = json.NewDecoder(r.Body).Decode(&map[string]string{})
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		case "/api/fs/get":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
		case "/api/fs/put":
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			decoded, err := url.PathUnescape(r.Header.Get("File-Path"))
			if err != nil {
				t.Fatalf("decode file path: %v", err)
			}
			uploaded = append(uploaded, decoded)
			_, _ = w.Write([]byte(`{"code":200,"message":"success"}`))
		default:
			t.Fatalf("unexpected alist path %s", r.URL.Path)
		}
	}))
	defer alist.Close()

	repos, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server": alist.URL,
			"token":  "alist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Movie.2026.mkv"), []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Movie.2026.nfo"), []byte("nfo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "ignore.txt"), []byte("txt"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := storage.UploadLocal(t.Context(), CloudUploadInput{
		Type:            "alist",
		SourcePath:      source,
		DestPath:        "/backup",
		Recursive:       true,
		IncludeSidecars: true,
	})
	if err != nil {
		t.Fatalf("upload local: %v", err)
	}
	if res.Uploaded != 2 || res.Skipped != 0 || len(res.Errors) != 0 {
		t.Fatalf("result = %+v", res)
	}
	sort.Strings(uploaded)
	want := []string{"/backup/Movie.2026.mkv", "/backup/Movie.2026.nfo"}
	if strings.Join(uploaded, "\n") != strings.Join(want, "\n") {
		t.Fatalf("uploaded = %#v, want %#v", uploaded, want)
	}
	for _, header := range authHeaders {
		if header != "alist-token" {
			t.Fatalf("authorization header = %q", header)
		}
	}
	if got, _ := repos.StorageConfig.Get(t.Context(), "alist"); got == nil {
		t.Fatalf("storage config should remain saved")
	}
}

func TestSchedulerCloudUploadUsesConfiguredLocalSource(t *testing.T) {
	var uploaded []string
	alist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/mkdir":
			_, _ = w.Write([]byte(`{"code":200}`))
		case "/api/fs/get":
			w.WriteHeader(http.StatusNotFound)
		case "/api/fs/put":
			decoded, _ := url.PathUnescape(r.Header.Get("File-Path"))
			uploaded = append(uploaded, decoded)
			_, _ = w.Write([]byte(`{"code":200}`))
		default:
			t.Fatalf("unexpected alist path %s", r.URL.Path)
		}
	}))
	defer alist.Close()

	repos, storage := newStorageUploadTestService(t)
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "alist",
		Config: map[string]any{
			"server": alist.URL,
			"token":  "token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "Show.S01E01.mkv"), []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		CloudUploadAutoEnabledKey: "true",
		CloudUploadProviderKey:    "alist",
		CloudUploadSourceDirKey:   source,
		CloudUploadDestPathKey:    "/cloud-media",
		CloudUploadRecursiveKey:   "true",
		CloudUploadSidecarsKey:    "false",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, nil, storage, NewHub(zap.NewNop()), "")
	if err := scheduler.jobUploadLocalToCloud(t.Context()); err != nil {
		t.Fatalf("cloud upload job: %v", err)
	}
	if len(uploaded) != 1 || uploaded[0] != "/cloud-media/Show.S01E01.mkv" {
		t.Fatalf("uploaded = %#v", uploaded)
	}
}

func newStorageUploadTestService(t *testing.T) (*repository.Container, *StorageConfigService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.StorageConfig{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	return repos, NewStorageConfigService(log, repos, NewCryptoService("", log))
}
