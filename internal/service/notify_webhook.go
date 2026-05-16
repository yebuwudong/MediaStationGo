// Package service — Webhook 通知 Provider。
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookProvider 通过自定义 HTTP Webhook 发送通知。
// 支持自定义 HTTP 方法和请求头。
type WebhookProvider struct{}

// Send 发送 Webhook 通知。
func (p *WebhookProvider) Send(ctx context.Context, cfg map[string]string, event NotifyEvent) error {
	webhookURL := cfg["url"]
	if webhookURL == "" {
		return fmt.Errorf("webhook: url is required")
	}

	method := cfg["method"]
	if method == "" {
		method = "POST"
	}
	method = strings.ToUpper(method)

	// 构建请求体
	bodyTemplate := cfg["body_template"]
	var bodyStr string
	if bodyTemplate != "" {
		bodyStr = renderTemplate(bodyTemplate, event)
	} else {
		// 默认 JSON 格式
		bodyStr = fmt.Sprintf(`{"type":"%s","title":"%s","message":"%s","data":{}}`,
			event.Type, event.Title, event.Message)
		if len(event.Data) > 0 {
			var dataParts []string
			for k, v := range event.Data {
				dataParts = append(dataParts, fmt.Sprintf(`"%s":%v`, k, v))
			}
			bodyStr = fmt.Sprintf(`{"type":"%s","title":"%s","message":"%s","data":{%s}}`,
				event.Type, event.Title, event.Message, strings.Join(dataParts, ","))
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, webhookURL, strings.NewReader(bodyStr))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	// 自定义请求头
	headersJSON := cfg["headers_json"]
	if headersJSON != "" {
		headers := parseHeadersJSON(headersJSON)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// ValidateConfig 验证 Webhook 配置。
func (p *WebhookProvider) ValidateConfig(cfg map[string]string) error {
	if cfg["url"] == "" {
		return fmt.Errorf("webhook: url is required")
	}
	return nil
}

// renderTemplate 简单模板渲染，支持 {{title}}, {{message}}, {{type}} 占位符。
func renderTemplate(template string, event NotifyEvent) string {
	result := template
	result = strings.ReplaceAll(result, "{{title}}", event.Title)
	result = strings.ReplaceAll(result, "{{message}}", event.Message)
	result = strings.ReplaceAll(result, "{{type}}", event.Type)
	return result
}

// parseHeadersJSON 简单解析 headers JSON（格式: {"key":"value",...}）。
func parseHeadersJSON(jsonStr string) map[string]string {
	result := make(map[string]string)
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || (jsonStr[0] != '{' && jsonStr[len(jsonStr)-1] != '}') {
		return result
	}

	// 简单 key:value 解析
	inner := jsonStr[1 : len(jsonStr)-1]
	parts := strings.Split(inner, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(kv[0]), `"`)
		value := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		if key != "" {
			result[key] = value
		}
	}
	return result
}
