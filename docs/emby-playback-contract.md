# Emby 播放兼容契约

本文件记录 MediaStationGo 在 Emby 协议兼容上的**有意设计与对上游的偏离**。
修改 `internal/handler/emby_*` 或 `internal/service/emby_*` 中与播放、图片相关的
解析逻辑前，请先读完本文。

## 核心原则

**请求体是能力协商的输入，不是访问准入的门槛。**

Emby 的 `PlaybackInfo` 语义是「客户端描述自己的能力 → 服务端决定
DirectPlay / DirectStream / Transcode」。真正决定能否播放的是：

1. 令牌是否有效
2. 媒体对该用户是否可见
3. 媒体本身是否有可用的播放路径

**客户端叫什么名字、请求体里的可选字段写成什么类型，都不应导致播放失败。**

## 事实标准依据

Emby 协议兼容的事实标准是 Jellyfin（Emby 的开源分支）。它在 API 层
**全局**开启了宽容反序列化：

`src/Jellyfin.Extensions/Json/JsonDefaults.cs`

```csharp
private static readonly JsonSerializerOptions _jsonSerializerOptions = new()
{
    DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    NumberHandling = JsonNumberHandling.AllowReadingFromString,   // 字符串可当数字读
    Converters =
    {
        new JsonNullableStructConverterFactory(),
        new JsonStringConverter(),   // 任意 token 都按字符串读
        ...
    },
```

`JsonDefaultConverter` 全局套用到 ASP.NET Core 的 JSON 选项
（`Jellyfin.Server/Extensions/ApiServiceCollectionExtensions.cs` 的
`AddJsonOptions`）。这就是各种「不太规范」的第三方客户端能连上 Jellyfin 的原因。

Go 侧的等价物是本仓库的 `internal/model/emby_flexjson.go`：
`FlexInt` / `FlexInt64` / `FlexBool` 的 `UnmarshalJSON` **永不返回错误**，
解析失败取零值。

### 与上游的偏离

上游 MediaStationGo 走的是相反路线：严格解码，类型不匹配返回 400，并有测试
固化该行为。实测后果是 **CapyPlayer（22 次）与 Hills（14 次）完全无法播放**，
因为二者把 `DeviceProfile.TranscodingProfiles[0].MinSegments` 写成字符串 `"1"`。

本仓库选择与 Jellyfin 对齐。偏离点集中且可枚举，见下文「变更清单」。

## 容忍度契约

以下行为由测试固定，改动会被 CI 拦下。

### PlaybackInfo 请求体

| 输入 | 期望 | 依据 |
|---|---|---|
| `MinSegments: "1"` | 200，正常返回 MediaSources | CapyPlayer / Hills 真实 body |
| `MaxAudioChannels: "2"` | 200 | 真实客户端常见 |
| `StartTimeTicks: "0"` | 200 | 真实客户端常见 |
| `IsPlayback: "true"` | 200 | 布尔写成字符串 |
| `DeviceProfile: {...}`（对象） | 200 | 绝大多数客户端 |
| `DeviceProfile: [{...}]`（数组） | 200，取首个元素 | 官方契约标注为 `DeviceProfile[]` |
| `DeviceProfile: null` | 200，视作空 | — |
| 未知字段 | 200，忽略 | 保持向前兼容 |
| 非 JSON body | 200，空 profile 降级 + 记日志 | body 不是准入门槛 |
| 空 body | 200 | — |

测试：`internal/handler/emby_playback_info_routes_test.go` 的
`TestEmbyPlaybackInfoTypeMismatchDoesNotBlockPlayback`、
`TestEmbyPlaybackInfoNonJSONBodyDegradesGracefully`。

### 播放进度上报

| 输入 | 期望 |
|---|---|
| `PositionTicks: 10000000` | 204 且写入 `playback_histories` |
| `PositionTicks: "10000000"` | 204 **且写入**（关键：字符串 ticks 必须被解析） |
| `PlaySessionId: 123`（数字） | 204 |
| 空 body | 200 |

测试：`internal/service` 与 handler 层；线上验证见本文「验证记录」。

### 图片变体参数

渲染偏好类参数一律**归一 + 夹取**，不返回 400。

| 输入 | 期望 |
|---|---|
| `maxWidth=400&MaxWidth=400` | 200（大小写重复取首值） |
| `maxWidth=400&maxwidth=400` | 200（同名重复取首值） |
| `width=0` / `quality=0` | 200（0 视为未指定） |
| `width=99999` | 200（夹取到 4096） |
| `quality=999` | 200（夹取到 100） |
| `format=bmp` | 200（不支持则沿用默认格式） |
| `fillWidth=100`（只给一个） | 200（回退为不裁切） |

测试：`internal/service/image_proxy_variant_a10_test.go`。

### 媒体 MIME

**背景**：Alpine 基础镜像不含 `/etc/mime.types`（也未安装 `mailcap`），
Go 内置 MIME 表对 `.mkv/.mov/.avi/.ts/.m2ts/.iso/.srt/.ass` 返回空值。
配合 `X-Content-Type-Options: nosniff`，客户端收到 `application/octet-stream`，
部分播放器据此拒播。

因此 `internal/service/media_mime.go` 维护一份**内置表**，不依赖宿主环境。

