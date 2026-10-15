package service

import (
	"encoding/json"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TestMediaMimeTypeCoversVideoContainersWithoutHostMimeTypes 固定「内置 MIME 表」契约。
//
// 依据：Alpine 基础镜像不含 /etc/mime.types（也未安装 mailcap），Go 内置
// MIME 表对 .mkv/.mov/.avi/.ts/.m2ts/.iso 等返回空值。配合
// X-Content-Type-Options: nosniff，客户端会收到 application/octet-stream，
// 部分播放器据此拒播。本测试保证这些扩展名始终解析出正确类型。
func TestMediaMimeTypeCoversVideoContainersWithoutHostMimeTypes(t *testing.T) {
	for _, tt := range []struct {
		path string
		want string
	}{
		// 视频容器：容器内 Go 内置表解析为空，必须由内置映射兜底。
		{"movie.mkv", "video/x-matroska"},
		{"movie.mov", "video/quicktime"},
		{"movie.avi", "video/x-msvideo"},
		{"movie.ts", "video/mp2t"},
		{"movie.m2ts", "video/mp2t"},
		{"movie.iso", "application/x-iso9660-image"},
		{"movie.wmv", "video/x-ms-wmv"},
		{"movie.rmvb", "application/vnd.rn-realmedia-vbr"},
		// Go 内置表已覆盖，仍需保持正确。
		{"movie.mp4", "video/mp4"},
		{"movie.webm", "video/webm"},
		// 音频。
		{"song.flac", "audio/flac"},
		{"song.mp3", "audio/mpeg"},
		// 字幕：容器内 .srt/.ass 解析为空，直接影响字幕加载。
		{"sub.srt", "application/x-subrip"},
		{"sub.ass", "text/x-ssa; charset=utf-8"},
		{"sub.vtt", "text/vtt; charset=utf-8"},
		// 大小写不敏感。
		{"MOVIE.MKV", "video/x-matroska"},
		// 带查询串时仍能取到扩展名。
		{"movie.mkv?api_key=abc", "video/x-matroska"},
	} {
		if got := MediaMimeType(tt.path); got != tt.want {
			t.Errorf("MediaMimeType(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestMediaMimeTypeUnknownFallsBackToOctetStream 固定未知扩展名的兜底行为。
func TestMediaMimeTypeUnknownFallsBackToOctetStream(t *testing.T) {
	for _, path := range []string{"movie.unknownext", "noextension", ""} {
		if got := MediaMimeType(path); got != fallbackMimeType {
			t.Errorf("MediaMimeType(%q) = %q, want %q", path, got, fallbackMimeType)
		}
	}
}

// TestMediaMimeTypeDoesNotReturnEmpty 保证永不返回空串。
// 空值配合 nosniff 会退化为 octet-stream，进而让严格客户端拒播。
func TestMediaMimeTypeDoesNotReturnEmpty(t *testing.T) {
	for _, path := range []string{"a.mkv", "a.mov", "a.ts", "a.iso", "a", "a.", ".hidden"} {
		if got := MediaMimeType(path); got == "" {
			t.Errorf("MediaMimeType(%q) returned empty string", path)
		}
	}
}

// TestFlexIntAcceptsStringsAndNumbers 固定「宽容 JSON 标量」契约。
//
// 依据：Emby 协议事实标准（Jellyfin）全局开启
// NumberHandling = AllowReadingFromString，真实客户端会把数字写成字符串。
func TestFlexIntAcceptsStringsAndNumbers(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want int
	}{
		{`1`, 1},
		{`"1"`, 1},
		{`" 2 "`, 2},
		{`0`, 0},
		{`"0"`, 0},
		{`null`, 0},
		{`""`, 0},
		{`"abc"`, 0},
		{`true`, 1},
		{`false`, 0},
		{`"true"`, 1},
		{`1.9`, 1},
		{`"1.9"`, 1},
	} {
		var got model.FlexInt
		if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
			t.Errorf("FlexInt(%s) returned error: %v", tt.raw, err)
			continue
		}
		if got.Int() != tt.want {
			t.Errorf("FlexInt(%s) = %d, want %d", tt.raw, got.Int(), tt.want)
		}
	}
}

// TestFlexInt64AndFlexBoolNeverError 固定「解析失败取零值且不报错」契约。
func TestFlexInt64AndFlexBoolNeverError(t *testing.T) {
	for _, raw := range []string{`10000000`, `"10000000"`, `null`, `""`, `"nonsense"`, `{}`, `[]`} {
		var v model.FlexInt64
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Errorf("FlexInt64(%s) returned error: %v", raw, err)
		}
		var b model.FlexBool
		if err := json.Unmarshal([]byte(raw), &b); err != nil {
			t.Errorf("FlexBool(%s) returned error: %v", raw, err)
		}
	}
}

// TestEmbyPlaybackInfoRequestTolerateClientShapes 固定真实客户端 body 形态。
//
// 用例取自生产日志：CapyPlayer 与 Hills 把
// DeviceProfile.TranscodingProfiles[0].MinSegments 写成字符串，历史上因此
// 收到 400 而完全无法播放。
func TestEmbyPlaybackInfoRequestTolerateClientShapes(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "CapyPlayer MinSegments 字符串",
			body: `{"UserId":"u1","MaxStreamingBitrate":42000000,"DeviceProfile":{"Name":"CapyPlayer","TranscodingProfiles":[{"Container":"ts","Type":"Video","MinSegments":"1","BreakOnNonKeyFrames":"true"}],"SubtitleProfiles":[{"Format":"srt","Method":"External"}]}}`,
		},
		{
			name: "Hills MinSegments 字符串",
			body: `{"UserId":"u2","DeviceProfile":{"Name":"Hills","TranscodingProfiles":[{"Container":"ts","MinSegments":"1"}],"CodecProfiles":[]}}`,
		},
		{
			name: "DeviceProfile 为数组",
			body: `{"UserId":"u3","DeviceProfile":[{"Name":"ArrayShape"}]}`,
		},
		{
			name: "数值字段全部写成字符串",
			body: `{"UserId":"u4","MaxStreamingBitrate":"42000000","StartTimeTicks":"0","AudioStreamIndex":"0","SubtitleStreamIndex":"-1","MaxAudioChannels":"2"}`,
		},
		{
			name: "空对象",
			body: `{}`,
		},
		{
			name: "DeviceProfile 为 null",
			body: `{"UserId":"u5","DeviceProfile":null}`,
		},
		{
			name: "未知字段",
			body: `{"UserId":"u6","UnknownField":"x","AnotherOne":[1,2,3]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var req model.EmbyPlaybackInfoRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal returned error: %v", err)
			}
			// 关键断言：解析不得因类型不匹配失败。
			// DeviceProfile 若存在则必须是可用的结构体指针。
			if req.DeviceProfile == nil {
				return
			}
			for _, profile := range req.DeviceProfile.TranscodingProfiles {
				if profile.MinSegments.Int() < 0 {
					t.Fatalf("MinSegments should be non-negative, got %d", profile.MinSegments.Int())
				}
			}
		})
	}
}

// TestEmbyDeviceProfileArrayShapeTakesFirstElement 固定官方契约的数组形态。
//
// 官方把 PlaybackInfoRequest.DeviceProfile 标注为 DeviceProfile[]，
// 大多数实现发单个对象。两种都必须接受。
func TestEmbyDeviceProfileArrayShapeTakesFirstElement(t *testing.T) {
	var req model.EmbyPlaybackInfoRequest
	body := `{"DeviceProfile":[{"Name":"First","MaxStreamingBitrate":100},{"Name":"Second"}]}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.DeviceProfile == nil {
		t.Fatal("DeviceProfile is nil, want first array element")
	}
	if req.DeviceProfile.Name != "First" {
		t.Fatalf("DeviceProfile.Name = %q, want %q", req.DeviceProfile.Name, "First")
	}
}
