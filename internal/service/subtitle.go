// Package service — subtitle handling.
//
// SubtitleService finds external subtitle files next to a media file and
// converts SRT to WebVTT on the fly so the browser <track> element can
// load them directly.
//
// External-subtitle discovery rules (matching the legacy Python defaults):
//
//  1. Same directory, same basename, different extension.
//  2. Same directory, ".sub/" or "subs/" subdirectory.
//  3. Sibling languages e.g. movie.zh.srt / movie.en.srt → exposed as
//     ?lang=zh / ?lang=en.
//
// Supported extensions: .srt, .ass, .ssa, .vtt.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// SubtitleService is the discovery + conversion entry point.
type SubtitleService struct {
	log     *zap.Logger
	repo    *repository.Container
	storage *StorageConfigService
}

// NewSubtitleService is the constructor.
func NewSubtitleService(log *zap.Logger, repo *repository.Container, storage ...*StorageConfigService) *SubtitleService {
	s := &SubtitleService{log: log, repo: repo}
	if len(storage) > 0 {
		s.storage = storage[0]
	}
	return s
}

func (s *SubtitleService) SetStorageConfig(storage *StorageConfigService) {
	if s != nil {
		s.storage = storage
	}
}

// SubtitleTrack describes one external subtitle file.
type SubtitleTrack struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	Path  string `json:"path"`
	URL   string `json:"url"`
	Codec string `json:"codec"`
}

// extToCodec maps the file extension to the inner codec name.
var extToCodec = map[string]string{
	".srt": "srt",
	".vtt": "vtt",
	".ass": "ass",
	".ssa": "ssa",
}

// Discover lists every external subtitle file for a media row. The URL is
// relative; the caller should prepend /api/subtitles/<media_id>?path=...
// when serializing for the frontend.
func (s *SubtitleService) Discover(ctx context.Context, mediaID string) ([]SubtitleTrack, error) {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("media not found")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		return s.discoverCloud(ctx, *m), nil
	}
	dir := filepath.Dir(m.Path)
	base := strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path))

	candidates := make([]string, 0, 16)
	candidates = append(candidates, dir)
	for _, sub := range []string{"subs", "Subs", "sub", ".sub"} {
		candidates = append(candidates, filepath.Join(dir, sub))
	}

	tracks := make([]SubtitleTrack, 0)
	for _, c := range candidates {
		entries, err := os.ReadDir(c)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			codec, ok := extToCodec[ext]
			if !ok {
				continue
			}
			fullName := strings.TrimSuffix(e.Name(), ext)
			if !strings.HasPrefix(strings.ToLower(fullName), strings.ToLower(base)) &&
				c == dir {
				// In the same directory we require a basename match;
				// inside subs/ subdirs we accept anything.
				continue
			}
			lang := detectLang(fullName, base)
			tracks = append(tracks, SubtitleTrack{
				Lang:  lang,
				Label: lang,
				Path:  filepath.Join(c, e.Name()),
				Codec: codec,
			})
		}
	}
	return tracks, nil
}

func (s *SubtitleService) discoverCloud(ctx context.Context, m model.Media) []SubtitleTrack {
	if s == nil || s.storage == nil {
		return []SubtitleTrack{}
	}
	typ, mediaRef, ok := cloudSubtitleMediaRef(m)
	if !ok {
		return []SubtitleTrack{}
	}
	dirRef, mediaName := splitCloudRef(mediaRef)
	if mediaName == "" {
		return []SubtitleTrack{}
	}
	base := strings.TrimSuffix(mediaName, filepath.Ext(mediaName))
	entries, err := s.storage.CloudList(ctx, typ, dirRef)
	if err != nil {
		if s.log != nil {
			s.log.Debug("list cloud subtitles failed", zap.String("provider", typ), zap.String("dir", dirRef), zap.Error(err))
		}
		return []SubtitleTrack{}
	}
	tracks := cloudSubtitleTracks(typ, entries, base, false)
	for _, entry := range entries {
		if !entry.IsDir || !isSubtitleDirectory(entry.Name) || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		subEntries, err := s.storage.CloudList(ctx, typ, entry.ID)
		if err != nil {
			continue
		}
		tracks = append(tracks, cloudSubtitleTracks(typ, subEntries, base, true)...)
	}
	return tracks
}

func cloudSubtitleTracks(typ string, entries []cloud.FileEntry, base string, subdir bool) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0)
	baseLower := strings.ToLower(base)
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name))
		codec, ok := extToCodec[ext]
		if !ok {
			continue
		}
		fullName := strings.TrimSuffix(entry.Name, ext)
		if !subdir && !strings.HasPrefix(strings.ToLower(fullName), baseLower) {
			continue
		}
		ref := cloudEntryRef(typ, entry.ID, entry.PickCode)
		if ref == "" {
			continue
		}
		lang := detectLang(fullName, base)
		tracks = append(tracks, SubtitleTrack{
			Lang:  lang,
			Label: lang,
			Path:  buildCloudSubtitlePath(typ, ref, entry.Name),
			Codec: codec,
		})
	}
	return tracks
}

func cloudSubtitleMediaRef(m model.Media) (typ, ref string, ok bool) {
	if info, parsed := ParseCloudLibraryMount(m.Path); parsed && strings.TrimSpace(info.DisplayDir) != "" {
		return info.Provider, info.DisplayDir, true
	}
	if typ, ref, parsed := parseCloudMediaPlaybackURL(m.STRMURL); parsed {
		return typ, ref, true
	}
	return "", "", false
}

