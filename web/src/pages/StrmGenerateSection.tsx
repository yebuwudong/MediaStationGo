import type { FormEvent } from 'react'
import { Loader2, Save, Wand2 } from 'lucide-react'

import type { GenerateSTRMResult } from '../api/strm'
import type { Library } from '../types'
import { currentOrigin, type CloudPlaybackMode } from './strmPageModel'

type StrmGenerateSectionProps = {
  libraries: Library[]
  generateLibraryID: string
  baseURL: string
  outputDir: string
  cloudPlaybackMode: CloudPlaybackMode
  strmPlaybackEnabled: boolean
  redirectProxyEnabled: boolean
  autoGenerate: boolean
  savingSettings: boolean
  overwrite: boolean
  generating: boolean
  generateResult: GenerateSTRMResult | null
  playbackStatus: string
  onGenerate: (event: FormEvent) => void
  saveSTRMSettings: () => void
  setGenerateLibraryID: (value: string) => void
  setBaseURL: (value: string) => void
  setOutputDir: (value: string) => void
  setCloudPlaybackMode: (value: CloudPlaybackMode) => void
  setStrmPlaybackEnabled: (value: boolean) => void
  setRedirectProxyEnabled: (value: boolean) => void
  setAutoGenerate: (value: boolean) => void
  setOverwrite: (value: boolean) => void
}

