// Package model — Emby API 兼容层请求/响应类型。
package model

import "time"

// ─── Emby 认证 ────────────────────────────────────────────────────────────────

// EmbyAuthRequest Emby 认证请求（用户名+密码）。
type EmbyAuthRequest struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// EmbyAuthResponse Emby 认证响应。
type EmbyAuthResponse struct {
	User    EmbyUser   `json:"User"`
	AccessToken string `json:"AccessToken"`
	ServerID    string `json:"ServerId"`
}

// EmbyApiKeyAuthRequest API Key 认证请求。
type EmbyApiKeyAuthRequest struct {
	ApiKey string `json:"ApiKey"`
}

// ─── Emby 用户 ────────────────────────────────────────────────────────────────

// EmbyUser Emby 用户信息。
type EmbyUser struct {
	Id                  string    `json:"Id"`
	Name                string    `json:"Name"`
	ServerId            string    `json:"ServerId"`
	HasPassword         bool      `json:"HasPassword"`
	PrimaryImageTag     string    `json:"PrimaryImageTag,omitempty"`
	Configuration       *EmbyUserConfiguration `json:"Configuration,omitempty"`
	LastActivityDate    *time.Time `json:"LastActivityDate,omitempty"`
	LastLoginDate       *time.Time `json:"LastLoginDate,omitempty"`
}

// EmbyUserConfiguration 用户配置。
type EmbyUserConfiguration struct {
	AudioLanguagePreference       string `json:"AudioLanguagePreference"`
	SubtitleLanguagePreference    string `json:"SubtitleLanguagePreference"`
	EnableAutoPlay                bool   `json:"EnableAutoPlay"`
	EnableNextEpisodeAutoPlay    bool   `json:"EnableNextEpisodeAutoPlay"`
}

// ─── Emby 系统信息 ────────────────────────────────────────────────────────────

// EmbySystemInfo 系统信息。
type EmbySystemInfo struct {
	Id                    string   `json:"Id"`
	ServerName            string   `json:"ServerName"`
	Version               string   `json:"Version"`
	ProductName           string   `json:"ProductName"`
	OperatingSystem       string   `json:"OperatingSystem"`
	Architecture          string   `json:"Architecture"`
	LocalAddress          string   `json:"LocalAddress"`
	WanAddress            string   `json:"WanAddress,omitempty"`
	HasPendingRestart     bool     `json:"HasPendingRestart"`
	IsShuttingDown        bool     `json:"IsShuttingDown"`
	SupportsLibraryScan   bool     `json:"SupportsLibraryScan"`
	SupportsHttps         bool     `json:"SupportsHttps"`
	SupportsAutoDiscovery bool     `json:"SupportsAutoDiscovery"`
	WebSocketPortNumber   int      `json:"WebSocketPortNumber"`
	TranscodingTempPath   string   `json:"TranscodingTempPath,omitempty"`
	CanSelfUpdate         bool     `json:"CanSelfUpdate"`
	CanLaunchWebBrowser   bool     `json:"CanLaunchWebBrowser"`
	CanRestart            bool     `json:"CanRestart"`
	CodecCount            int      `json:"CodecCount"`
}

// EmbyLogEntry 日志条目。
type EmbyLogEntry struct {
	Id           string    `json:"Id"`
	DateCreated  time.Time `json:"DateCreated"`
	Level        string    `json:"Level"`
	Message      string    `json:"Message"`
}

// EmbyServerConfiguration 服务器配置。
type EmbyServerConfiguration struct {
	Name               string `json:"Name"`
	ServerName         string `json:"ServerName"`
	EnableUPnP         bool   `json:"EnableUPnP"`
	PublicPort         int    `json:"PublicPort"`
	EnableHttps        bool   `json:"EnableHttps"`
	HttpServerPortNumber int  `json:"HttpServerPortNumber"`
	HttpsPortNumber    int    `json:"HttpsPortNumber"`
	EnableRemoteAccess bool   `json:"EnableRemoteAccess"`
}

