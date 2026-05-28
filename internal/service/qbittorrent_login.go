package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type qbitLoginVariant struct {
	name    string
	referer bool
	origin  bool
}

func qbitLogin(ctx context.Context, client *http.Client, baseURL, username, password string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return errors.New("qbittorrent host not configured")
	}
	var lastErr error
	for _, variant := range []qbitLoginVariant{
		{name: "minimal"},
		{name: "referer", referer: true},
		{name: "referer-origin", referer: true, origin: true},
	} {
		err := qbitLoginOnce(ctx, client, baseURL, username, password, variant)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, errQbitBadCredentials) {
			return err
		}
	}
	return lastErr
}

var errQbitBadCredentials = errors.New("qbittorrent: 用户名/密码错误")

func qbitLoginOnce(ctx context.Context, client *http.Client, baseURL, username, password string, variant qbitLoginVariant) error {
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if variant.referer {
		req.Header.Set("Referer", baseURL)
	}
	if variant.origin {
		req.Header.Set("Origin", baseURL)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	text := strings.TrimSpace(string(raw))
	switch {
	case resp.StatusCode == http.StatusOK && text == "Ok.":
		return nil
	case resp.StatusCode == http.StatusOK && text == "Fails.":
		return errQbitBadCredentials
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("qbittorrent: 403 forbidden during %s login, body=%q; check qBittorrent WebUI bypass/auth settings for the container IP and host %s", variant.name, text, baseURL)
	case resp.StatusCode >= 400:
		return fmt.Errorf("qbittorrent login failed during %s login: status=%d body=%q", variant.name, resp.StatusCode, text)
	default:
		return fmt.Errorf("qbittorrent login unexpected response during %s login: status=%d body=%q", variant.name, resp.StatusCode, text)
	}
}
