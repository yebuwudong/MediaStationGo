import { useState } from 'react'
import toast from 'react-hot-toast'
import { Download, Rss, X } from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { buildSiteSearchFeedURL, buildSubscriptionAliases, subscriptionsAPI } from '../api/subscriptions'
import { buildSubscribeKeyword, discoverItemSource } from './discoverPageModel'

export function DiscoverDetailModal({ item, onClose }: { item: DiscoverItem; onClose: () => void }) {
  const source = discoverItemSource(item)
  const keyword = item.subscribe_keyword || buildSubscribeKeyword(item)
  const [form, setForm] = useState({
    keyword,
    search_mode: 'keyword',
    imdb_id: '',
    media_type: item.media_type || '',
    resolution: 'best',
    quality: '',
    effects: '',
    release_groups: '',
    exclude_words: 'cam,ts,tc,枪版',
    wash_enabled: false,
    wash_priority: 'balanced',
    save_path: '',
    media_category: '',
    priority: 50,
    run_now: true,
  })
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    const finalKeyword = form.keyword.trim() || keyword
    const feed = buildSiteSearchFeedURL(finalKeyword, source, buildSubscriptionAliases(item))
    setBusy(true)
    try {
      const sub = await subscriptionsAPI.create({
        name: `${item.title} 自动订阅`,
        feed_url: feed,
        filter: finalKeyword,
        media_type: form.media_type || undefined,
        media_category: form.media_category || undefined,
        save_path: form.save_path || undefined,
        search_mode: form.search_mode,
        imdb_id: form.imdb_id || undefined,
        source,
        poster_url: item.poster_url || undefined,
        backdrop_url: item.backdrop_url || undefined,
        overview: item.overview || undefined,
        original_name: item.original_name || undefined,
        year: item.year || undefined,
        resolution: form.resolution === 'best' ? 'best' : form.resolution,
        quality: form.quality || undefined,
        effects: form.effects || undefined,
        release_groups: form.release_groups || undefined,
        exclude_words: form.exclude_words || undefined,
        wash_enabled: form.wash_enabled,
        wash_priority: form.wash_priority,
        priority: form.priority,
        enabled: true,
      })
      if (form.run_now) {
        const run = await subscriptionsAPI.runNow(sub.id)
        toast.success(run.queued > 0 ? `已订阅并加入 ${run.queued} 个下载` : '已订阅，暂未命中可下载资源')
      } else {
        toast.success('已创建订阅')
      }
      onClose()
    } catch (err) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '订阅失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm">
      <div className="max-h-[92vh] w-full max-w-5xl overflow-y-auto rounded-3xl border border-white/60 bg-white p-5 shadow-2xl">
        <div className="mb-4 flex items-start justify-between gap-3">
          <div>
            <p className="text-xs font-semibold uppercase tracking-widest text-brand-500">{source}</p>
            <h2 className="font-display text-2xl font-bold text-ink-600">{item.title}</h2>
            <p className="mt-1 text-sm text-sand-500">
              {[item.media_type, item.year && item.year > 0 ? item.year : '', item.rating ? `★ ${item.rating.toFixed(1)}` : '']
                .filter(Boolean)
                .join(' · ')}
            </p>
          </div>
          <button className="rounded-full border border-gray-200 p-2 text-ink-50 hover:bg-gray-50" onClick={onClose}>
            <X size={18} />
          </button>
        </div>

        <div className="grid gap-5 lg:grid-cols-[260px_1fr]">
          <div className="space-y-3">
            <div className="overflow-hidden rounded-2xl bg-gray-100">
              {item.poster_url ? (
                <img src={imageURL(item.poster_url)} alt={item.title} className="aspect-[2/3] w-full object-cover" />
              ) : (
                <div className="flex aspect-[2/3] items-center justify-center text-sand-500">无海报</div>
              )}
            </div>
            {item.backdrop_url && (
              <img src={imageURL(item.backdrop_url)} alt="" className="h-24 w-full rounded-2xl object-cover" />
            )}
          </div>

          <div className="space-y-5">
            <section className="rounded-2xl border border-gray-200 bg-gray-50 p-4">
              <h3 className="mb-2 font-semibold text-ink-600">简介</h3>
              <p className="text-sm leading-6 text-ink-100">{item.overview || '当前数据源没有返回简介。'}</p>
            </section>

            <section className="rounded-2xl border border-primary-400/20 bg-primary-400/5 p-4">
              <h3 className="mb-3 flex items-center gap-2 font-semibold text-ink-600">
                <Rss size={16} />
                订阅下载规则
              </h3>
              <div className="grid gap-3 md:grid-cols-3">
                <label className="text-xs text-sand-500 md:col-span-2">
                  搜索关键词
                  <input className="input-base mt-1" value={form.keyword} onChange={(e) => setForm((f) => ({ ...f, keyword: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  搜索方式
                  <select className="input-base mt-1" value={form.search_mode} onChange={(e) => setForm((f) => ({ ...f, search_mode: e.target.value }))}>
                    <option value="keyword">标题关键词</option>
                    <option value="imdb">IMDB ID</option>
                  </select>
                </label>
                <label className="text-xs text-sand-500">
                  IMDB ID
                  <input className="input-base mt-1" placeholder="tt1160419" value={form.imdb_id} onChange={(e) => setForm((f) => ({ ...f, imdb_id: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  类型
                  <select className="input-base mt-1" value={form.media_type} onChange={(e) => setForm((f) => ({ ...f, media_type: e.target.value }))}>
                    <option value="">自动识别</option>
                    <option value="movie">电影</option>
                    <option value="tv">电视剧</option>
                    <option value="anime">动漫</option>
                    <option value="variety">综艺</option>
                  </select>
                </label>
                <label className="text-xs text-sand-500">
                  分辨率
                  <select className="input-base mt-1" value={form.resolution} onChange={(e) => setForm((f) => ({ ...f, resolution: e.target.value }))}>
                    <option value="best">自动择优</option>
                    <option value="2160p">2160p / 4K</option>
                    <option value="1080p">1080p</option>
                    <option value="720p">720p</option>
                  </select>
                </label>
                <label className="text-xs text-sand-500">
                  质量
                  <select className="input-base mt-1" value={form.quality} onChange={(e) => setForm((f) => ({ ...f, quality: e.target.value }))}>
                    <option value="">不限</option>
                    <option value="remux">REMUX</option>
                    <option value="bluray">BluRay</option>
                    <option value="web-dl">WEB-DL</option>
                    <option value="hdtv">HDTV</option>
                  </select>
                </label>
                <label className="text-xs text-sand-500">
                  特效 / 音轨
                  <input className="input-base mt-1" placeholder="hdr,dolby-vision,atmos" value={form.effects} onChange={(e) => setForm((f) => ({ ...f, effects: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  洗版优先级
                  <select className="input-base mt-1 disabled:opacity-50" disabled={!form.wash_enabled} value={form.wash_priority} onChange={(e) => setForm((f) => ({ ...f, wash_priority: e.target.value }))}>
                    <option value="balanced">均衡</option>
                    <option value="resolution">分辨率优先</option>
                    <option value="quality">片源质量优先</option>
                    <option value="effects">HDR/DV/Atmos 优先</option>
                    <option value="seeders">做种数优先</option>
                  </select>
                </label>
                <label className="flex items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs text-ink-100">
                  <input type="checkbox" checked={form.wash_enabled} onChange={(e) => setForm((f) => ({ ...f, wash_enabled: e.target.checked }))} />
                  启用洗版择优
                </label>
                <label className="text-xs text-sand-500">
                  发布组
                  <input className="input-base mt-1" placeholder="如 FRDS,OurTV" value={form.release_groups} onChange={(e) => setForm((f) => ({ ...f, release_groups: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  排除词
                  <input className="input-base mt-1" value={form.exclude_words} onChange={(e) => setForm((f) => ({ ...f, exclude_words: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  分类覆盖
                  <input className="input-base mt-1" placeholder="综艺 / 日番 / 欧美剧" value={form.media_category} onChange={(e) => setForm((f) => ({ ...f, media_category: e.target.value }))} />
                </label>
                <label className="text-xs text-sand-500">
                  保存路径覆盖
                  <input className="input-base mt-1" value={form.save_path} onChange={(e) => setForm((f) => ({ ...f, save_path: e.target.value }))} />
                </label>
              </div>
              <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                <label className="flex items-center gap-2 text-sm text-ink-100">
                  <input type="checkbox" checked={form.run_now} onChange={(e) => setForm((f) => ({ ...f, run_now: e.target.checked }))} />
                  创建后立即搜索并下载
                </label>
                <button disabled={busy} onClick={submit} className="neon-button disabled:opacity-60">
                  <Download size={16} />
                  {busy ? '处理中…' : '创建订阅'}
                </button>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>
  )
}
