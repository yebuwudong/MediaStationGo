package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestGenerateSTRMFromTreePaths(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider:   "115",
		Paths:      []string{"/电视剧/国产剧/南部档案/Season 01/Archives.S01E01.mkv", "/电视剧/国产剧/南部档案/poster.jpg", "/电视剧/国产剧/南部档案/Existing.strm"},
		TreeText:   "电视剧\n└── 国产剧\n    └── 南部档案\n        └── Existing.Tree.strm",
		SourceRoot: "/电视剧",
		OutputDir:  outDir,
		BaseURL:    "https://media.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Skipped != 0 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want one generated video and ignored sidecar", res)
	}
	path := filepath.Join(outDir, "国产剧", "南部档案", "Season 01", "Archives.S01E01.strm")
	got := readSTRM(t, path)
	if !strings.HasPrefix(got, "https://media.example.com/api/cloud/play/cloud115?") {
		t.Fatalf("strm url = %q, want cloud115 play url", got)
	}
	if !strings.Contains(got, "ref=%2F%E7%94%B5%E8%A7%86%E5%89%A7%2F%E5%9B%BD%E4%BA%A7%E5%89%A7%2F%E5%8D%97%E9%83%A8%E6%A1%A3%E6%A1%88%2FSeason+01%2FArchives.S01E01.mkv") {
		t.Fatalf("strm url = %q, missing encoded source ref", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "国产剧", "南部档案", "Existing.strm")); !os.IsNotExist(err) {
		t.Fatalf("existing .strm source should be ignored by tree generator, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "电视剧", "国产剧", "南部档案", "Existing.Tree.strm")); !os.IsNotExist(err) {
		t.Fatalf("tree .strm source should be ignored by tree generator, stat err=%v", err)
	}
}

func TestGenerateSTRMFromTreeSupportsCommonVideoExtensions(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "strm")
	svc := NewSTRMService(zap.NewNop(), nil, nil)

	res, err := svc.GenerateFromTree(t.Context(), GenerateSTRMTreeOptions{
		Provider: "openlist",
		Paths: []string{
			"/Movies/BluRay.Stream.2026.m2ts",
			"/Movies/Camera.Source.2026.MTS",
			"/Movies/DVD.Feature.2026.vob",
			"/Movies/Legacy.Video.2026.wmv",
			"/Movies/Web.Legacy.2026.flv",
			"/Movies/Disc.Image.2026.iso",
		},
		OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 5 || len(res.Errors) != 0 {
		t.Fatalf("result = %#v, want five common video sources generated and iso ignored", res)
	}
	for _, name := range []string{
		"BluRay.Stream.2026",
		"Camera.Source.2026",
		"DVD.Feature.2026",
		"Legacy.Video.2026",
		"Web.Legacy.2026",
	} {
		got := readSTRM(t, filepath.Join(outDir, "Movies", name+".strm"))
		if !strings.Contains(got, "/api/cloud/play/openlist?") {
			t.Fatalf("%s strm url = %q, want cloud play url", name, got)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "Movies", "Disc.Image.2026.strm")); !os.IsNotExist(err) {
		t.Fatalf("iso source should stay ignored by tree generator, stat err=%v", err)
	}
}
