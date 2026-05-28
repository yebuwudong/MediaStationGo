import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Pencil, Play, Plus, Save, Trash2 } from 'lucide-react'

import { subscriptionsAPI } from '../api/subscriptions'
import type { Subscription } from '../types'

export function SubscriptionsPage() {
  const [items, setItems] = useState<Subscription[]>([])
  const [name, setName] = useState('')
  const [feed, setFeed] = useState('')
  const [filter, setFilter] = useState('')
  const [mediaType, setMediaType] = useState('')
  const [mediaCategory, setMediaCategory] = useState('')
  const [savePath, setSavePath] = useState('')
  const [searchMode, setSearchMode] = useState('keyword')
  const [imdbID, setImdbID] = useState('')
  const [resolution, setResolution] = useState('best')
  const [quality, setQuality] = useState('')
  const [effects, setEffects] = useState('')
  const [releaseGroups, setReleaseGroups] = useState('')
  const [excludeWords, setExcludeWords] = useState('cam,ts,tc,枪版')
  const [washPriority, setWashPriority] = useState('balanced')
  const [editingId, setEditingId] = useState('')
  const [loading, setLoading] = useState(true)

  const refresh = () =>
    subscriptionsAPI
      .list()
      .then(setItems)
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      const payload = {
        name,
        feed_url: feed,
        filter,
        media_type: mediaType || undefined,
        media_category: mediaCategory || undefined,
        save_path: savePath || undefined,
        search_mode: searchMode,
        imdb_id: imdbID || undefined,
        resolution,
        quality: quality || undefined,
        effects: effects || undefined,
        release_groups: releaseGroups || undefined,
        exclude_words: excludeWords || undefined,
        wash_priority: washPriority,
        priority: 50,
      }
      if (editingId) {
        await subscriptionsAPI.update(editingId, payload)
        toast.success('已更新订阅')
      } else {
        await subscriptionsAPI.create(payload)
        toast.success('已创建订阅')
      }
      resetForm()
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '创建失败'
      toast.error(msg)
    }
  }

  const resetForm = () => {
    setEditingId('')
    setName('')
    setFeed('')
    setFilter('')
    setMediaType('')
    setMediaCategory('')
    setSavePath('')
    setSearchMode('keyword')
    setImdbID('')
    setResolution('best')
    setQuality('')
    setEffects('')
    setReleaseGroups('')
    setExcludeWords('cam,ts,tc,枪版')
    setWashPriority('balanced')
  }

  const startEdit = (s: Subscription) => {
    setEditingId(s.id)
    setName(s.name)
    setFeed(s.feed_url)
    setFilter(s.filter || '')
    setMediaType(s.media_type || '')
    setMediaCategory(s.media_category || '')
    setSavePath(s.save_path || '')
    setSearchMode(s.search_mode || 'keyword')
    setImdbID(s.imdb_id || '')
    setResolution(s.resolution || 'best')
    setQuality(s.quality || '')
    setEffects(s.effects || '')
    setReleaseGroups(s.release_groups || '')
    setExcludeWords(s.exclude_words || '')
    setWashPriority(s.wash_priority || 'balanced')
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  return (
    <div className="space-y-6">
      <h1 className="font-display text-3xl font-bold text-ink-600">RSS 订阅</h1>
      <p className="text-sm text-ink-50">
        定期轮询 RSS 源(每 10 分钟一次),将匹配过滤器的项目自动加入下载队列；启用智能分类后会按媒体类型和二级分类写入下载目录。
      </p>

      <form onSubmit={onCreate} className="glass-panel grid gap-3 md:grid-cols-4">
        <input
          required
          className="input-base"
          placeholder="名称(显示用)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          required
          className="input-base"
          placeholder="RSS 地址"
          value={feed}
          onChange={(e) => setFeed(e.target.value)}
        />
        <input
          className="input-base"
          placeholder="过滤器(正则,可选)"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <select className="input-base" value={mediaType} onChange={(e) => setMediaType(e.target.value)}>
          <option value="">自动识别类型</option>
          <option value="movie">电影</option>
          <option value="tv">电视剧</option>
          <option value="anime">动漫</option>
          <option value="variety">综艺</option>
        </select>
        <input
          className="input-base"
          placeholder="二级分类覆盖(如 综艺/日番,可选)"
          value={mediaCategory}
          onChange={(e) => setMediaCategory(e.target.value)}
        />
        <input
          className="input-base md:col-span-2"
          placeholder="下载根目录覆盖(可选,默认使用下载器保存路径)"
          value={savePath}
          onChange={(e) => setSavePath(e.target.value)}
        />
        <select className="input-base" value={searchMode} onChange={(e) => setSearchMode(e.target.value)}>
          <option value="keyword">标题关键词搜索</option>
          <option value="imdb">IMDB ID 搜索</option>
        </select>
        <input className="input-base" placeholder="IMDB ID，如 tt1160419" value={imdbID} onChange={(e) => setImdbID(e.target.value)} />
        <select className="input-base" value={resolution} onChange={(e) => setResolution(e.target.value)}>
          <option value="best">分辨率自动择优</option>
          <option value="2160p">2160p / 4K</option>
          <option value="1080p">1080p</option>
          <option value="720p">720p</option>
        </select>
        <select className="input-base" value={quality} onChange={(e) => setQuality(e.target.value)}>
          <option value="">质量不限</option>
          <option value="remux">REMUX</option>
          <option value="bluray">BluRay</option>
          <option value="web-dl">WEB-DL</option>
          <option value="hdtv">HDTV</option>
        </select>
        <input className="input-base" placeholder="特效/音轨 hdr,dolby-vision,atmos" value={effects} onChange={(e) => setEffects(e.target.value)} />
        <select className="input-base" value={washPriority} onChange={(e) => setWashPriority(e.target.value)}>
          <option value="balanced">洗版：均衡</option>
          <option value="resolution">洗版：分辨率优先</option>
          <option value="quality">洗版：片源质量优先</option>
          <option value="effects">洗版：HDR/DV/Atmos 优先</option>
          <option value="seeders">洗版：做种数优先</option>
        </select>
        <input className="input-base" placeholder="发布组白名单，如 FRDS,OurTV" value={releaseGroups} onChange={(e) => setReleaseGroups(e.target.value)} />
        <input className="input-base" placeholder="排除词，如 cam,ts,tc" value={excludeWords} onChange={(e) => setExcludeWords(e.target.value)} />
        <button type="submit" className="neon-button md:col-span-1">
          {editingId ? <Save size={16} /> : <Plus size={16} />}
          {editingId ? '保存' : '添加'}
        </button>
        {editingId && (
          <button type="button" onClick={resetForm} className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:bg-gray-50">
            取消编辑
          </button>
        )}
      </form>

      {loading && <p className="text-sand-500">加载中…</p>}
      {!loading && items.length === 0 && <p className="text-ink-50">暂无订阅。</p>}

      {items.length > 0 && (
        <div className="glass-panel">
          <table className="w-full text-left text-sm">
            <thead className="text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="py-2">名称</th>
                <th>RSS</th>
                <th>过滤器</th>
                <th>分类</th>
                <th>规则</th>
                <th>最近运行</th>
                <th className="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((s) => (
                <tr key={s.id} className="border-t border-gray-200">
                  <td className="py-2 text-ink-600">{s.name}</td>
                  <td className="max-w-md truncate text-ink-100" title={s.feed_url}>
                    {s.feed_url}
                  </td>
                  <td className="text-ink-100">{s.filter || '—'}</td>
                  <td className="text-ink-100">
                    {[s.media_type, s.media_category].filter(Boolean).join(' / ') || '自动'}
                  </td>
                  <td className="max-w-xs text-xs text-ink-100">
                    {[s.search_mode === 'imdb' ? `IMDB:${s.imdb_id || '未填'}` : '', s.resolution, s.quality, s.effects, s.wash_priority]
                      .filter(Boolean)
                      .join(' · ') || '默认'}
                  </td>
                  <td className="text-sand-500">
                    {s.last_run_at ? new Date(s.last_run_at).toLocaleString() : '—'}
                  </td>
                  <td className="space-x-2 py-2 text-right">
                    <button
                      className="rounded-lg border border-gray-300 px-2 py-1 text-xs text-ink-100 hover:bg-gray-50"
                      onClick={() => startEdit(s)}
                    >
                      <Pencil size={12} />
                    </button>
                    <button
                      className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                      onClick={async () => {
                        const r = await subscriptionsAPI.runNow(s.id)
                        toast.success(`已加入 ${r.queued} 项`)
                      }}
                    >
                      <Play size={12} />
                    </button>
                    <button
                      className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                      onClick={async () => {
                        if (!confirm(`删除订阅「${s.name}」?`)) return
                        await subscriptionsAPI.remove(s.id)
                        toast.success('已删除')
                        await refresh()
                      }}
                    >
                      <Trash2 size={12} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
