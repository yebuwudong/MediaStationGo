import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Trash2 } from 'lucide-react'

import { libraryAPI } from '../api/library'
import type { Library } from '../types'
import { confirmAction } from '../components/confirmAction'

export function AdminLibraryPanel() {
  const [libs, setLibs] = useState<Library[]>([])
  const [name, setName] = useState('')
  const [path, setPath] = useState('')
  const [type, setType] = useState('movie')

  const refresh = () => libraryAPI.list({ includeHidden: true }).then(setLibs)
  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await libraryAPI.create(name, path, type)
      toast.success('媒体库已创建')
      setName('')
      setPath('')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '创建失败'
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <form onSubmit={handleCreate} className="glass-panel grid gap-3 md:grid-cols-4">
        <input
          required
          className="input-base"
          placeholder="名称"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          required
          className="input-base md:col-span-2"
          placeholder="容器路径，如 /media/电视剧/国产剧"
          value={path}
          onChange={(e) => setPath(e.target.value)}
        />
        <p className="md:col-span-4 -mt-2 text-xs text-sand-500">
          Docker 部署时请优先填写容器内路径，例如 /media/电影、/media/电视剧/国产剧；如果误填 NAS
          宿主机路径，系统会尝试按 compose 挂载自动转换。
        </p>
        <select className="input-base" value={type} onChange={(e) => setType(e.target.value)}>
          <option value="movie">电影</option>
          <option value="tv">电视剧</option>
          <option value="variety">综艺</option>
          <option value="anime">动漫</option>
          <option value="music">音乐</option>
        </select>
        <button type="submit" className="neon-button md:col-span-4">
          新建媒体库
        </button>
      </form>

      <div className="glass-panel">
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase tracking-wider text-sand-500">
            <tr>
              <th className="py-2">名称</th>
              <th>路径</th>
              <th>类型</th>
              <th className="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {libs.map((l) => (
              <tr key={l.id} className="border-t border-gray-200">
                <td className="py-2 text-ink-600">{l.name}</td>
                <td className="text-ink-100">{l.path}</td>
                <td className="text-ink-100">{l.type}</td>
                <td className="space-x-2 py-2 text-right">
                  <button
                    className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
                    onClick={async () => {
                      const r = await libraryAPI.scan(l.id)
                      if (r.queued) toast.success('云盘扫描已加入后台队列，会自动入库')
                      else toast.success(`扫描完成，新增 ${r.added}，更新 ${r.updated ?? 0}`)
                    }}
                  >
                    扫描
                  </button>
                  <button
                    className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                    onClick={async () => {
                      if (!(await confirmAction({ title: '删除媒体库', message: `确定删除「${l.name}」?`, confirmText: '删除' }))) return
                      await libraryAPI.remove(l.id)
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
    </div>
  )
}
