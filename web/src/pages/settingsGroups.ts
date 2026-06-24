import type { SettingDef } from './SettingsRow'

export interface SettingGroup {
  key: string
  label: string
  description?: string
  items: SettingDef[]
}

export const GROUPS: SettingGroup[] = [
  {
    key: 'general',
    label: '常规',
    description: '语言 / 转码引擎参数（API 密钥请在管理后台 → 外部API 配置）',
    items: [
      {
        key: 'tmdb.language',
        label: 'TMDb 元数据语言',
        type: 'select',
        options: [
          { value: 'zh-CN', label: '简体中文' },
          { value: 'zh-TW', label: '繁体中文' },
          { value: 'en-US', label: 'English' },
          { value: 'ja-JP', label: '日本語' },
        ],
      },
      {
        key: 'app.server_url',
        label: '公开访问域名 / STRM 域名',
        type: 'text',
        hint: '例如 http://NAS-IP:18080 或 https://media.example.com。填写后网盘媒体扫描会自动生成完整 STRM/302 播放入口；不填则使用同源相对路径。',
        placeholder: 'http://192.168.1.125:18080',
      },
      {
        key: 'playback.direct_only',
        label: '客户端直连解码（释放宿主机资源）',
        type: 'toggle',
        hint: '默认关闭。开启后宿主机不再进行任何 FFmpeg 转码，所有播放交给第三方客户端（Infuse / VLC / Emby 客户端等）或浏览器本地解码直连（direct play / 302 直链），大幅降低宿主机 CPU 占用。若客户端不支持源编码可能无法播放。',
        defaultValue: 'false',
      },
      {
        key: 'transcode.enabled',
        label: '启用转码',
        type: 'toggle',
        hint: '关闭后所有视频直连播放（「客户端直连解码」开启时本项自动失效）',
        defaultValue: 'true',
      },
      {
        key: 'transcode.hw_accel',
        label: '硬件编码器',
        type: 'select',
        hint: '只有开启下方「启用硬件加速」后才会使用；未开启时强制软件转码',
        defaultValue: 'none',
        options: [
          { value: 'none', label: '软件转码' },
          { value: 'nvenc', label: 'NVIDIA NVENC' },
          { value: 'qsv', label: 'Intel QSV' },
          { value: 'vaapi', label: 'VAAPI (Linux)' },
        ],
      },
      {
        key: 'transcode.hw_enabled',
        label: '启用硬件加速',
        type: 'toggle',
        hint: '关闭时即使选择了 NVENC/QSV/VAAPI，也不会调用硬件编码参数',
        defaultValue: 'false',
      },
      {
        key: 'transcode.max_jobs',
        label: '最大并发转码任务',
        type: 'number',
        hint: 'NAS 建议 1',
        defaultValue: '1',
      },
      {
        key: 'transcode.realtime',
        label: '按播放速度转码',
        type: 'toggle',
        hint: '开启后 ffmpeg 不会抢跑压完整片，可显著降低 CPU 峰值',
        defaultValue: 'true',
      },
      {
        key: 'transcode.threads',
        label: '软件转码线程数',
        type: 'number',
        hint: 'NAS 建议 1-2；仅软件转码生效',
        defaultValue: '2',
      },
      {
        key: 'transcode.idle_timeout_seconds',
        label: '转码空闲停止秒数',
        type: 'number',
        hint: '播放器关闭或停止请求分片后自动结束 ffmpeg',
        defaultValue: '120',
      },
      {
        key: 'ffmpeg.path',
        label: 'FFmpeg 路径',
        type: 'text',
        placeholder: 'ffmpeg',
      },
      {
        key: 'ffprobe.path',
        label: 'FFprobe 路径',
        type: 'text',
        placeholder: 'ffprobe',
      },
      {
        key: 'ffprobe.max_concurrent',
        label: 'FFprobe 最大并发',
        type: 'number',
        hint: 'NAS 建议 1；用于扫描、整理洗版和手动探测，避免同时启动多个 ffprobe 进程',
        defaultValue: '1',
      },
    ],
  },
  {
    key: 'license',
    label: '授权服务',
    description: '连接私有 MediaStationGo 授权服务；开源版默认最多 20 个用户，激活后按授权策略提升额度。',
    items: [
      {
        key: 'license.server_url',
        label: 'License Server 地址',
        type: 'text',
        placeholder: 'http://127.0.0.1:8001',
      },
      {
        key: 'license.hmac_secret',
        label: 'HMAC 签名密钥',
        type: 'text',
        hint: '必须与 License Server 的 LICENSE_HMAC_SECRET 保持一致；留空则跳过响应签名校验。',
      },
    ],
  },
  {
    key: 'cloud-upload',
    label: '网盘转存',
    description: '把本地媒体复制上传到外部存储。推荐：将 115、123、阿里等网盘挂载到 OpenList、CloudDrive2 或 Alist 后，使用桥接目标转存。',
    items: [
      {
        key: 'cloud.auto_sync_enabled',
        label: '夜间自动同步网盘媒体库',
        type: 'toggle',
        hint: '默认关闭。开启后仅在每天 23:00-05:00 按检查间隔触发；每次完整扫描所有启用网盘库一次后自动停止。手动扫描仍可随时执行。',
        defaultValue: 'false',
      },
      {
        key: 'cloud.sync_interval_seconds',
        label: '夜间窗口检查间隔秒数',
        type: 'number',
        hint: '最小 300 秒，建议 1800 秒；同一个夜间窗口成功同步后不会重复全量扫，避免大型网盘反复递归。',
        defaultValue: '1800',
      },
      {
        key: 'cloud.boot_scan_enabled',
        label: '启动后立即扫描网盘',
        type: 'toggle',
        hint: '默认关闭。仅排障或小型网盘建议开启；大型库请使用手动扫描或夜间自动同步。',
        defaultValue: 'false',
      },
      {
        key: 'cloud.upload_auto_enabled',
        label: '启用自动转存',
        type: 'toggle',
        hint: '开启后后台会按间隔扫描本地源目录，把视频、NFO、海报、字幕转存到目标存储；还需要在外部存储页开启该目标的“允许转存写入”。',
        defaultValue: 'false',
      },
      {
        key: 'cloud.upload_provider',
        label: '转存目标',
        type: 'select',
        defaultValue: 'alist',
        options: [
          { value: 'openlist', label: 'OpenList（推荐，可桥接 115/123/阿里等）' },
          { value: 'clouddrive2', label: 'CloudDrive2（推荐，可桥接 115/123/阿里等）' },
          { value: 'alist', label: 'Alist（可桥接多网盘）' },
          { value: 'webdav', label: 'WebDAV' },
          { value: 'cloud115', label: '115 原生（待接分片上传）' },
        ],
      },
      {
        key: 'cloud.upload_source_dir',
        label: '本地源目录',
        type: 'text',
        placeholder: '/media/电影 或 F:\\media\\Movies',
      },
      {
        key: 'cloud.upload_dest_path',
        label: '网盘目标目录',
        type: 'text',
        defaultValue: '/MediaStationGo',
        placeholder: '/MediaStationGo',
      },
      {
        key: 'cloud.upload_recursive',
        label: '递归扫描源目录',
        type: 'toggle',
        defaultValue: 'true',
      },
      {
        key: 'cloud.upload_sidecars',
        label: '同步 NFO / 海报 / 字幕',
        type: 'toggle',
        defaultValue: 'true',
      },
      {
        key: 'cloud.upload_overwrite',
        label: '覆盖远端同名文件',
        type: 'toggle',
        defaultValue: 'false',
      },
      {
        key: 'cloud.upload_transfer_mode',
        label: '自动转存方式',
        type: 'select',
        defaultValue: 'copy',
        hint: '复制会保留本地源文件；移动只在上传成功后删除本地文件。',
        options: [
          { value: 'copy', label: '复制' },
          { value: 'move', label: '移动' },
        ],
      },
      {
        key: 'cloud.upload_interval_seconds',
        label: '自动转存间隔秒数',
        type: 'number',
        hint: '最小 300 秒，建议 3600 秒或更高，避免频繁读盘和触发网盘风控。',
        defaultValue: '3600',
      },
    ],
  },
  {
    key: 'adult',
    label: 'Adult / NSFW',
    description: '成人内容隔离开关 (默认隐藏)',
    items: [
      {
        key: 'adult.enabled',
        label: '启用成人内容',
        type: 'toggle',
        hint: '全局开关。关闭后所有人都无法显示成人库；开启后用户仍默认隐藏，可在个人资料或 Bot 中自行显示。',
        defaultValue: 'true',
      },
      {
        key: 'adult.library_ids',
        label: '指定成人媒体库',
        type: 'library-multiselect',
        hint: '管理员指定哪些媒体库目录属于成人影视库。指定后网页、搜索和第三方客户端都会统一隐藏。',
        defaultValue: '[]',
      },
      {
        key: 'adult.require_pin',
        label: '访问需要 PIN',
        type: 'toggle',
      },
      {
        key: 'adult.pin',
        label: 'PIN 码',
        type: 'text',
        hint: '4-8 位数字',
      },
    ],
  },
]

export const ALL_KEYS = new Set(GROUPS.flatMap((group) => group.items.map((item) => item.key)))