// EmbySession 会话信息。
type EmbySession struct {
	Id                string              `json:"Id"`
	Client            string              `json:"Client"`
	ClientVersion     string              `json:"ClientVersion"`
	DeviceId          string              `json:"DeviceId"`
	DeviceName        string              `json:"DeviceName"`
	UserName          string              `json:"UserName,omitempty"`
	UserId            string              `json:"UserId,omitempty"`
	LastActivityDate  *time.Time          `json:"LastActivityDate,omitempty"`
	RemoteEndPoint    string              `json:"RemoteEndPoint,omitempty"`
	NowPlayingItem    *EmbyItem           `json:"NowPlayingItem,omitempty"`
	PlayState         *EmbyPlaybackState  `json:"PlayState,omitempty"`
	Capabilities      EmbyClientCapabilities `json:"Capabilities,omitempty"`
	SupportsRemoteControl bool            `json:"SupportsRemoteControl"`
	AdditionalUsers   []EmbySessionUserInfo `json:"AdditionalUsers,omitempty"`
}

// EmbySessionUserInfo 会话中的附加用户。
type EmbySessionUserInfo struct {
	UserId string `json:"UserId"`
	UserName string `json:"UserName"`
}

// EmbyPlaybackState 播放状态。
type EmbyPlaybackState struct {
	PositionTicks  int64  `json:"PositionTicks"`
	VolumeLevel    int    `json:"VolumeLevel"`
	IsMuted        bool   `json:"IsMuted"`
	IsPaused       bool   `json:"IsPaused"`
	PlayMethod     string `json:"PlayMethod,omitempty"`
	CanSeek        bool   `json:"CanSeek"`
}

// EmbyClientCapabilities 客户端能力描述。
type EmbyClientCapabilities struct {
	PlayableMediaTypes []string `json:"PlayableMediaTypes"`
	SupportedCommands  []string `json:"SupportedCommands"`
	SupportsMediaControl bool    `json:"SupportsMediaControl"`
	SupportsSync        bool    `json:"SupportsSync"`
}

// ─── Emby 虚拟文件夹 / 媒体库 ────────────────────────────────────────────────

// EmbyVirtualFolder 虚拟文件夹（媒体库）。
type EmbyVirtualFolder struct {
	Name               string            `json:"Name"`
	Locations          []string          `json:"Locations"`
		CollectionType    string            `json:"CollectionType"`
		LibraryOptions    EmbyLibraryOptions `json:"LibraryOptions,omitempty"`
		RefreshStatus     *EmbyRefreshStatus `json:"RefreshStatus,omitempty"`
		ItemId            string            `json:"ItemId"`
}

// EmbyLibraryOptions 媒体库选项。
type EmbyLibraryOptions struct {
	PreferredMetadataLanguage        string `json:"PreferredMetadataLanguage"`
	MetadataCountryCode              string `json:"MetadataCountryCode"`
	EnableRealtimeMonitor            bool   `json:"EnableRealtimeMonitor"`
	EnableAutomaticSeriesGrouping    bool   `json:"EnableAutomaticSeriesGrouping"`
}

// EmbyRefreshStatus 刷新状态。
type EmbyRefreshStatus struct {
	LastRefreshResult string    `json:"LastRefreshResult"`
	LastRefreshedAt   time.Time `json:"LastRefreshedAt"`
	IsActive          bool      `json:"IsActive"`
}

// EmbyItemsCounts 项目计数。
type EmbyItemsCounts struct {
	MovieCount    int `json:"MovieCount"`
	SeriesCount   int `json:"SeriesCount"`
	EpisodeCount  int `json:"EpisodeCount"`
	ArtistCount   int `json:"ArtistCount"`
	AlbumCount    int `json:"AlbumCount"`
	SongCount     int `json:"SongCount"`
	MusicVideoCount int `json:"MusicVideoCount"`
	BookCount     int `json:"BookCount"`
	BoxSetCount   int `json:"BoxSetCount"`
}

// ─── Emby Items ───────────────────────────────────────────────────────────────

// EmbyItemsResponse Emby 标准分页响应包装。
type EmbyItemsResponse struct {
	Items            []EmbyItem `json:"Items"`
	TotalRecordCount int        `json:"TotalRecordCount"`
	StartIndex       int        `json:"StartIndex"`
}

