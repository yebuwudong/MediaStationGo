import { ChangeEvent, FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Sparkles } from 'lucide-react'

import { aiAPI, type SearchIntent } from '../api/ai'
import { mediaAPI } from '../api/library'
import { MediaCard } from '../components/MediaCard'
import type { Media } from '../types'

// SearchPage runs a fast LIKE search by default, with an optional AI
// "smart search" toggle that calls /api/ai/search to produce a structured
// intent before executing the library lookup.
export function SearchPage() {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Media[]>([])
  const [loading, setLoading] = useState(false)
  const [aiOn, setAiOn] = useState(false)
  const [aiAvailable, setAiAvailable] = useState(false)
  const [intent, setIntent] = useState<SearchIntent | null>(null)

  useEffect(() => {
    aiAPI
      .status()
      .then((s) => setAiAvailable(s.enabled))
      .catch(() => setAiAvailable(false))
  }, [])

  // Fast LIKE search-as-you-type when AI mode is OFF.
  useEffect(() => {
    if (aiOn) return
    const t = setTimeout(() => {
      setLoading(true)
      mediaAPI
        .search(q, 60)
        .then((d) => {
          setItems(d.items)
          setIntent(null)
        })
        .finally(() => setLoading(false))
    }, 300)
    return () => clearTimeout(t)
  }, [q, aiOn])

  const onAISubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!q.trim()) return
    setLoading(true)
    try {
      const data = await aiAPI.smartSearch(q)
      setItems(data.items)
      setIntent(data.intent)
    } catch {
      toast.error('AI 搜索失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <h1 className="font-display text-3xl font-bold text-white">搜索</h1>
        <button
          disabled={!aiAvailable}
          className={
            'neon-button !px-3 !py-1 !text-xs ' +
            (aiOn ? '!border-accent-400 !bg-accent-400/20 !text-accent-400' : '')
          }
          onClick={() => setAiOn((on) => !on)}
          title={aiAvailable ? '启用 AI 智能搜索' : '请先在 ai.* 中配置 API Key'}
        >
          <Sparkles size={12} /> {aiOn ? 'AI 已开启' : 'AI 智能搜索'}
        </button>
      </header>

      {aiOn ? (
        <form onSubmit={onAISubmit} className="flex gap-2">
          <input
            autoFocus
            className="input-base"
            placeholder='例如:“2010 年后的科幻电影” / “最近的动漫”'
            value={q}
            onChange={(e: ChangeEvent<HTMLInputElement>) => setQ(e.target.value)}
          />
          <button type="submit" className="neon-button">
            搜索
          </button>
        </form>
      ) : (
        <input
          autoFocus
          className="input-base"
          placeholder="按标题搜索…"
          value={q}
          onChange={(e: ChangeEvent<HTMLInputElement>) => setQ(e.target.value)}
        />
      )}

      {intent && (
        <div className="glass-panel !p-3 text-xs text-slate-300">
          AI 解析:
          <span className="ml-2 font-mono text-primary-400">{JSON.stringify(intent)}</span>
        </div>
      )}

      {loading && <p className="text-slate-500">搜索中…</p>}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        {items.map((m) => (
          <MediaCard key={m.id} media={m} />
        ))}
      </div>
    </div>
  )
}
