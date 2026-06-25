package service

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func newTestEmbyService(t *testing.T) *EmbyService {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.User{}, &model.Setting{})
	// 内存库 + 异步探测协程：限制为单连接，避免连接池新建连接时
	// 拿到一个空白的 :memory: 实例（no such table）。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	repos := repository.New(db)
	return NewEmbyService(&config.Config{}, zap.NewNop(), repos)
}

type fakeCloudPlaybackResolver struct {
	link *cloud.DirectLink
	typ  string
	ref  string
	ua   string
}

func (f *fakeCloudPlaybackResolver) CloudResolve(_ context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error) {
	f.typ = typ
	f.ref = fileRef
	f.ua = clientUA
	return f.link, nil
}

type fakeCloudPlaybackProber struct {
	probe   *ProbeResult
	rawURL  string
	headers map[string]string
}

func (f *fakeCloudPlaybackProber) ProbeHTTP(_ context.Context, rawURL string, headers map[string]string) (*ProbeResult, error) {
	f.rawURL = rawURL
	f.headers = headers
	return f.probe, nil
}