// EmbyItem Emby 媒体项。
type EmbyItem struct {
	Id                  string              `json:"Id"`
	Name                string              `json:"Name"`
	Type                string              `json:"Type"` // Movie / Series / Episode / Season / BoxSet / Folder
	Overview            string              `json:"Overview,omitempty"`
	ProductionYear      int                 `json:"ProductionYear,omitempty"`
	PremiereDate        *time.Time          `json:"PremiereDate,omitempty"`
	CommunityRating     float64             `json:"CommunityRating,omitempty"`
	OfficialRating      string              `json:"OfficialRating,omitempty"`
	RunTimeTicks        int64               `json:"RunTimeTicks,omitempty"`
	ParentId            string              `json:"ParentId,omitempty"`
	SeriesId            string              `json:"SeriesId,omitempty"`
	SeasonId            string              `json:"SeasonId,omitempty"`
	IndexNumber         int                 `json:"IndexNumber,omitempty"`
	ParentIndexNumber   int                 `json:"ParentIndexNumber,omitempty"`
	UserData            *EmbyUserData       `json:"UserData,omitempty"`
	ImageTags           map[string]string   `json:"ImageTags,omitempty"`
	BackdropImageTags   []string            `json:"BackdropImageTags,omitempty"`
	Genres              []string            `json:"Genres,omitempty"`
	Studios             []EmbyNameId        `json:"Studios,omitempty"`
	People              []EmbyPerson        `json:"People,omitempty"`
	MediaSources        []EmbyMediaSource   `json:"MediaSources,omitempty"`
	RecursiveItemCount  int                 `json:"RecursiveItemCount,omitempty"`
	ChildCount          int                 `json:"ChildCount,omitempty"`
	Status              string              `json:"Status,omitempty"`
	AirDays             []string            `json:"AirDays,omitempty"`
	EndDate             *time.Time          `json:"EndDate,omitempty"`
	ProviderIds         map[string]string   `json:"ProviderIds,omitempty"`
	Taglines            []string            `json:"Taglines,omitempty"`
	GenreItems          []EmbyNameId        `json:"GenreItems,omitempty"`
	DateCreated         *time.Time          `json:"DateCreated,omitempty"`
	Path                string              `json:"Path,omitempty"`
	SortName            string              `json:"SortName,omitempty"`
	ForcedSortName      string              `json:"ForcedSortName,omitempty"`
	Width               int                 `json:"Width,omitempty"`
	Height              int                 `json:"Height,omitempty"`
	Container           string              `json:"Container,omitempty"`
}

// EmbyUserData 用户播放数据。
type EmbyUserData struct {
	PlaybackPositionTicks int64   `json:"PlaybackPositionTicks"`
	PlayCount             int     `json:"PlayCount"`
	IsFavorite            bool    `json:"IsFavorite"`
	Played                bool    `json:"Played"`
	UnplayedItemCount     int     `json:"UnplayedItemCount"`
	PercentagePlayed      float64 `json:"PercentagePlayed"`
	Rating                float64 `json:"Rating,omitempty"`
	PlayedPercentage      float64 `json:"PlayedPercentage,omitempty"`
}

// EmbyMediaSource 媒体源。
type EmbyMediaSource struct {
	Id                    string             `json:"Id"`
	Name                  string             `json:"Name"`
	Path                  string             `json:"Path"`
	Size                  int64              `json:"Size"`
	Container             string             `json:"Container,omitempty"`
	Bitrate               int64              `json:"Bitrate,omitempty"`
	MediaStreams           []EmbyMediaStream  `json:"MediaStreams"`
	SupportsTranscoding   bool               `json:"SupportsTranscoding"`
	SupportsDirectStream  bool               `json:"SupportsDirectStream"`
	SupportsDirectPlay    bool               `json:"SupportsDirectPlay"`
	TranscodingUrl        string             `json:"TranscodingUrl,omitempty"`
	Protocol              string             `json:"Protocol,omitempty"`
	Type                  string             `json:"Type,omitempty"`
	IsRemote              bool               `json:"IsRemote,omitempty"`
	RunTimeTicks          int64              `json:"RunTimeTicks,omitempty"`
	ETag                  string             `json:"ETag,omitempty"`
	SupportsProbing       bool               `json:"SupportsProbing,omitempty"`
}

