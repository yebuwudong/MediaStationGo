package handler

import "testing"

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
