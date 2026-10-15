package model

// EmbyPlaybackInfoRequest 播放信息请求。
//
// 数字/布尔字段一律使用 FlexInt / FlexInt64 / FlexBool，以对齐 Emby 协议
// 事实标准（Jellyfin 全局开启 AllowReadingFromString）的宽容解析语义。
// 详见 emby_flexjson.go 的说明。
type EmbyPlaybackInfoRequest struct {
	UserId              string             `json:"UserId,omitempty"`
	MediaSourceId       string             `json:"MediaSourceId,omitempty"`
	IsPlayback          FlexBool           `json:"IsPlayback,omitempty"`
	MaxStreamingBitrate FlexInt64          `json:"MaxStreamingBitrate,omitempty"`
	StartTimeTicks      FlexInt64          `json:"StartTimeTicks,omitempty"`
	AudioStreamIndex    FlexInt            `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex FlexInt            `json:"SubtitleStreamIndex,omitempty"`
	MaxAudioChannels    FlexInt            `json:"MaxAudioChannels,omitempty"`
	ItemId              string             `json:"ItemId,omitempty"`
	LiveStreamId        string             `json:"LiveStreamId,omitempty"`
	CurrentPlaySessionID string            `json:"CurrentPlaySessionId,omitempty"`
	DeviceProfile       *EmbyDeviceProfile `json:"DeviceProfile,omitempty"`
	EnableDirectStream  FlexBool           `json:"EnableDirectStream,omitempty"`
	EnableDirectPlay    FlexBool           `json:"EnableDirectPlay,omitempty"`
	EnableTranscoding   FlexBool           `json:"EnableTranscoding,omitempty"`
	AutoOpenLiveStream  FlexBool           `json:"AutoOpenLiveStream,omitempty"`
	// 以下字段出现在官方契约中，这里保留接收以便客户端回传时不被丢弃。
	AllowVideoStreamCopy          FlexBool `json:"AllowVideoStreamCopy,omitempty"`
	AllowAudioStreamCopy          FlexBool `json:"AllowAudioStreamCopy,omitempty"`
	AllowInterlacedVideoStreamCopy FlexBool `json:"AllowInterlacedVideoStreamCopy,omitempty"`
}

// EmbyPlaybackInfoResponse 播放信息响应。
type EmbyPlaybackInfoResponse struct {
	MediaSources  []EmbyMediaSource `json:"MediaSources"`
	PlaySessionId string            `json:"PlaySessionId"`
}

// EmbyDeviceProfile 设备配置文件。
type EmbyDeviceProfile struct {
	Name                             string                   `json:"Name,omitempty"`
	Id                               string                   `json:"Id,omitempty"`
	MaxStaticBitrate                 FlexInt                  `json:"MaxStaticBitrate,omitempty"`
	MaxStreamingBitrate              FlexInt64                `json:"MaxStreamingBitrate,omitempty"`
	MusicStreamingTranscodingBitrate FlexInt                  `json:"MusicStreamingTranscodingBitrate,omitempty"`
	MaxStaticMusicBitrate            FlexInt                  `json:"MaxStaticMusicBitrate,omitempty"`
	DirectPlayProfiles               []EmbyDirectPlayProfile  `json:"DirectPlayProfiles,omitempty"`
	TranscodingProfiles              []EmbyTranscodingProfile `json:"TranscodingProfiles,omitempty"`
	ContainerProfiles                []EmbyContainerProfile   `json:"ContainerProfiles,omitempty"`
	CodecProfiles                    []EmbyCodecProfile       `json:"CodecProfiles,omitempty"`
	ResponseProfiles                 []EmbyResponseProfile    `json:"ResponseProfiles,omitempty"`
	SubtitleProfiles                 []EmbySubtitleProfile    `json:"SubtitleProfiles,omitempty"`
}

// EmbyDirectPlayProfile 直接播放配置。
type EmbyDirectPlayProfile struct {
	Container  string `json:"Container,omitempty"`
	AudioCodec string `json:"AudioCodec,omitempty"`
	VideoCodec string `json:"VideoCodec,omitempty"`
	Type       string `json:"Type,omitempty"`
}

// EmbyTranscodingProfile 转码配置。
//
// 数值字段使用 FlexInt：真实客户端（CapyPlayer、Hills 等）会把 MinSegments
// 写成字符串 "1"，严格解析会报 UnmarshalTypeError 并使整个播放请求失败。
type EmbyTranscodingProfile struct {
	Container                 string   `json:"Container,omitempty"`
	Type                      string   `json:"Type,omitempty"`
	VideoCodec                string   `json:"VideoCodec,omitempty"`
	AudioCodec                string   `json:"AudioCodec,omitempty"`
	Protocol                  string   `json:"Protocol,omitempty"`
	EstimateContentLength     FlexBool `json:"EstimateContentLength,omitempty"`
	EnableMpegtsM2TsMode      FlexBool `json:"EnableMpegtsM2TsMode,omitempty"`
	TranscodeSeekInfo         string   `json:"TranscodeSeekInfo,omitempty"`
	Context                   string   `json:"Context,omitempty"`
	EnableSubtitlesInManifest FlexBool `json:"EnableSubtitlesInManifest,omitempty"`
	MaxAudioChannels          string   `json:"MaxAudioChannels,omitempty"`
	MinSegments               FlexInt  `json:"MinSegments,omitempty"`
	SegmentLength             FlexInt  `json:"SegmentLength,omitempty"`
	BreakOnNonKeyFrames       FlexBool `json:"BreakOnNonKeyFrames,omitempty"`
}

// EmbyResponseProfile 响应配置（官方契约字段）。
type EmbyResponseProfile struct {
	Container  string                 `json:"Container,omitempty"`
	AudioCodec string                 `json:"AudioCodec,omitempty"`
	VideoCodec string                 `json:"VideoCodec,omitempty"`
	Type       string                 `json:"Type,omitempty"`
	OrgPn      string                 `json:"OrgPn,omitempty"`
	MimeType   string                 `json:"MimeType,omitempty"`
	Conditions []EmbyProfileCondition `json:"Conditions,omitempty"`
}

// EmbyContainerProfile 容器配置。
type EmbyContainerProfile struct {
	Type       string                 `json:"Type,omitempty"`
	Conditions []EmbyProfileCondition `json:"Conditions,omitempty"`
	Container  string                 `json:"Container,omitempty"`
}

// EmbyCodecProfile 编解码器配置。
type EmbyCodecProfile struct {
	Type       string                 `json:"Type,omitempty"`
	Conditions []EmbyProfileCondition `json:"Conditions,omitempty"`
	Codec      string                 `json:"Codec,omitempty"`
	Container  string                 `json:"Container,omitempty"`
}

// EmbyProfileCondition 配置条件。
type EmbyProfileCondition struct {
	Condition  string `json:"Condition,omitempty"`
	Property   string `json:"Property,omitempty"`
	Value      string `json:"Value,omitempty"`
	IsRequired bool   `json:"IsRequired,omitempty"`
}

// EmbySubtitleProfile 字幕配置。
type EmbySubtitleProfile struct {
	Format    string `json:"Format,omitempty"`
	Method    string `json:"Method,omitempty"`
	DidlMode  string `json:"DidlMode,omitempty"`
	Language  string `json:"Language,omitempty"`
	Container string `json:"Container,omitempty"`
}

// EmbyPlaybackProgressRequest 播放进度上报。
type EmbyPlaybackProgressRequest struct {
	CanSeek             bool     `json:"CanSeek"`
	ItemId              string   `json:"ItemId"`
	MediaSourceId       string   `json:"MediaSourceId,omitempty"`
	PositionTicks       int64    `json:"PositionTicks"`
	RunTimeTicks        int64    `json:"RunTimeTicks,omitempty"`
	IsPaused            bool     `json:"IsPaused"`
	IsMuted             bool     `json:"IsMuted"`
	VolumeLevel         int      `json:"VolumeLevel,omitempty"`
	PlayMethod          string   `json:"PlayMethod,omitempty"`
	PlaySessionId       string   `json:"PlaySessionId,omitempty"`
	LiveStreamId        string   `json:"LiveStreamId,omitempty"`
	QueueableMediaTypes []string `json:"QueueableMediaTypes,omitempty"`
}

// EmbyStopPlaybackRequest 停止播放上报。
type EmbyStopPlaybackRequest struct {
	ItemId        string `json:"ItemId"`
	MediaSourceId string `json:"MediaSourceId,omitempty"`
	PositionTicks int64  `json:"PositionTicks"`
	RunTimeTicks  int64  `json:"RunTimeTicks,omitempty"`
	PlaySessionId string `json:"PlaySessionId,omitempty"`
	LiveStreamId  string `json:"LiveStreamId,omitempty"`
}

// EmbyUserDataRequest 用户数据更新。
type EmbyUserDataRequest struct {
	PlaybackPositionTicks int64   `json:"PlaybackPositionTicks,omitempty"`
	PlayCount             int     `json:"PlayCount,omitempty"`
	IsFavorite            bool    `json:"IsFavorite,omitempty"`
	Played                bool    `json:"Played,omitempty"`
	PlayedPercentage      float64 `json:"PlayedPercentage,omitempty"`
}

// EmbyActiveEncodingRequest 活跃编码请求（客户端报告转码进度）。
type EmbyActiveEncodingRequest struct {
	PlaySessionId string `json:"PlaySessionId"`
	When          string `json:"When"`
	PositionTicks int64  `json:"PositionTicks,omitempty"`
	IsPaused      bool   `json:"IsPaused,omitempty"`
	IsUserPaused  bool   `json:"IsUserPaused,omitempty"`
}
