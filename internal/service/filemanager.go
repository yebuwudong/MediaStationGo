// Package service — server-side file browser.
//
// FileManagerService exposes a strict, allow-listed view of the server's
// filesystem so the React Library / Storage tabs can let the operator
// pick library roots without typing absolute paths from memory.
//
// Allow-list rules:
//
//   - Roots: every Library.Path + the configured app.data_dir +
//     app.cache_dir, plus the operator-supplied app.media.* defaults.
//   - Children must resolve under one of the roots after symlink-free
//     filepath.Abs(). Anything else returns ErrPathOutOfBounds.
//
// We never write to the filesystem here; this is read-only browsing.
package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// FileManagerService browses the server-side filesystem.
type FileManagerService struct {
	cfg  *config.Config
	log  *zap.Logger
	repo *repository.Container
}

// NewFileManagerService is the constructor.
func NewFileManagerService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *FileManagerService {
	return &FileManagerService{cfg: cfg, log: log, repo: repo}
}

// Entry is one file or directory shown in the browser.
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Modified int64 `json:"modified"`
}

// Listing describes the contents of a directory plus navigation hints.
type Listing struct {
	Path    string  `json:"path"`
	Parent  string  `json:"parent,omitempty"`
	Roots   []Root  `json:"roots,omitempty"`
	Entries []Entry `json:"entries"`
}

// Root is the entry-point label shown when no path is given.
type Root struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

// ErrPathOutOfBounds is returned when path falls outside every allowed root.
var ErrPathOutOfBounds = errors.New("path is outside the allowed roots")

// List enumerates a directory under one of the allowed roots, returning
// up to maxEntries items sorted by (dir-first, alphabetical).
func (s *FileManagerService) List(path string, maxEntries int) (*Listing, error) {
	if maxEntries <= 0 || maxEntries > 5000 {
		maxEntries = 1000
	}
	roots, err := s.allowedRoots()
	if err != nil {
		return nil, err
	}
	rootList := make([]Root, 0, len(roots))
	seen := map[string]struct{}{}
	for label, p := range roots {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		rootList = append(rootList, Root{Label: label, Path: p})
	}
	sort.Slice(rootList, func(i, j int) bool { return rootList[i].Label < rootList[j].Label })

	if path == "" {
		// Listing the (virtual) root: just hand back the labels.
		return &Listing{Path: "", Roots: rootList}, nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !s.withinAllowed(abs, roots) {
		return nil, ErrPathOutOfBounds
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := &Listing{Path: abs, Roots: rootList}
	parent := filepath.Dir(abs)
	if parent != abs && s.withinAllowed(parent, roots) {
		out.Parent = parent
	}

	for i, e := range entries {
		if i >= maxEntries {
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(abs, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		out.Entries = append(out.Entries, Entry{
			Name:     name,
			Path:     full,
			IsDir:    e.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime().Unix(),
		})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		if out.Entries[i].IsDir != out.Entries[j].IsDir {
			return out.Entries[i].IsDir
		}
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	return out, nil
}

// allowedRoots returns the union of {libraries, data_dir, cache_dir,
// media.movies/tv/anime} as label → absolute-path.
func (s *FileManagerService) allowedRoots() (map[string]string, error) {
	roots := map[string]string{}
	add := func(label, p string) {
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return
		}
		if _, err := os.Stat(abs); err != nil {
			return
		}
		roots[label] = abs
	}
	add("data", s.cfg.App.DataDir)
	add("cache", s.cfg.Cache.CacheDir)
	add("movies", s.cfg.Media.MoviesDir)
	add("tv", s.cfg.Media.TVDir)
	add("anime", s.cfg.Media.AnimeDir)
	libs, err := s.repo.Library.List(context.Background()) // librarian list is fast; ctx not propagated from request
	if err == nil {
		for _, l := range libs {
			add("library:"+l.Name, l.Path)
		}
	}
	return roots, nil
}

// withinAllowed reports whether path lives under any allowed root.
func (s *FileManagerService) withinAllowed(path string, roots map[string]string) bool {
	for _, r := range roots {
		rel, err := filepath.Rel(r, path)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