// EmbyMediaStream 媒体流（视频/音频/字幕）。
type EmbyMediaStream struct {
	Codec              string  `json:"Codec"`
	Type               string  `json:"Type"` // Video / Audio / Subtitle
	Language           string  `json:"Language,omitempty"`
	DisplayTitle       string  `json:"DisplayTitle,omitempty"`
	Index              int     `json:"Index"`
	IsDefault          bool    `json:"IsDefault"`
	IsForced           bool    `json:"IsForced"`
	IsExternal         bool    `json:"IsExternal,omitempty"`
	Height             int     `json:"Height,omitempty"`
	Width              int     `json:"Width,omitempty"`
	BitRate            int64   `json:"BitRate,omitempty"`
	Channels           int     `json:"Channels,omitempty"`
	SampleRate         int     `json:"SampleRate,omitempty"`
	AspectRatio        string  `json:"AspectRatio,omitempty"`
	VideoRange         string  `json:"VideoRange,omitempty"`
	DeliveryUrl        string  `json:"DeliveryUrl,omitempty"`
	DeliveryMethod     string  `json:"DeliveryMethod,omitempty"`
	ExternalUrl        string  `json:"ExternalUrl,omitempty"`
	ExternalSubtitleId string  `json:"ExternalSubtitleId,omitempty"`
	SubtitleFileName   string  `json:"SubtitleFileName,omitempty"`
	Title              string  `json:"Title,omitempty"`
	Comment            string  `json:"Comment,omitempty"`
	Path               string  `json:"Path,omitempty"`
}

// EmbyPerson 人员信息。
type EmbyPerson struct {
	Id      string `json:"Id"`
	Name    string `json:"Name"`
	Role    string `json:"Role,omitempty"`
	Type    string `json:"Type,omitempty"`
	PrimaryImageTag string `json:"PrimaryImageTag,omitempty"`
}

// EmbyNameId 名称+ID 对。
type EmbyNameId struct {
	Name string `json:"Name"`
	Id   string `json:"Id"`
}

// ─── Emby PlaybackInfo ────────────────────────────────────────────────────────

