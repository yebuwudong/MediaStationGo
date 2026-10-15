package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PlaybackInfo returns a PlaybackInfoResponse usable by Emby clients.
func (e *EmbyService) PlaybackInfo(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	return e.PlaybackInfoForMediaSource(ctx, mediaID, userID, "", false)
}

// PlaybackInfoForMediaSource honors the source selected by an Emby client. A
// real playback request also records that selection for the user's next item
// detail and PlaybackInfo request.
func (e *EmbyService) PlaybackInfoForMediaSource(
	ctx context.Context,
	mediaID string,
	userID string,
	requestedSourceID string,
	remember bool,
) (map[string]any, error) {
	m, err := e.playableMedia(ctx, mediaID, userID)
	if err != nil || m == nil {
		return nil, err
	}
	sources, err := e.orderMediaSourcesForUser(
		ctx,
		m,
		userID,
		e.mediaSourcesForItem(ctx, m, false, e.directPlayOnly(ctx)),
		requestedSourceID,
		remember,
	)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"MediaSources":  sources,
		"PlaySessionId": fmt.Sprintf("%s-%d", m.ID, time.Now().Unix()),
		// 官方 PlaybackInfoResponse 固定包含 ErrorCode。成功时为 null，
		// 客户端按「非 null 即失败」判断，因此必须显式给出该字段。
		"ErrorCode": nil,
	}, nil
}

func parseCloudMediaPlaybackURL(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	path := strings.Trim(u.Path, "/")
	const prefix = "api/cloud/play/"
	idx := strings.Index(strings.ToLower(path), prefix)
	if idx < 0 {
		return "", "", false
	}
	typ := strings.TrimSpace(path[idx+len(prefix):])
	ref := strings.TrimSpace(u.Query().Get("ref"))
	return typ, ref, typ != "" && ref != ""
}

