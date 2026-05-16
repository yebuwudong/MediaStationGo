import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Plug, Trash2, Zap, Settings, Loader2 } from 'lucide-react'

import { downloadClientAPI, type DownloadClientCreateParams } from '../api/downloadClient'
import type { DownloadClient } from '../types'

const CLIENT_TYPES = [
  { value: 'qbittorrent', label: 'qBittorrent' },
  { value: 'transmission', label: 'Transmission' },
  { value: 'aria2', label: 'Aria2' },
] as const

const TYPE_LABELS: Record<string, string> = {
  qbittorrent: 'qBittorrent',
  transmission: 'Transmission',
  aria2: 'Aria2',
}

export function DownloadClientCard() {
  const [clients, setClients] = useState<DownloadClient[]>([])
  const [showForm, setShowForm] = useState(false)
  const [testing, setTesting] = useState<string | null>(null)
  const [form, setForm] = useState<DownloadClientCreateParams>({
    name: '',
    type: 'qbittorrent',
    host: '',
    username: '',
    password: '',
    is_default: false,
  })

  const refresh = () => downloadClientAPI.list().then(setClients)
  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await downloadClientAPI.create(form)
      toast.success('下载客户端已创建')
      setForm({ name: '', type: 'qbittorrent', host: '', username: '', password: '', is_default: false })
      setShowForm(false)
      await refresh()
    } catch {
      toast.error('创建失败')
    }
  }

  const handleTest = async (id: string) => {
    setTesting(id)
    try {
      await downloadClientAPI.test(id)
      toast.success('连接测试成功')
    } catch {
      toast.error('连接测试失败')
    } finally {
      setTesting(null)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`确定删除「${name}」?`)) return
    await downloadClientAPI.delete(id)
    toast.success('已删除')
    await refresh()
  }

  const handleToggleDefault = async (client: DownloadClient) => {
    await downloadClientAPI.update(client.id, { is_default: true })
    toast.success(`已设「${client.name}」为默认客户端`)
    await refresh()
  }

  const handleToggleEnabled = async (client: DownloadClient) => {
    await downloadClientAPI.update(client.id, { enabled: !client.enabled })
    toast.success(client.enabled ? '已禁用' : '已启用')
    await refresh()
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-white">下载客户端</h2>
        <button
          className="neon-button flex items-center gap-1 text-sm"
          onClick={() => setShowForm(!showForm)}
        >
          <Plug size={14} />
          添加客户端
        </button>
      </div>

      {showForm && (
        <form onSubmit={handleSubmit} className="glass-panel space-y-3">
          <div className="grid gap-3 md:grid-cols-2">
            <input
              required
              className="input-base"
              placeholder="名称"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
            <select
              className="input-base"
              value={form.type}
              onChange={(e) =>
                setForm({ ...form, type: e.target.value as DownloadClientCreateParams['type'] })
              }
            >
              {CLIENT_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
          <input
            required
            className="input-base"
            placeholder="地址 (如 http://127.0.0.1:8080)"
            value={form.host}
            onChange={(e) => setForm({ ...form, host: e.target.value })}
          />
          <div className="grid gap-3 md:grid-cols-2">
            <input
              className="input-base"
              placeholder="用户名"
              value={form.username}
              onChange={(e) => setForm({ ...form, username: e.target.value })}
            />
            <input
              className="input-base"
              type="password"
              placeholder="密码"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
            />
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input
              type="checkbox"
              checked={form.is_default}
              onChange={(e) => setForm({ ...form, is_default: e.target.checked })}
              className="rounded border-slate-600"
            />
            设为默认客户端
          </label>
          <div className="flex gap-2">
            <button type="submit" className="neon-button">
              创建
            </button>
            <button
              type="button"
              className="rounded border border-white/20 px-3 py-1.5 text-sm text-slate-300 hover:bg-white/5"
              onClick={() => setShowForm(false)}
            >
              取消
            </button>
          </div>
        </form>
      )}

      <div className="space-y-2">
        {clients.map((client) => (
          <div
            key={client.id}
            className={`glass-panel flex items-center justify-between ${
              !client.enabled ? 'opacity-50' : ''
            }`}
          >
            <div className="flex items-center gap-3">
              <div
                className={`rounded-lg p-2 ${
                  client.type === 'qbittorrent'
                    ? 'bg-blue-500/20 text-blue-400'
                    : client.type === 'transmission'
                      ? 'bg-purple-500/20 text-purple-400'
                      : 'bg-orange-500/20 text-orange-400'
                }`}
              >
                <Settings size={18} />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-white">{client.name}</span>
                  <span className="rounded bg-white/10 px-1.5 py-0.5 text-xs text-slate-400">
                    {TYPE_LABELS[client.type]}
                  </span>
                  {client.is_default && (
                    <span className="rounded bg-primary-400/20 px-1.5 py-0.5 text-xs text-primary-400">
                      默认
                    </span>
                  )}
                  {!client.enabled && (
                    <span className="rounded bg-red-400/20 px-1.5 py-0.5 text-xs text-red-400">
                      已禁用
                    </span>
                  )}
                </div>
                <span className="text-xs text-slate-500">{client.host}</span>
              </div>
            </div>
            <div className="flex items-center gap-1">
              {!client.is_default && (
                <button
                  className="rounded border border-primary-400/40 px-2 py-1 text-xs text-primary-400 hover:bg-primary-400/10"
                  onClick={() => handleToggleDefault(client)}
                  title="设为默认"
                >
                  <Zap size={12} />
                </button>
              )}
              <button
                className="rounded border border-yellow-400/40 px-2 py-1 text-xs text-yellow-400 hover:bg-yellow-400/10"
                onClick={() => handleToggleEnabled(client)}
                title={client.enabled ? '禁用' : '启用'}
              >
                {client.enabled ? '暂停' : '启用'}
              </button>
              <button
                className="rounded border border-primary-400/40 px-2 py-1 text-xs text-primary-400 hover:bg-primary-400/10"
                onClick={() => handleTest(client.id)}
                disabled={testing === client.id}
              >
                {testing === client.id ? (
                  <Loader2 size={12} className="animate-spin" />
                ) : (
                  '测试'
                )}
              </button>
              <button
                className="rounded border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                onClick={() => handleDelete(client.id, client.name)}
              >
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        ))}
        {clients.length === 0 && (
          <div className="py-8 text-center text-sm text-slate-500">
            暂无下载客户端，点击上方按钮添加
          </div>
        )}
      </div>
    </div>
  )
}