export function StrmGenerateSection({
  libraries,
  generateLibraryID,
  baseURL,
  outputDir,
  cloudPlaybackMode,
  strmPlaybackEnabled,
  redirectProxyEnabled,
  autoGenerate,
  savingSettings,
  overwrite,
  generating,
  generateResult,
  playbackStatus,
  onGenerate,
  saveSTRMSettings,
  setGenerateLibraryID,
  setBaseURL,
  setOutputDir,
  setCloudPlaybackMode,
  setStrmPlaybackEnabled,
  setRedirectProxyEnabled,
  setAutoGenerate,
  setOverwrite,
}: StrmGenerateSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="font-display text-lg font-semibold text-ink-600">自动生成 STRM 文件</h2>
          <p className="text-sm text-ink-50">
            只需要填写自己的访问域名，系统会按媒体库内每个媒体批量生成可播放的 .strm 文件。
          </p>
        </div>
        <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${
          strmPlaybackEnabled || redirectProxyEnabled
            ? 'border-emerald-300/40 bg-emerald-400/10 text-emerald-500'
            : 'border-red-300/40 bg-red-400/10 text-red-500'
        }`}>
          {playbackStatus}
        </span>
      </div>
      <div className="grid gap-3 rounded-2xl border border-gray-200 bg-white/70 p-4 md:grid-cols-2">
        <label className="flex items-start gap-3 text-sm text-ink-100">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4 accent-primary-400"
            checked={strmPlaybackEnabled}
            onChange={(e) => setStrmPlaybackEnabled(e.target.checked)}
          />
          <span>
            <span className="block font-medium text-ink-600">启用 STRMURL 播放</span>
            <span className="text-xs text-ink-50">第三方客户端可拿到 /api/stream/媒体ID 入口，适合 STRM 管理和自动生成方案。</span>
          </span>
        </label>
        <label className="flex items-start gap-3 text-sm text-ink-100">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4 accent-primary-400"
            checked={redirectProxyEnabled}
            onChange={(e) => setRedirectProxyEnabled(e.target.checked)}
          />
          <span>
            <span className="block font-medium text-ink-600">启用 302/反代播放</span>
            <span className="text-xs text-ink-50">第三方客户端可拿到 /Videos/媒体ID/stream 入口，由服务端解析后 302 或必要时反代。</span>
          </span>
        </label>
      </div>
      <div className="grid gap-3 rounded-2xl border border-gray-200 bg-white/70 p-4 md:grid-cols-[1fr_1fr_auto]">
        <label className="text-sm text-ink-100">
          <span className="mb-1 block font-medium text-ink-600">两者都开启时优先</span>
          <select
            className="input-base"
            value={cloudPlaybackMode}
            onChange={(e) => setCloudPlaybackMode(e.target.value as CloudPlaybackMode)}
            disabled={!strmPlaybackEnabled || !redirectProxyEnabled}
          >
            <option value="strm">优先 STRMURL</option>
            <option value="redirect_proxy">优先 302/反代</option>
          </select>
          <span className="mt-1 block text-xs text-ink-50">只开启一个时自动使用已开启的播放方式；两个都关闭时云盘媒体不向第三方提供播放。</span>
        </label>
        <label className="flex items-start gap-3 text-sm text-ink-100">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4 accent-primary-400"
            checked={autoGenerate}
            onChange={(e) => setAutoGenerate(e.target.checked)}
          />
          <span>
            <span className="block font-medium text-ink-600">扫描后自动刷新 STRM 文件</span>
            <span className="text-xs text-ink-50">默认关闭，避免扫描大型网盘库时重复写文件。</span>
          </span>
        </label>
        <button type="button" className="neon-button self-center" disabled={savingSettings} onClick={saveSTRMSettings}>
          {savingSettings ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          保存开关
        </button>
      </div>
      <form onSubmit={onGenerate} className="grid gap-3 md:grid-cols-4">
        <select
          required
          className="input-base"
          value={generateLibraryID}
          onChange={(e) => setGenerateLibraryID(e.target.value)}
        >
          <option value="" disabled>
            选择媒体库
          </option>
          <option value="*">全部媒体库</option>
          {libraries.map((library) => (
            <option key={library.id} value={library.id}>
              {library.name} ({library.type})
            </option>
          ))}
        </select>
        <input
          required
          className="input-base md:col-span-2"
          placeholder="http://NAS-IP:18080 或 https://media.example.com"
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
        />
        <button
          type="button"
          className="rounded-2xl border border-primary-400/40 px-3 py-2 text-sm text-brand-500 transition hover:bg-primary-400/10"
          onClick={() => setBaseURL(currentOrigin())}
        >
          使用当前访问地址
        </button>
        <label className="flex items-center gap-2 rounded-2xl border border-gray-200 bg-white/70 px-3 py-2 text-sm text-ink-50">
          <input
            type="checkbox"
            checked={overwrite}
            onChange={(e) => setOverwrite(e.target.checked)}
          />
          覆盖已存在
        </label>
        <input
          className="input-base md:col-span-3"
          placeholder="输出目录可留空，默认写入 data/strm/分类/子分类"
          value={outputDir}
          onChange={(e) => setOutputDir(e.target.value)}
        />
        <button type="submit" disabled={generating || !generateLibraryID || !baseURL.trim()} className="neon-button md:col-span-4">
          {generating ? <Loader2 size={16} className="animate-spin" /> : <Wand2 size={16} />}
          {generating ? '生成中…' : '批量生成 STRM'}
        </button>
      </form>
      <p className="text-xs text-sand-500">
        生成内容为 <code>域名 + /api/stream/媒体ID?token=...</code>；第三方客户端播放优先方式由上方「STRMURL / 302反代」模式决定。域名会同步保存到系统设置中的「公开访问域名 / STRM 域名」。
      </p>
      {generateResult && (
        <div className="rounded-2xl border border-gray-200 bg-gray-50 p-4 text-sm text-ink-50">
          <div className="font-semibold text-ink-600">
            输出目录：{generateResult.output_dir}
          </div>
          <div className="mt-1">
            新增 {generateResult.generated} · 更新 {generateResult.updated} · 跳过 {generateResult.skipped} · 清理 {generateResult.cleaned || 0}
          </div>
          {generateResult.errors && generateResult.errors.length > 0 && (
            <div className="mt-2 text-red-500">
              失败 {generateResult.errors.length} 条：{generateResult.errors.slice(0, 3).join('；')}
            </div>
          )}
        </div>
      )}
    </section>
  )
}