func splitCloudRef(ref string) (dir, name string) {
	ref = strings.Trim(strings.ReplaceAll(strings.TrimSpace(ref), "\\", "/"), "/")
	if ref == "" {
		return "", ""
	}
	idx := strings.LastIndex(ref, "/")
	if idx < 0 {
		return "", ref
	}
	return ref[:idx], ref[idx+1:]
}

func isSubtitleDirectory(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "subs", "sub", ".sub", "subtitles", "subtitle":
		return true
	default:
		return false
	}
}

func buildCloudSubtitlePath(typ, ref, name string) string {
	u := url.URL{
		Scheme: "cloud",
		Host:   strings.TrimSpace(typ),
		Path:   "/" + strings.TrimLeft(strings.TrimSpace(ref), "/"),
	}
	q := u.Query()
	q.Set("name", strings.TrimSpace(name))
	u.RawQuery = q.Encode()
	return u.String()
}

func parseCloudSubtitlePath(raw string) (typ, ref, name string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.ToLower(u.Scheme) != "cloud" || strings.TrimSpace(u.Host) == "" {
		return "", "", "", false
	}
	ref = strings.TrimLeft(u.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	return strings.TrimSpace(u.Host), strings.TrimSpace(ref), strings.TrimSpace(u.Query().Get("name")), ref != ""
}

// langTag matches the .zh / .zh-cn / .chs language sub-extensions.
var langTag = regexp.MustCompile(`(?i)\.([a-z]{2,3}(?:[-_][a-z]{2,4})?)$`)

func detectLang(name, base string) string {
	suffix := strings.TrimPrefix(name, base)
	suffix = strings.TrimPrefix(suffix, ".")
	if m := langTag.FindStringSubmatch("." + suffix); len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	if suffix == "" {
		return "und" // undetermined
	}
	return strings.ToLower(suffix)
}

// Serve writes the subtitle file as WebVTT (.vtt). SRT/SSA files are
// converted minimally on the fly. Returns ErrSubtitleNotFound when the
// path is rejected (path traversal / not in the media directory).
func (s *SubtitleService) Serve(ctx context.Context, mediaID, sub string, w io.Writer) error {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return errors.New("media not found")
	}
	if typ, ref, name, ok := parseCloudSubtitlePath(sub); ok {
		return s.serveCloud(ctx, *m, typ, ref, name, w)
	}
	abs, err := filepath.Abs(sub)
	if err != nil {
		return err
	}
	mediaDir, _ := filepath.Abs(filepath.Dir(m.Path))
	if !pathWithin(abs, mediaDir) {
		return fmt.Errorf("path escape")
	}

	f, err := os.Open(abs) // #nosec G304 -- abs is constrained to the media file directory with pathWithin.
	if err != nil {
		return err
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	switch strings.ToLower(filepath.Ext(abs)) {
	case ".vtt":
		_, err = w.Write(body)
	case ".srt":
		_, err = w.Write([]byte(srtToVTT(string(body))))
	case ".ass", ".ssa":
		_, err = w.Write([]byte(assToVTT(string(body))))
	default:
		return errors.New("unsupported subtitle format")
	}
	return err
}

func (s *SubtitleService) serveCloud(ctx context.Context, m model.Media, typ, ref, name string, w io.Writer) error {
	if s == nil || s.storage == nil {
		return errors.New("cloud storage service unavailable")
	}
	mediaTyp, _, ok := cloudSubtitleMediaRef(m)
	if !ok || mediaTyp != typ {
		return ErrCloudPlaybackUnavailable
	}
	allowed := false
	for _, track := range s.discoverCloud(ctx, m) {
		if track.Path == buildCloudSubtitlePath(typ, ref, name) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("path escape")
	}
	body, err := s.storage.CloudReadText(ctx, typ, ref, 8<<20)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(firstNonEmpty(name, ref)))
	switch ext {
	case ".vtt":
		_, err = io.WriteString(w, body)
	case ".srt":
		_, err = io.WriteString(w, srtToVTT(body))
	case ".ass", ".ssa":
		_, err = io.WriteString(w, assToVTT(body))
	default:
		return errors.New("unsupported subtitle format")
	}
	return err
}

// srtToVTT performs the minimal SRT → WebVTT transformation: prepend
// "WEBVTT\n\n" and replace ',' with '.' in the timecode separators.
func srtToVTT(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	out := strings.Builder{}
	out.WriteString("WEBVTT\n\n")
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "-->") {
			line = strings.ReplaceAll(line, ",", ".")
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// assToVTT extracts the dialogue lines from an ASS/SSA subtitle. Styling
// is dropped — the goal is to produce something usable in <track> rather
// than a pixel-perfect render.
func assToVTT(body string) string {
	out := strings.Builder{}
	out.WriteString("WEBVTT\n\n")
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		parts := strings.SplitN(line, ",", 10)
		if len(parts) < 10 {
			continue
		}
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n",
			i,
			normaliseTimecode(parts[1]),
			normaliseTimecode(parts[2]),
			stripASSTags(parts[9]),
		)
	}
	return out.String()
}

func normaliseTimecode(t string) string {
	t = strings.TrimSpace(t)
	parts := strings.Split(t, ":")
	if len(parts) != 3 {
		return t
	}
	hh := parts[0]
	if len(hh) == 1 {
		hh = "0" + hh
	}
	return hh + ":" + parts[1] + ":" + strings.ReplaceAll(parts[2], ".", ".")
}

var assTag = regexp.MustCompile(`\{[^}]*\}`)

func stripASSTags(s string) string {
	return assTag.ReplaceAllString(s, "")
}
