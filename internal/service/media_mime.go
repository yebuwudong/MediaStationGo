package service

import (
	"mime"
	"path/filepath"
	"strings"
	"sync"
)

// 媒体文件 MIME 解析。
//
// 背景：Go 的内置 MIME 表很小，且 mime.TypeByExtension 会读取宿主机的
// /etc/mime.types（若存在）。Alpine 基础镜像默认不安装该文件（也不安装
// mailcap），于是容器内对 .mkv/.mov/.avi/.ts/.m2ts/.iso/.srt/.ass 等扩展名
// 一律返回空字符串。配合 stream_file.go 设置的
// X-Content-Type-Options: nosniff，客户端会拿到
// Content-Type: application/octet-stream，部分播放器因此拒绝播放或退化为
// 下载。
//
// 这里维护一份内置表并优先使用，使 MIME 结果不依赖宿主环境。

// mediaMimeTypes 是内置的扩展名 → MIME 映射。
// 覆盖视频容器、音频、字幕、图片与常见旁挂文件。
var mediaMimeTypes = map[string]string{
	// ---- 视频容器 ----
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".mkv":  "video/x-matroska",
	".mk3d": "video/x-matroska",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".qt":   "video/quicktime",
	".avi":  "video/x-msvideo",
	".wmv":  "video/x-ms-wmv",
	".asf":  "video/x-ms-asf",
	".flv":  "video/x-flv",
	".f4v":  "video/x-f4v",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".m2v":  "video/mpeg",
	".vob":  "video/dvd",
	".ts":   "video/mp2t",
	".m2ts": "video/mp2t",
	".mts":  "video/mp2t",
	".tp":   "video/mp2t",
	".trp":  "video/mp2t",
	".3gp":  "video/3gpp",
	".3g2":  "video/3gpp2",
	".ogv":  "video/ogg",
	".rmvb": "application/vnd.rn-realmedia-vbr",
	".rm":   "application/vnd.rn-realmedia",
	".divx": "video/divx",
	".mxf":  "application/mxf",
	".iso":  "application/x-iso9660-image",
	".img":  "application/octet-stream",

	// ---- 音频 ----
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".aac":  "audio/aac",
	".m4a":  "audio/mp4",
	".m4b":  "audio/mp4",
	".opus": "audio/opus",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".wav":  "audio/wav",
	".wma":  "audio/x-ms-wma",
	".ape":  "audio/x-ape",
	".alac": "audio/x-alac",
	".dsf":  "audio/x-dsf",
	".dff":  "audio/x-dff",
	".aif":  "audio/x-aiff",
	".aiff": "audio/x-aiff",
	".mka":  "audio/x-matroska",
	".ac3":  "audio/ac3",
	".eac3": "audio/eac3",
	".dts":  "audio/vnd.dts",
	".truehd": "audio/true-hd",
	".amr":  "audio/amr",

	// ---- 字幕 ----
	".srt":  "application/x-subrip",
	".vtt":  "text/vtt; charset=utf-8",
	".webvtt": "text/vtt; charset=utf-8",
	".ass":  "text/x-ssa; charset=utf-8",
	".ssa":  "text/x-ssa; charset=utf-8",
	".sub":  "text/plain; charset=utf-8",
	".idx":  "text/plain; charset=utf-8",
	".sup":  "application/x-pgs",
	".pgs":  "application/x-pgs",
	".smi":  "application/smil",
	".sami": "application/smil",
	".ttml": "application/ttml+xml",
	".dfxp": "application/ttml+xml",

	// ---- 图片 ----
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".jfif": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".avif": "image/avif",
	".heic": "image/heic",
	".svg":  "image/svg+xml",

	// ---- 媒体旁挂/元数据 ----
	".nfo":  "text/xml; charset=utf-8",
	".xml":  "text/xml; charset=utf-8",
	".json": "application/json",
	".txt":  "text/plain; charset=utf-8",
	".cue":  "text/plain; charset=utf-8",
	".m3u":  "audio/x-mpegurl",
	".m3u8": "application/vnd.apple.mpegurl",
	".pls":  "audio/x-scpls",
	".log":  "text/plain; charset=utf-8",
}

// mpegTSMimeType 是 HLS 分片与 MPEG-TS 流的 MIME。
const mpegTSMimeType = "video/mp2t"

// hlsPlaylistMimeType 是 HLS 播放列表的 MIME。
const hlsPlaylistMimeType = "application/vnd.apple.mpegurl"

// fallbackMimeType 是既无内置映射、宿主也无 mime.types 时的兜底类型。
// 这是唯一允许出现 octet-stream 的场景。
const fallbackMimeType = "application/octet-stream"

var registerBuiltinMimeOnce sync.Once

// registerBuiltinMediaMimeTypes 把内置表注册进 mime 包，使
// mime.TypeByExtension 在缺少 /etc/mime.types 的环境下也能返回正确结果。
// 只注册一次；mime.AddExtensionType 会覆盖旧值，因此这里优先于宿主表。
func registerBuiltinMediaMimeTypes() {
	registerBuiltinMimeOnce.Do(func() {
		for ext, typ := range mediaMimeTypes {
			// 忽略非法项；内置表是常量，实际不会失败。
			_ = mime.AddExtensionType(ext, typ)
		}
	})
}

// MediaMimeType 返回媒体文件应使用的 MIME 类型。
//
// 解析顺序：
//  1. 内置表（不依赖宿主环境，优先保证视频容器正确）
//  2. 注册后的 mime.TypeByExtension（覆盖内置表未收录的扩展名）
//  3. application/octet-stream 兜底
//
// 路径可以带查询串；调用方传入原始路径即可。
func MediaMimeType(path string) string {
	registerBuiltinMediaMimeTypes()

	ext := strings.ToLower(filepath.Ext(trimQueryPrefix(path)))
	if ext != "" {
		if typ, ok := mediaMimeTypes[ext]; ok {
			return typ
		}
		if typ := mime.TypeByExtension(ext); typ != "" {
			return typ
		}
	}
	return fallbackMimeType
}

// trimQueryPrefix 去掉 URL 查询串与片段，便于取扩展名。
func trimQueryPrefix(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// HLSPlaylistMimeType 返回 HLS 播放列表 MIME。
func HLSPlaylistMimeType() string { return hlsPlaylistMimeType }

// MPEGTSMimeType 返回 MPEG-TS（含 HLS 分片）MIME。
func MPEGTSMimeType() string { return mpegTSMimeType }