// directPlayOnly reports whether the admin enabled「客户端直连解码」mode.
// In that mode the host never transcodes; clients must direct-play.
func (e *EmbyService) directPlayOnly(ctx context.Context) bool {
	if e.repo == nil || e.repo.Setting == nil {
		return false
	}
	v, err := e.repo.Setting.Get(ctx, PlaybackDirectOnlySettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

func (e *EmbyService) playableMedia(ctx context.Context, id, userID string) (*model.Media, error) {
	if season, ok, err := e.findSeasonGroup(ctx, id, userID); err != nil {
		return nil, err
	} else if ok && len(season.Episodes) > 0 {
		return &season.Episodes[0], nil
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, userID); err != nil {
		return nil, err
	} else if ok && len(series.Episodes) > 0 {
		return &series.Episodes[0], nil
	}
	m, err := e.repo.Media.FindByID(ctx, id)
	if err != nil || m == nil {
		return m, err
	}
	if !e.mediaVisibility(ctx, userID).Allows(m) {
		return nil, nil
	}
	return m, nil
}

// mediaSource 是 /Items 与 /PlaybackInfo 共享的 MediaSource 结构。
//
// asEmbedded=true：嵌在 /Items 列表里，不包含完整 stream URL（避免暴露
// 直链给搜索接口）。/PlaybackInfo 走 false 路径，URL 指向 Emby 兼容
// /Videos/{id}/stream（客户端会继续携带 X-Emby-Token 或 append api_key）。
func (e *EmbyService) mediaSource(ctx context.Context, m *model.Media, asEmbedded, directOnly bool) map[string]any {
	container := embyMediaContainer(m)
	strmTarget, _ := mediaSTRMTarget(m)
	_, localSTRM := localSTRMTargetPath(m.Path, strmTarget)
	isCloud := strmTarget != "" && !localSTRM
	playURL := e.embyMediaPlayURL(ctx, m, container, isCloud)
	if isCloud || container == "iso" {
		// Cloud/WebDAV media is already a direct/proxy stream. Advertising HLS
		// transcoding makes some Emby clients pick /master.m3u8, forcing this
		// lightweight server to pull remote bytes through ffmpeg and often
		// surfacing as "network/playback failed". ISO images require a
		// disc-capable client. Neither should advertise the generic HLS path.
		directOnly = true
	}
	// 云盘媒体的 Path 保留源路径标识(不暴露带 token 的播放地址);本地 STRM
	// 保留目标文件路径;普通本地文件的 Path 使用 Emby 流地址,供按 Path
	// 直连的第三方播放器(Protocol=Http)使用。SupportsDirectPlay/DirectStream
	// 对云盘媒体在 playURL 可用时保持 true:Infuse/Emby 官方客户端会优先挑选
	// DirectPlay 源,标 false 可能被判定为"没有可播放媒体源"。
	src := e.baseMediaSource(m, container, isCloud, playURL, directOnly)
	if !asEmbedded {
		if doc := loadMediaProbeDocument(ctx, e.repo, m); doc != nil {
			src["MediaStreams"] = embyProbeStreams(doc)
			src["DefaultAudioStreamIndex"], src["DefaultSubtitleStreamIndex"] = probeDefaultStreamIndexes(doc)
		}
	}
	if !isCloud && !localSTRM && playURL != "" {
		src["Path"] = playURL
	}
	if playURL != "" {
		src["DirectStreamUrl"] = playURL
		// 直链是否需要客户端附带凭据。DirectStreamUrl 会由 handler 追加
		// api_key，客户端据此决定是否复用已有令牌。
		src["AddApiKeyToDirectStreamUrl"] = true
		// 直连解码模式下不下发 TranscodingUrl，迫使客户端本地解码直连，
		// 宿主机不参与转码。
		if !asEmbedded && !directOnly {
			src["TranscodingUrl"] = "/Videos/" + m.ID + "/master.m3u8"
			// HLS 客户端靠这两个字段判断转码子协议与分片容器；缺失时部分
			// 播放器不会走 m3u8 路径。官方 Emby 在此固定下发 hls/ts。
			src["TranscodingSubProtocol"] = "hls"
			src["TranscodingContainer"] = "ts"
		}
	}
	if isCloud && playURL != "" {
		// STRM / cloud:// media must stay behind a token-aware endpoint. When
		// STRM playback is enabled we expose /api/stream so third-party clients
		// follow the same STRM entry as generated .strm files; when disabled we
		// expose /Videos/{id}/stream so playback uses the Emby 302/proxy path.
		src["IsRemote"] = true
	}
	if !asEmbedded {
		e.attachExternalSubtitleStreams(ctx, m, src)
	}
	return src
}

func (e *EmbyService) baseMediaSource(m *model.Media, container string, isCloud bool, playURL string, directOnly bool) map[string]any {
	size := m.SizeBytes
	if target, err := mediaSTRMTarget(m); err == nil {
		if local, ok := localSTRMTargetPath(m.Path, target); ok {
			if _, info, err := resolveAccessibleMappedPath(local); err == nil && info.Mode().IsRegular() {
				size = info.Size()
			}
		}
	}
	// MimeType / TranscodingMimeType：部分播放器不读 Container，只看 MIME
	// 判断能否直连播放。缺失会导致客户端把可播媒体判定为不可播。
	mimeType := MediaMimeType(m.Path)
	// AddApiKeyToDirectStreamUrl 告知客户端「这个直链需要附带 api_key」。
	// 官方 Emby 在 DirectStreamUrl 已含 api_key 时通常给 true。
	return map[string]any{
		"Id":                    m.ID,
		"Name":                  m.Title,
		"Path":                  embyMediaSourcePath(m),
		"Container":             container,
		"Size":                  size,
		"Bitrate":               effectiveMediaBitRate(m.BitRate, m.SizeBytes, m.DurationSec),
		"Protocol":              "Http",
		"Type":                  "Default",
		"IsRemote":              isCloud,
		"MimeType":              mimeType,
		"TranscodingMimeType":   hlsPlaylistMimeType,
		"RequiresOpening":       false,
		"RequiresClosing":       false,
		"ReadAtNativeFramerate": false,
		"SupportsTranscoding":   !directOnly,
		"SupportsDirectStream":  !isCloud || playURL != "",
		"SupportsDirectPlay":    !isCloud || playURL != "",
		"SupportsProbing":       true,
		"HasMixedProtocols":     false,
		"RunTimeTicks":          int64(m.DurationSec) * 10_000_000,
		"MediaStreams":          e.mediaStreams(m),
		// 官方契约要求给出这两个数组；缺失时部分客户端在解析阶段报错。
		// 内嵌字幕/字体附件当前来自探测文档，无数据时给空数组。
		"MediaAttachments": e.mediaAttachments(m),
		"Chapters":         []any{},
	}
}

// mediaAttachments 返回内嵌字幕/字体等附件流。
//
// Emby 的 MediaAttachments 通常承载内嵌 ASS 字体、内嵌字幕等，客户端用它
// 在播放前建立字幕轨列表。当前实现从探测文档提取；没有探测数据时返回空
// 数组（而非省略字段），避免客户端把「字段缺失」与「无附件」混淆。
func (e *EmbyService) mediaAttachments(m *model.Media) []map[string]any {
	attachments := []map[string]any{}
	if e == nil || m == nil {
		return attachments
	}
	doc := loadMediaProbeDocument(context.Background(), e.repo, m)
	if doc == nil {
		return attachments
	}
	for _, stream := range embyProbeStreams(doc) {
		if !strings.EqualFold(mediaStreamString(stream, "Type"), "Attachment") {
			continue
		}
		attachments = append(attachments, stream)
	}
	return attachments
}

// mediaStreamString 从 MediaStream map 中取字符串字段。
func mediaStreamString(stream map[string]any, key string) string {
	if stream == nil {
		return ""
	}
	value, _ := stream[key].(string)
	return value
}

// embyMediaSourcePath keeps Emby's Path field as a source identity rather
// than a playback endpoint. OpenList and other path-based cloud providers
// carry the real provider path in the internal cloud-play ref query. For
// opaque provider refs, fall back to the display path stored in cloud://.
func embyMediaSourcePath(m *model.Media) string {
	if m == nil {
		return ""
	}
	if target, err := mediaSTRMTarget(m); err == nil {
		if local, ok := localSTRMTargetPath(m.Path, target); ok {
			return local
		}
	}
	if _, ref, ok := parseCloudMediaPlaybackURL(m.STRMURL); ok && strings.HasPrefix(ref, "/") {
		return ref
	}
	raw := strings.TrimSpace(m.Path)
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "cloud") {
		return m.Path
	}
	sourcePath := strings.TrimSpace(u.Path)
	if decoded, decodeErr := url.PathUnescape(sourcePath); decodeErr == nil {
		sourcePath = decoded
	}
	if sourcePath == "" {
		return "/"
	}
	if !strings.HasPrefix(sourcePath, "/") {
		sourcePath = "/" + sourcePath
	}
	return sourcePath
}

func embyMediaContainer(m *model.Media) string {
	// embyPlaybackContainer 归一化 ffprobe 风格的容器列表(如 "mov,mp4,m4a")
	// 与别名(matroska→mkv),并优先采用路径扩展名。
	container := embyPlaybackContainer(m.Container, m.Path)
	if container == "strm" || container == "" {
		if targetContainer := strmTargetContainer(m); targetContainer != "" {
			return targetContainer
		}
	}
	if container == "" && strings.TrimSpace(m.STRMURL) != "" {
		return "strm"
	}
	return container
}

func (e *EmbyService) embyMediaPlayURL(ctx context.Context, m *model.Media, container string, isCloud bool) string {
	if !isCloud || CloudPlaybackMode(ctx, e.repo) != "" {
		return embyDirectStreamURL(m.ID, container)
	}
	return ""
}
