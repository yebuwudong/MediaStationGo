// Package service — external storage configuration management.
//
// StorageConfigService stores connection settings encrypted at rest
// (via CryptoService). It also exposes a Test() probe so the React UI
// can verify the credentials before saving.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// StorageConfigService encrypts + persists external storage configs.
type StorageConfigService struct {
	log           *zap.Logger
	repo          *repository.Container
	crypto        *CryptoService
	client        *http.Client
	resolveMu     sync.Mutex
	resolveCache  map[string]cloudResolveCacheEntry
	resolveFlight map[string]*cloudResolveCall
}

// NewStorageConfigService is the constructor.
func NewStorageConfigService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *StorageConfigService {
	return &StorageConfigService{
		log:           log,
		repo:          repo,
		crypto:        crypto,
		client:        &http.Client{Timeout: 120 * time.Second},
		resolveCache:  make(map[string]cloudResolveCacheEntry),
		resolveFlight: make(map[string]*cloudResolveCall),
	}
}

// StorageInput is the create / update payload accepted by the API.
// Config is a free-form map whose required keys depend on Type.
type StorageInput struct {
	Type    string         `json:"type" binding:"required"`
	Config  map[string]any `json:"config" binding:"required"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// StorageView is what we return to the React UI. The actual ciphertext
// is decoded back to a map (with secret keys still redacted in the
// list endpoint via Redact).
type StorageView struct {
	model.StorageConfig
	Config map[string]any `json:"config"`
}

// Get returns the decrypted config view, or (nil, nil).
func (s *StorageConfigService) Get(ctx context.Context, kind string) (*StorageView, error) {
	row, err := s.repo.StorageConfig.Get(ctx, kind)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	plain := s.crypto.Decrypt(row.Config)
	var cfg map[string]any
	_ = json.Unmarshal([]byte(plain), &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	return &StorageView{StorageConfig: *row, Config: cfg}, nil
}

// List returns every config view (used by /admin/storage/status).
func (s *StorageConfigService) List(ctx context.Context) ([]StorageView, error) {
	rows, err := s.repo.StorageConfig.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StorageView, 0, len(rows))
	for _, r := range rows {
		if !IsAdminStorageConfigurable(r.Type) {
			continue
		}
		plain := s.crypto.Decrypt(r.Config)
		var cfg map[string]any
		_ = json.Unmarshal([]byte(plain), &cfg)
		// Redact secrets when listing.
		for _, k := range []string{"password", "secret_key", "token", "cookie", "access_key"} {
			if v, ok := cfg[k]; ok && fmt.Sprint(v) != "" {
				cfg[k] = "********"
			}
		}
		out = append(out, StorageView{StorageConfig: r, Config: cfg})
	}
	return out, nil
}

// Save inserts or updates the config row.
func (s *StorageConfigService) Save(ctx context.Context, in StorageInput) (*StorageView, error) {
	if !validStorageType(in.Type) {
		return nil, fmt.Errorf("unsupported storage type %q", in.Type)
	}
	blob, err := json.Marshal(in.Config)
	if err != nil {
		return nil, err
	}
	cipher := s.crypto.Encrypt(string(blob))
	row := &model.StorageConfig{
		Type:    in.Type,
		Config:  cipher,
		Enabled: true,
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if err := s.repo.StorageConfig.Upsert(ctx, row); err != nil {
		return nil, err
	}
	s.clearResolveCacheForType(in.Type)
	return s.Get(ctx, in.Type)
}

// Logout clears saved cloud login credentials, disables the storage backend,
// and removes virtual cloud libraries/media for that provider. It intentionally
// keeps non-secret connection hints such as server / WebDAV URL / timeout so
// the admin can log in again without rebuilding the form.
func (s *StorageConfigService) Logout(ctx context.Context, typ string) (*StorageView, error) {
	if !validStorageType(typ) {
		return nil, fmt.Errorf("unsupported storage type %q", typ)
	}
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	cfg := make(map[string]any, len(view.Config))
	for k, v := range view.Config {
		if isStorageLoginSecretKey(k) || isDeprecatedStoragePlaybackKey(k) {
			continue
		}
		cfg[k] = v
	}
	enabled := false
	saved, err := s.Save(ctx, StorageInput{Type: typ, Config: cfg, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	purged, err := s.purgeCloudLibraries(ctx, typ)
	if err != nil {
		return nil, err
	}
	if s.log != nil {
		s.log.Info("storage logout cleared cloud libraries",
			zap.String("storage_type", typ),
			zap.Int("libraries_deleted", purged))
	}
	return saved, nil
}

func (s *StorageConfigService) purgeCloudLibraries(ctx context.Context, storageType string) (int, error) {
	if s == nil || s.repo == nil || s.repo.Library == nil || s.repo.Media == nil {
		return 0, nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list libraries: %w", err)
	}
	var affectedLibs []string
	for _, lib := range libs {
		if mount, ok := ParseCloudLibraryMount(lib.Path); ok && mount.Provider == storageType {
			affectedLibs = append(affectedLibs, lib.ID)
		}
	}
	for _, libID := range affectedLibs {
		if err := s.repo.Media.PurgeByLibrary(ctx, libID); err != nil {
			if s.log != nil {
				s.log.Warn("purge media by library failed", zap.String("library_id", libID), zap.Error(err))
			}
			return len(affectedLibs), fmt.Errorf("purge media by library %s: %w", libID, err)
		}
	}
	for _, libID := range affectedLibs {
		if err := s.repo.Library.Delete(ctx, libID); err != nil {
			if s.log != nil {
				s.log.Warn("delete library failed", zap.String("library_id", libID), zap.Error(err))
			}
			return len(affectedLibs), fmt.Errorf("delete library %s: %w", libID, err)
		}
	}
	return len(affectedLibs), nil
}

func isStorageLoginSecretKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "cookie", "token", "username", "password", "access_key", "secret_key":
		return true
	default:
		return false
	}
}

func isDeprecatedStoragePlaybackKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "force_302", "force_proxy":
		return true
	default:
		return false
	}
}

// Test runs a connection probe against the supplied (un-saved) config.
// The implementation is best-effort: it issues a single HEAD/PROPFIND
// to verify reachability, not full functionality.
func (s *StorageConfigService) Test(ctx context.Context, in StorageInput) error {
	cfg := in.Config
	if cfg == nil {
		return errors.New("config required")
	}
	client := s.clientForConfig(cfg)
	switch in.Type {
	case "alist":
		server := strings.TrimRight(strr(cfg["server"]), "/")
		if server == "" {
			return errors.New("alist missing server")
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/me", nil)
		if tok := strr(cfg["token"]); tok != "" {
			req.Header.Set("Authorization", tok)
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("alist returned %d", resp.StatusCode)
		}
		return nil
	case cloud.TypeOpenList:
		if hasWebDAVProbeConfig(cfg) {
			p, err := cloud.New(in.Type, cfg, client)
			if err != nil {
				return err
			}
			return p.Ping(ctx)
		}
		server := strings.TrimRight(strr(cfg["server"]), "/")
		if server != "" {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/me", nil)
			if tok := strr(cfg["token"]); tok != "" {
				req.Header.Set("Authorization", tok)
			}
			resp, err := client.Do(req)
			if err != nil {
				return decorateStorageTransportError("openlist", server, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 500 {
				return fmt.Errorf("openlist returned %d", resp.StatusCode)
			}
			return nil
		}
		p, err := cloud.New(in.Type, cfg, client)
		if err != nil {
			return err
		}
		return p.Ping(ctx)
	case "webdav":
		u := strr(cfg["url"])
		if u == "" {
			return errors.New("webdav missing url")
		}
		req, _ := http.NewRequestWithContext(ctx, "PROPFIND", u, nil)
		if user := strr(cfg["username"]); user != "" {
			req.SetBasicAuth(user, strr(cfg["password"]))
		}
		req.Header.Set("Depth", "0")
		resp, err := client.Do(req)
		if err != nil {
			return decorateStorageTransportError("webdav", u, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusUnauthorized {
			// 401 with creds means bad creds; with no creds it's reachable.
			if user := strr(cfg["username"]); user == "" && resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
			return fmt.Errorf("webdav returned %d", resp.StatusCode)
		}
		return nil
	case "s3":
		ep := strr(cfg["endpoint"])
		if ep == "" {
			return errors.New("s3 missing endpoint")
		}
		// We only verify endpoint reachability — full SigV4 is a large
		// dependency; the upstream Vue project also stops at this level.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	case cloud.Type115, cloud.TypeCloudDrive2:
		p, err := cloud.New(in.Type, cfg, client)
		if err != nil {
			return err
		}
		return p.Ping(ctx)
	default:
		return fmt.Errorf("unsupported storage type %q", in.Type)
	}
}

// CloudProvider constructs a cloud-disk provider from the saved (decrypted)
// config for the given type, or returns an error if not configured.
func (s *StorageConfigService) CloudProvider(ctx context.Context, typ string) (cloud.Provider, error) {
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	if !view.Enabled {
		return nil, fmt.Errorf("%s storage disabled", typ)
	}
	return cloud.New(typ, view.Config, s.clientForConfig(view.Config))
}

// CloudList lists entries under dirID for the configured cloud provider.
func (s *StorageConfigService) CloudList(ctx context.Context, typ, dirID string) ([]cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	return p.List(ctx, dirID)
}

func (s *StorageConfigService) CloudMkdir(ctx context.Context, typ, parentDir, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	mutable, ok := p.(cloud.MutableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support folder creation", typ)
	}
	return mutable.Mkdir(ctx, parentDir, name)
}

func (s *StorageConfigService) CloudRename(ctx context.Context, typ, ref, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	mutable, ok := p.(cloud.MutableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support rename", typ)
	}
	return mutable.Rename(ctx, ref, name)
}

func (s *StorageConfigService) CloudMove(ctx context.Context, typ, ref, targetDir, name string) (*cloud.FileEntry, error) {
	p, err := s.CloudProvider(ctx, typ)
	if err != nil {
		return nil, err
	}
	movable, ok := p.(cloud.MovableProvider)
	if !ok {
		return nil, fmt.Errorf("%s does not support move", typ)
	}
	return movable.Move(ctx, ref, targetDir, name)
}

func (s *StorageConfigService) clientForConfig(cfg map[string]any) *http.Client {
	if s == nil || s.client == nil {
		return &http.Client{Timeout: 120 * time.Second}
	}
	timeout := storageTimeoutFromConfig(cfg, s.client.Timeout)
	if timeout == s.client.Timeout {
		return s.client
	}
	cp := *s.client
	cp.Timeout = timeout
	return &cp
}

func storageTimeoutFromConfig(cfg map[string]any, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 120 * time.Second
	}
	raw := ""
	for _, key := range []string{"timeout_seconds", "webdav_timeout_seconds", "request_timeout_seconds"} {
		if value := strr(cfg[key]); value != "" {
			raw = value
			break
		}
	}
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		if f, ferr := strconv.ParseFloat(raw, 64); ferr == nil {
			seconds = int(f)
		}
	}
	if seconds <= 0 {
		return fallback
	}
	if seconds < 5 {
		seconds = 5
	}
	if seconds > 600 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

// cloudLibraryName maps a provider type to a friendly Chinese library name.
func cloudLibraryName(typ string) string {
	switch typ {
	case cloud.Type115:
		return "115 网盘"
	case cloud.TypeCloudDrive2:
		return "CloudDrive2"
	case cloud.TypeOpenList:
		return "OpenList"
	default:
		return typ
	}
}

// ensureCloudLibrary returns (creating if necessary) the per-provider cloud
// library that owns imported 302 media.
func (s *StorageConfigService) ensureCloudLibrary(ctx context.Context, typ string) (*model.Library, error) {
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	path := "cloud://" + typ
	for i := range libs {
		if libs[i].Path == path {
			return &libs[i], nil
		}
	}
	lib := &model.Library{Name: cloudLibraryName(typ), Path: path, Type: "movie", Enabled: true}
	if err := s.repo.Library.Create(ctx, lib); err != nil {
		return nil, err
	}
	return lib, nil
}

// CloudImport creates (or refreshes) a playable media row backed by a cloud
// file. Playback is served entirely via 302 redirect — the host never streams
// the bytes (unless the provider requires proxy mode).
func (s *StorageConfigService) CloudImport(ctx context.Context, typ, fileRef, name string, size int64) (*model.Media, error) {
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	if strings.TrimSpace(fileRef) == "" {
		return nil, errors.New("file reference required")
	}
	lib, err := s.ensureCloudLibrary(ctx, typ)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(name)
	container := ""
	if i := strings.LastIndex(title, "."); i > 0 {
		container = strings.ToLower(strings.TrimPrefix(title[i:], "."))
		title = title[:i]
	}
	if title == "" {
		title = fileRef
	}
	m := &model.Media{
		LibraryID:    lib.ID,
		Title:        title,
		Path:         cloudMediaPath(typ, fileRef),
		SizeBytes:    size,
		Container:    container,
		STRMURL:      BuildRelativeCloudPlayURL(typ, fileRef),
		ScrapeStatus: "pending",
	}
	if err := s.repo.Media.Upsert(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func validStorageType(t string) bool {
	switch t {
	case "alist", "s3", "webdav", cloud.Type115, cloud.TypeCloudDrive2, cloud.TypeOpenList:
		return true
	}
	return false
}

func hasWebDAVProbeConfig(cfg map[string]any) bool {
	return strr(cfg["url"]) != "" ||
		strr(cfg["webdav_url"]) != "" ||
		strr(cfg["username"]) != "" ||
		strr(cfg["password"]) != ""
}

func decorateStorageTransportError(name, target string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "server gave HTTP response to HTTPS client") {
		return fmt.Errorf("%s: %w；当前地址使用 https://，但服务端返回 HTTP。请改用 http:// 地址；OpenList 默认 WebDAV 通常是 http://host:5244/dav/，管理页面/API 地址通常是 http://host:5244", name, err)
	}
	if strings.Contains(message, "first record does not look like a TLS handshake") {
		return fmt.Errorf("%s: %w；疑似把 HTTP 服务配置成了 https://，请检查 %s 的协议头", name, err, target)
	}
	return err
}

// strr is a tiny helper to avoid importing fmt.Sprint just to coerce
// interface{} → string. (Named "strr" so it doesn't collide with the
// notify channel's `str` helper which already lives in this package.)
func strr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// DeleteStorage 删除存储配置并清理关联数据
func (s *StorageConfigService) DeleteStorage(ctx context.Context, storageType string) error {
	// 查找配置
	cfg, err := s.repo.StorageConfig.Get(ctx, storageType)
	if err != nil || cfg == nil {
		return fmt.Errorf("storage config not found: %s", storageType)
	}

	affectedLibs, err := s.purgeCloudLibraries(ctx, storageType)
	if err != nil {
		return err
	}

	// 删除存储配置
	if err := s.repo.StorageConfig.Delete(ctx, cfg.ID); err != nil {
		return fmt.Errorf("delete storage config: %w", err)
	}

	s.log.Info("storage deleted",
		zap.String("storage_type", storageType),
		zap.Int("libraries_deleted", affectedLibs))

	return nil
}