| 扩展名 | MIME |
|---|---|
| `.mp4` | `video/mp4` |
| `.mkv` | `video/x-matroska` |
| `.mov` | `video/quicktime` |
| `.ts` / `.m2ts` / `.mts` | `video/mp2t` |
| `.iso` | `application/x-iso9660-image` |
| `.srt` | `application/x-subrip` |
| `.ass` / `.ssa` | `text/x-ssa; charset=utf-8` |
| 未知扩展名 | `application/octet-stream`（唯一允许的场景） |

测试：`internal/service/media_mime_test.go`。

### MediaSource 响应字段

官方契约（`dev.emby.media` RestAPI）要求、且客户端会读取的字段均已下发：

`MimeType`、`TranscodingMimeType`、`TranscodingUrl`、
`TranscodingSubProtocol`、`TranscodingContainer`、`AddApiKeyToDirectStreamUrl`、
`HasMixedProtocols`、`MediaAttachments`、`Chapters`；
`PlaybackInfoResponse` 顶层含 `ErrorCode`。

注意：**`TranscodingUrl` / `TranscodingSubProtocol` / `TranscodingContainer`
在「客户端直连解码」模式下有意不下发**——该模式的语义就是宿主机不参与转码，
下发 HLS 地址会诱导客户端请求转码流（见 `service/emby_playback.go` 注释）。
这是功能设计，不是字段缺失。

## 有意保持未实现的部分

### `/Users/Public` 返回 403

该接口是 Emby 官方登录页用来**枚举可见用户**的公开接口，只需 Id + Name 即可
列出全部账号。本服务将其归入「不对外暴露用户列表」的安全基线，由
`routes_authenticated_test.go` 的 forbidden 列表与
`emby_discovery_routes_test.go` 的断言共同守护。

**实测真实客户端（CapyPlayer、Hills）在收到 403/401 后仍能正常完成
`AuthenticateByName` 并进入 `PlaybackInfo`**，因此它不是播放链路的一环。

若未来确有客户端因缺该接口无法添加服务器，应按「需鉴权 + 返回脱敏列表」
的方式放开，而不是改成公开接口。

### 客户端专属分支

`internal/handler/emby_filmly_resume_compat.go` 是仓库中唯一按客户端名分流的
代码。它给 Filmly 的 Resume 响应补 `UserData.Key` / `UserData.ItemId`。

**注意**：这两个字段在 Emby 官方契约中是**标准字段**，理论上应对所有客户端下发。
当前保持仅 Filmly 生效，是为了避免改变其他客户端已验证的行为。若后续要统一，
需按客户端矩阵回归（见下）。

## 回归验证要求

改动播放相关代码后，除 `go test ./...` 外，建议用真实客户端 UA 与 body 回放：

| 客户端 | 关键特征 | 重点验证 |
|---|---|---|
| CapyPlayer | `DeviceProfile.TranscodingProfiles[0].MinSegments` 为字符串 | PlaybackInfo 200 |
| Hills | `SubtitleProfiles` 32 项、`MinSegments` 字符串 | PlaybackInfo 200 |
| Infuse | 缓存 URL 时会丢 token | 图片/字幕端点可访问 |
| VidHub / Yamby / SenPlayer | 分页滚动 | 首屏条数与 TotalRecordCount |
| Emby AndroidTV / Kodi | 字幕能力弱 | 字幕不挂载属已知限制 |

生产日志（`data/logs/app.log`）是首选证据源：
`grep -c 'emby_playback_info_parse_failed'` 应为 0（除结构性错误外）。

## 变更清单（相对上游 `dc6e5a7`）

| 文件 | 变更 |
|---|---|
| `internal/model/emby_flexjson.go` | **新增**。FlexInt / FlexInt64 / FlexBool / DeviceProfile 双形态 |
| `internal/model/emby_playback_types.go` | 数值字段改用 Flex 类型；补齐官方契约字段 |
| `internal/handler/emby_playback.go` | 宽容解析；解析错误改为诊断信息而非 400 |
| `internal/handler/emby_playstate_handlers.go` | 同上，进度上报不再拒绝 |
| `internal/service/media_mime.go` | **新增**。内置 MIME 表，不依赖宿主 |
| `internal/service/stream_file.go` | 显式设置正确 Content-Type |
| `internal/service/image_proxy_variant.go` | 参数归一 + 夹取，不再 400 |
| `internal/service/emby_playback.go` | 补 MediaSource 字段、ErrorCode、MediaAttachments |

同时**更新了两条固化旧行为的测试断言**：
`emby_playback_info_routes_test.go`（原断言「类型不匹配必须 400」）与
`image_proxy_variant_a10_test.go`（原断言「无效参数必须 400」）。

## 已知的既有问题（非本次变更引入）

以下测试存在**并发竞态抖动**，与本文改动无关，在未改动的上游树上同样可复现：

- `TestAdultProviderCoalescesConcurrentJavDBClearanceSolves`
  （FlareSolverr 单飞合并竞态，上游树 4/60 失败）
- `TestImageVariantA10ConcurrentCancellation`
  （8 goroutine 竞态，上游树 1/30 失败）

修复它们需要改动 adult_scraper 的合并逻辑，属独立议题。
