// Package service — Bark 通知 Provider。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BarkProvider 通过 Bark 推送通知到 iOS 设备。
// Bark API 文档: https://github.com/Finb/bark-server
type BarkProvider struct{}

// Send 发送 Bark 推送通知。
func (p *BarkProvider) Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error {
	serverURL := cfg["server_url"]
	deviceKey := cfg["device_key"]
	if serverURL == "" {
		serverURL = "https://api.day.app"
	}
	serverURL = strings.TrimRight(serverURL, "/")
	if deviceKey == "" {
		return fmt.Errorf("bark: device_key is required")
	}

	payload := map[string]interface{}{
		"title": event.Title,
		"body":  event.Message,
		"group": "MediaStationGo",
	}

	if len(event.Data) > 0 {
		var extra string
		for k, v := range event.Data {
			extra += fmt.Sprintf("%s: %v\n", k, v)
		}
		payload["body"] = event.Message + "\n\n" + extra
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("%s/%s", serverURL, deviceKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("bark api error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ValidateConfig 验证 Bark 配置。
func (p *BarkProvider) ValidateConfig(cfg map[string]string) error {
	if cfg["device_key"] == "" {
		return fmt.Errorf("bark: device_key is required")
	}
	return nil
}
