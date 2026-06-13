package handler

import (
	"strings"
	"testing"
)

func TestCloudMountLibraryNameDefaultsToDirectoryBaseName(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		dir        string
		displayDir string
		want       string
	}{
		{name: "openlist directory", provider: "openlist", dir: "/国产剧", displayDir: "/国产剧", want: "国产剧"},
		{name: "nested directory", provider: "openlist", dir: "id-123", displayDir: "剧集/国产剧", want: "国产剧"},
		{name: "provider root", provider: "openlist", dir: "", displayDir: "", want: "OpenList"},
		{name: "115 root id", provider: "cloud115", dir: "0", displayDir: "", want: "115 网盘"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cloudMountLibraryName(tt.provider, tt.dir, tt.displayDir); got != tt.want {
				t.Fatalf("cloudMountLibraryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloudPlaybackDiagnosticsDoNotExposeRawRefOrURL(t *testing.T) {
	rawRef := "/剧集/国产剧/很长的敏感文件名.S01E01.mkv"
	refHash, refExt := cloudPlaybackRefFingerprint(rawRef)
	if refHash == "" || strings.Contains(rawRef, refHash) {
		t.Fatalf("ref hash should be a short fingerprint, got %q", refHash)
	}
	if refExt != ".mkv" {
		t.Fatalf("ref ext = %q, want .mkv", refExt)
	}
	if host := cloudPlaybackLinkHost("https://cdn.example.test/movie.mkv?token=secret"); host != "cdn.example.test" {
		t.Fatalf("host = %q, want cdn.example.test", host)
	}
	names := cloudPlaybackHeaderNames(map[string]string{
		"Authorization": "Bearer secret",
		"Cookie":        "sid=secret",
	})
	if got := strings.Join(names, ","); got != "Authorization,Cookie" {
		t.Fatalf("header names = %q", got)
	}
}