// EmbyPlaybackInfoRequest 播放信息请求。
type EmbyPlaybackInfoRequest struct {
	UserId            string                `json:"UserId,omitempty"`
	MaxStreamingBitrate int64                `json:"MaxStreamingBitrate,omitempty"`
	StartTimeTicks    int64                 `json:"StartTimeTicks,omitempty"`
	AudioStreamIndex  int                   `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex int                 `json:"SubtitleStreamIndex,omitempty"`
	MaxAudioChannels  int                   `json:"MaxAudioChannels,omitempty"`
	ItemId            string                `json:"ItemId,omitempty"`
	DeviceProfile     *EmbyDeviceProfile    `json:"DeviceProfile,omitempty"`
	EnableDirectStream bool                 `json:"EnableDirectStream,omitempty"`
	EnableDirectPlay   bool                 `json:"EnableDirectPlay,omitempty"`
	AutoOpenLiveStream bool                 `json:"AutoOpenLiveStream,omitempty"`
}

// EmbyPlaybackInfoResponse 播放信息响应。
type EmbyPlaybackInfoResponse struct {
	MediaSources []EmbyMediaSource `json:"MediaSources"`
	PlaySessionId string          `json:"PlaySessionId"`
}

// EmbyDeviceProfile 设备配置文件。
type EmbyDeviceProfile struct {
	Name        string                `json:"Name,omitempty"`
	MaxStaticBitrate int              `json:"MaxStaticBitrate,omitempty"`
	MaxStreamingBitrate int           `json:"MaxStreamingBitrate,omitempty"`
	MusicStreamingTranscodingBitrate int `json:"MusicStreamingTranscodingBitrate,omitempty"`
	DirectPlayProfiles []EmbyDirectPlayProfile `json:"DirectPlayProfiles,omitempty"`
	TranscodingProfiles []EmbyTranscodingProfile `json:"TranscodingProfiles,omitempty"`
	ContainerProfiles []EmbyContainerProfile `json:"ContainerProfiles,omitempty"`
	CodecProfiles []EmbyCodecProfile `json:"CodecProfiles,omitempty"`
	SubtitleProfiles []EmbySubtitleProfile `json:"SubtitleProfiles,omitempty"`
}

// EmbyDirectPlayProfile 直接播放配置。
type EmbyDirectPlayProfile struct {
	Container string `json:"Container,omitempty"`
	AudioCodec string `json:"AudioCodec,omitempty"`
	VideoCodec string `json:"VideoCodec,omitempty"`
	Type       string `json:"Type,omitempty"`
}

// EmbyTranscodingProfile 转码配置。
type EmbyTranscodingProfile struct {
	Container       string `json:"Container,omitempty"`
	Type            string `json:"Type,omitempty"`
	VideoCodec      string `json:"VideoCodec,omitempty"`
	AudioCodec      string `json:"AudioCodec,omitempty"`
	Protocol        string `json:"Protocol,omitempty"`
	EstimateContentLength bool `json:"EstimateContentLength,omitempty"`
	EnableMpegtsM2TsMode bool `json:"EnableMpegtsM2TsMode,omitempty"`
	TranscodeSeekInfo string `json:"TranscodeSeekInfo,omitempty"`
	Context           string `json:"Context,omitempty"`
	EnableSubtitlesInManifest bool `json:"EnableSubtitlesInManifest,omitempty"`
	MaxAudioChannels int `json:"MaxAudioChannels,omitempty"`
	MinSegments int `json:"MinSegments,omitempty"`
	SegmentLength int `json:"SegmentLength,omitempty"`
	BreakOnNonKeyFrames bool `json:"BreakOnNonKeyFrames,omitempty"`
}

// EmbyContainerProfile 容器配置。
type EmbyContainerProfile struct {
	Type       string   `json:"Type,omitempty"`
	Conditions []string `json:"Conditions,omitempty"`
	Container  string   `json:"Container,omitempty"`
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
	Condition     string `json:"Condition,omitempty"`
	Property      string `json:"Property,omitempty"`
	Value         string `json:"Value,omitempty"`
	IsRequired    bool   `json:"IsRequired,omitempty"`
}

// EmbySubtitleProfile 字幕配置。
type EmbySubtitleProfile struct {
	Format   string `json:"Format,omitempty"`
	Method   string `json:"Method,omitempty"`
	DidlMode string `json:"DidlMode,omitempty"`
	Language string `json:"Language,omitempty"`
	Container string `json:"Container,omitempty"`
}

// ─── Emby 播放进度上报 ────────────────────────────────────────────────────────

// EmbyPlaybackProgressRequest 播放进度上报。
type EmbyPlaybackProgressRequest struct {
	CanSeek            bool           `json:"CanSeek"`
	ItemId             string         `json:"ItemId"`
	MediaSourceId      string         `json:"MediaSourceId,omitempty"`
	PositionTicks      int64          `json:"PositionTicks"`
	RunTimeTicks       int64          `json:"RunTimeTicks,omitempty"`
	IsPaused           bool           `json:"IsPaused"`
	IsMuted            bool           `json:"IsMuted"`
	VolumeLevel        int            `json:"VolumeLevel,omitempty"`
	PlayMethod         string         `json:"PlayMethod,omitempty"`
	PlaySessionId      string         `json:"PlaySessionId,omitempty"`
	LiveStreamId       string         `json:"LiveStreamId,omitempty"`
	QueueableMediaTypes []string     `json:"QueueableMediaTypes,omitempty"`
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

// ─── Emby Hubs ────────────────────────────────────────────────────────────────

// EmbyHubResponse Hub 响应。
type EmbyHubResponse struct {
	Items []EmbyHubItem `json:"Items"`
}

// EmbyHubItem Hub 条目。
type EmbyHubItem struct {
	Id          string              `json:"Id"`
	Name        string              `json:"Name"`
	Type        string              `json:"Type"`
	Items       []EmbyItem          `json:"Items"`
	TotalCount  int                 `json:"TotalCount,omitempty"`
	ImageUrl    string              `json:"ImageUrl,omitempty"`
}

// ─── Emby 字幕 ────────────────────────────────────────────────────────────────

// EmbyRemoteSubtitleInfo 远程字幕信息。
type EmbyRemoteSubtitleInfo struct {
	ThreeLetterISOLanguageName string  `json:"ThreeLetterISOLanguageName"`
	Id                         string  `json:"Id"`
	ProviderName               string  `json:"ProviderName"`
	Name                       string  `json:"Name"`
	Format                     string  `json:"Format"`
	Author                     string  `json:"Author"`
	Comment                    string  `json:"Comment"`
	DateCreated                *time.Time `json:"DateCreated,omitempty"`
	CommunityRating            float64 `json:"CommunityRating,omitempty"`
	DownloadCount              int     `json:"DownloadCount"`
	IsHashMatch                bool    `json:"IsHashMatch,omitempty"`
	IsForced                   bool    `json:"IsForced,omitempty"`
	IsHearingImpaired          bool    `json:"IsHearingImpaired,omitempty"`
}

// EmbySubtitleSearchRequest 字幕搜索请求。
type EmbySubtitleSearchRequest struct {
	ItemId      string `json:"ItemId"`
	Language    string `json:"Language"`
	IsPerfectMatch bool `json:"IsPerfectMatch,omitempty"`
}

// EmbyImageRemoteInfo 远程图片信息。
type EmbyImageRemoteInfo struct {
	Providers []EmbyImageProviderInfo `json:"Providers"`
	TotalRecordCount int              `json:"TotalRecordCount"`
}

// EmbyImageProviderInfo 图片提供者信息。
type EmbyImageProviderInfo struct {
	Name             string               `json:"Name"`
	RemoteImages     []EmbyRemoteImageInfo `json:"RemoteImages,omitempty"`
	SupportedImages  []string             `json:"SupportedImages"`
}

// EmbyRemoteImageInfo 远程图片信息。
type EmbyRemoteImageInfo struct {
	Url          string    `json:"Url"`
	ThumbnailUrl string    `json:"ThumbnailUrl,omitempty"`
	Height       int       `json:"Height"`
	Width        int       `json:"Width"`
	CommunityRating float64 `json:"CommunityRating,omitempty"`
	VoteCount    int       `json:"VoteCount,omitempty"`
	Language     string    `json:"Language,omitempty"`
	Type         string    `json:"Type"`
	RatingType   string    `json:"RatingType,omitempty"`
	ProviderName string    `json:"ProviderName"`
}

// ─── Emby Genre / Person ─────────────────────────────────────────────────────

// EmbyGenre 类型。
type EmbyGenre struct {
	Name        string `json:"Name"`
	Id          string `json:"Id,omitempty"`
}

// EmbyPersonInfo 人物信息。
type EmbyPersonInfo struct {
	Id             string    `json:"Id"`
	Name           string    `json:"Name"`
	Type           string    `json:"Type,omitempty"`
	PrimaryImageTag string   `json:"PrimaryImageTag,omitempty"`
	Overview       string    `json:"Overview,omitempty"`
	BirthDate      string    `json:"BirthDate,omitempty"`
	ProductionYear int       `json:"ProductionYear,omitempty"`
	EndDate        string    `json:"EndDate,omitempty"`
	PremiereDate   *time.Time `json:"PremiereDate,omitempty"`
}

// ─── Emby Active Encoding ────────────────────────────────────────────────────

// EmbyActiveEncodingRequest 活跃编码请求（客户端报告转码进度）。
type EmbyActiveEncodingRequest struct {
	PlaySessionId string `json:"PlaySessionId"`
	When          string `json:"When"`
	PositionTicks int64  `json:"PositionTicks,omitempty"`
	IsPaused      bool   `json:"IsPaused,omitempty"`
	IsUserPaused  bool   `json:"IsUserPaused,omitempty"`
}
