package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AddTorrent submits a magnet URL or HTTP(S) URL to qBittorrent.
//
// qBittorrent 的 /api/v2/torrents/add 在很多失败场景下仍然返回 HTTP 200
// 但 body 里写 "Fails."。我们把这些情况也识别为错误并返回，避免
// "API 返回 200 → 我们告诉前端成功 → qb 中却没下载" 这种迷惑性失败。
func (q *QBitClient) AddTorrent(ctx context.Context, magnetOrURL, savePath string) error {
	return q.AddTorrentWithCategory(ctx, magnetOrURL, savePath, "")
}

func (q *QBitClient) AddTorrentWithCategory(ctx context.Context, magnetOrURL, savePath, category string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}

	torrentData, torrentName, fetchErr := q.fetchTorrentFile(ctx, magnetOrURL)
	useFileUpload := fetchErr == nil && len(torrentData) > 0
	return q.addTorrentLocked(ctx, magnetOrURL, torrentData, torrentName, useFileUpload, savePath, category)
}

func (q *QBitClient) AddTorrentFile(ctx context.Context, data []byte, name, savePath string) error {
	return q.AddTorrentFileWithCategory(ctx, data, name, savePath, "")
}

func (q *QBitClient) AddTorrentFileWithCategory(ctx context.Context, data []byte, name, savePath, category string) error {
	if len(data) == 0 {
		return errors.New("empty torrent data")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	return q.addTorrentLocked(ctx, "", data, name, true, savePath, category)
}

func (q *QBitClient) addTorrentLocked(ctx context.Context, magnetOrURL string, torrentData []byte, torrentName string, useFileUpload bool, savePath, category string) error {
	before, beforeErr := q.listLocked(ctx, "")
	beforeHashes := make(map[string]struct{}, len(before))
	if beforeErr == nil {
		for _, torrent := range before {
			if torrent.Hash != "" {
				beforeHashes[strings.ToLower(torrent.Hash)] = struct{}{}
			}
		}
	}
	if useFileUpload && beforeErr == nil {
		if hash := torrentInfoHash(torrentData); hash != "" {
			if _, ok := beforeHashes[hash]; ok {
				q.log.Info("qbittorrent: torrent already exists", zap.String("hash", hash), zap.String("name", torrentName))
				return ErrDownloadAlreadyExists
			}
		}
	}

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if useFileUpload {
		if strings.TrimSpace(torrentName) == "" {
			torrentName = "download.torrent"
		}
		part, err := w.CreateFormFile("torrents", torrentName)
		if err != nil {
			return err
		}
		if _, err := part.Write(torrentData); err != nil {
			return err
		}
	} else {
		_ = w.WriteField("urls", magnetOrURL)
	}
	if savePath != "" {
		_ = w.WriteField("savepath", savePath)
	}
	if strings.TrimSpace(category) != "" {
		_ = w.WriteField("category", sanitizeQBitCategory(category))
	}
	_ = w.Close()

	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/add", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", q.cfg.BaseURL)
	req.Header.Set("Origin", q.cfg.BaseURL)

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(raw))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qbittorrent add: HTTP %d: %s", resp.StatusCode, bodyText)
	}
	// qb 的成功响应是 "Ok." 或空体；任何 "Fails." 视为失败。
	if strings.EqualFold(bodyText, "Fails.") {
		return fmt.Errorf("qbittorrent add: 拒绝任务 (检查 URL 是否需要认证或 savePath 是否可写)")
	}
	if beforeErr == nil {
		accepted := false
		var lastListErr error
		for attempt := 0; attempt < qbitAddVerifyAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(qbitAddVerifyInterval)
			}
			after, err := q.listLocked(ctx, "")
			if err != nil {
				lastListErr = err
				continue
			}
			for _, torrent := range after {
				if torrent.Hash == "" {
					continue
				}
				if _, ok := beforeHashes[torrent.Hash]; !ok {
					accepted = true
					break
				}
			}
			if accepted {
				break
			}
		}
		if !accepted {
			if lastListErr != nil {
				return fmt.Errorf("qbittorrent add: 无法确认任务已加入下载器: %w", lastListErr)
			}
			return fmt.Errorf("qbittorrent add: 下载器未出现新任务，可能种子已存在或 URL 未被下载器接受")
		}
	}
	q.log.Info("qbittorrent: torrent added",
		zap.String("url", redactTorrentURL(magnetOrURL)),
		zap.String("save_path", savePath),
		zap.String("category", sanitizeQBitCategory(category)),
		zap.Bool("file_upload", useFileUpload),
		zap.String("body", bodyText))
	return nil
}

func sanitizeQBitCategory(category string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(category, "\r", " "), "\n", " "))
}

func redactTorrentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "magnet:?xt=***"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted-download-url]"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (q *QBitClient) fetchTorrentFile(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, "", errors.New("not a remote URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", errors.New("not an HTTP torrent URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1")
	req.Header.Set("Accept", "application/x-bittorrent,application/octet-stream,*/*")

	client := NewExternalHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("torrent fetch: HTTP %d", resp.StatusCode)
	}

	const maxTorrentSize = 32 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("torrent fetch: empty body")
	}
	if len(data) > maxTorrentSize {
		return nil, "", errors.New("torrent fetch: body too large")
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return nil, "", errors.New("torrent fetch: upstream returned HTML")
	}

	name := strings.TrimSpace(path.Base(u.Path))
	if name == "" || name == "." || name == "/" {
		name = "download.torrent"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return data, name, nil
}
