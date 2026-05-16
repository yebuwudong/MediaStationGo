// Package service — Server酱(WeChat) 通知 Provider。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WechatProvider 通过 Server酱 API 推送消息到微信。
// Server酱 API 文档: https://sct.ftqq.com/
type WechatProvider struct{}

// Send 发送 Server酱 推送消息。
func (p *WechatProvider) Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error {
	sendkey := cfg["sendkey"]
	if sendkey == "" {
		return fmt.Errorf("wechat: sendkey is required")
	}

	payload := map[string]string{
		"title": event.Title,
		"desp":  event.Message,
	}
	if len(event.Data) > 0 {
		payload["desp"] += "\n\n---\n\n"
		for k, v := range event.Data {
			payload["desp"] += fmt.Sprintf("- **%s**: %v\n", k, v)
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	apiURL := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", sendkey)
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
		return fmt.Errorf("wechat server酱 api error %d: %s", resp.StatusCode, string(respBody))
	}

	// 检查 Server酱 响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err == nil {
		if code, ok := result["code"].(float64); ok && code != 0 {
			msg, _ := result["message"].(string)
			return fmt.Errorf("wechat server酱 error: %s", msg)
		}
	}
	return nil
}

// ValidateConfig 验证 Server酱 配置。
func (p *WechatProvider) ValidateConfig(cfg map[string]string) error {
	if cfg["sendkey"] == "" {
		return fmt.Errorf("wechat: sendkey is required")
	}
	return nil
}
