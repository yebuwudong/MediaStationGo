import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Bell, Trash2, Loader2 } from 'lucide-react'

import { notifyAPI, type NotifyChannelCreateParams } from '../api/notify'
import type { NotifyChannel } from '../types'

const CHANNEL_TYPES = [
  { value: 'telegram', label: 'Telegram' },
  { value: 'wechat', label: 'Server酱' },
  { value: 'bark', label: 'Bark' },
  { value: 'webhook', label: 'Webhook' },
  { value: 'email', label: 'Email' },
] as const

const TYPE_LABELS: Record<string, string> = {
  telegram: 'Telegram',
  wechat: 'Server酱',
  bark: 'Bark',
  webhook: 'Webhook',
  email: 'Email',
}

const EVENT_OPTIONS = [
  { value: 'subscription_hit', label: '订阅命中' },
  { value: 'download_complete', label: '下载完成' },
  { value: 'scrape_failed', label: '刮削失败' },
  { value: 'system_alert', label: '系统告警' },
]

const CONFIG_FIELDS: Record<string, { key: string; label: string; type?: string; placeholder: string }[]> = {
  telegram: [
    { key: 'bot_token', label: 'Bot Token', placeholder: '123456:ABC-DEF...' },
    { key: 'chat_id', label: 'Chat ID', placeholder: '你的 Chat ID' },
    { key: 'parse_mode', label: 'Parse Mode', placeholder: 'HTML (可选)' },
  ],
  wechat: [{ key: 'sendkey', label: 'SendKey', placeholder: 'SCT...' }],
  bark: [
    { key: 'server_url', label: '服务器地址', placeholder: 'https://api.day.app (可选)' },
    { key: 'device_key', label: 'Device Key', placeholder: '你的 Bark Key' },
  ],
  webhook: [
    { key: 'url', label: 'Webhook URL', placeholder: 'https://...' },
    { key: 'method', label: 'HTTP 方法', placeholder: 'POST (可选)' },
    { key: 'headers_json', label: 'Headers JSON', placeholder: '{"Authorization":"Bearer ..."} (可选)' },
    { key: 'body_template', label: 'Body 模板', placeholder: '{{title}}: {{message}} (可选)' },
  ],
  email: [
    { key: 'smtp_host', label: 'SMTP 地址', placeholder: 'smtp.gmail.com' },
    { key: 'smtp_port', label: 'SMTP 端口', placeholder: '465' },
    { key: 'username', label: '用户名', placeholder: '' },
    { key: 'password', label: '密码', placeholder: '', type: 'password' },
    { key: 'from', label: '发件人', placeholder: 'noreply@example.com' },
    { key: 'to', label: '收件人', placeholder: 'user@example.com (多个用逗号分隔)' },
    { key: 'tls', label: 'TLS', placeholder: 'true/false' },
  ],
}

export function NotifyChannelCard() {
  const [channels, setChannels] = useState<NotifyChannel[]>([])
  const [showForm, setShowForm] = useState(false)
  const [testing, setTesting] = useState<string | null>(null)
  const [form, setForm] = useState<NotifyChannelCreateParams>({
    name: '',
    type: 'telegram',
    enabled: true,
    config: {},
    events: [],
  })

  const refresh = () => notifyAPI.list().then(setChannels)
  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const handleTypeChange = (type: string) => {
    setForm({
      ...form,
      type: type as NotifyChannelCreateParams['type'],
      config: {},
    })
  }

  const handleConfigChange = (key: string, value: string) => {
    setForm({ ...form, config: { ...form.config, [key]: value } })
  }

  const handleEventToggle = (event: string) => {
    const events = form.events || []
    const idx = events.indexOf(event)
    if (idx >= 0) {
      events.splice(idx, 1)
    } else {
      events.push(event)
    }
    setForm({ ...form, events: [...events] })
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    try {
      await notifyAPI.create(form)
      toast.success('通知渠道已创建')
      setForm({ name: '', type: 'telegram', enabled: true, config: {}, events: [] })
      setShowForm(false)
      await refresh()
    } catch {
      toast.error('创建失败')
    }
  }

  const handleTest = async (id: string) => {
    setTesting(id)
    try {
      await notifyAPI.test(id)
      toast.success('测试通知已发送')
    } catch {
      toast.error('测试通知发送失败')
    } finally {
      setTesting(null)
    }
  }

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`确定删除「${name}」?`)) return
    await notifyAPI.delete(id)
    toast.success('已删除')
    await refresh()
  }

  const handleToggleEnabled = async (channel: NotifyChannel) => {
    await notifyAPI.update(channel.id, { enabled: !channel.enabled })
    toast.success(channel.enabled ? '已禁用' : '已启用')
    await refresh()
  }

  const configFields = CONFIG_FIELDS[form.type] || []

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-white">通知渠道</h2>
        <button
          className="neon-button flex items-center gap-1 text-sm"
          onClick={() => setShowForm(!showForm)}
        >
          <Bell size={14} />
          添加渠道
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
            <select className="input-base" value={form.type} onChange={(e) => handleTypeChange(e.target.value)}>
              {CHANNEL_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>

          {configFields.map((field) => (
            <input
              key={field.key}
              required={field.key !== 'parse_mode' && field.key !== 'method' && field.key !== 'headers_json' && field.key !== 'body_template' && field.key !== 'server_url' && field.key !== 'tls'}
              className="input-base"
              type={field.type || 'text'}
              placeholder={field.label + (field.placeholder ? ` (${field.placeholder})` : '')}
              value={form.config[field.key] || ''}
              onChange={(e) => handleConfigChange(field.key, e.target.value)}
            />
          ))}

          <div>
            <label className="mb-1 block text-xs text-slate-500">订阅事件</label>
            <div className="flex flex-wrap gap-2">
              {EVENT_OPTIONS.map((event) => (
                <label
                  key={event.value}
                  className="flex cursor-pointer items-center gap-1 rounded border border-white/10 px-2 py-1 text-xs text-slate-300"
                >
                  <input
                    type="checkbox"
                    checked={(form.events || []).includes(event.value)}
                    onChange={() => handleEventToggle(event.value)}
                    className="rounded border-slate-600"
                  />
                  {event.label}
                </label>
              ))}
            </div>
          </div>

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
        {channels.map((channel) => (
          <div
            key={channel.id}
            className={`glass-panel flex items-center justify-between ${
              !channel.enabled ? 'opacity-50' : ''
            }`}
          >
            <div className="flex items-center gap-3">
              <div className="rounded-lg bg-green-500/20 p-2 text-green-400">
                <Bell size={18} />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-white">{channel.name}</span>
                  <span className="rounded bg-white/10 px-1.5 py-0.5 text-xs text-slate-400">
                    {TYPE_LABELS[channel.type]}
                  </span>
                  {!channel.enabled && (
                    <span className="rounded bg-red-400/20 px-1.5 py-0.5 text-xs text-red-400">
                      已禁用
                    </span>
                  )}
                </div>
                <span className="text-xs text-slate-500">
                  事件: {channel.events || '无'}
                </span>
              </div>
            </div>
            <div className="flex items-center gap-1">
              <button
                className="rounded border border-yellow-400/40 px-2 py-1 text-xs text-yellow-400 hover:bg-yellow-400/10"
                onClick={() => handleToggleEnabled(channel)}
              >
                {channel.enabled ? '禁用' : '启用'}
              </button>
              <button
                className="rounded border border-primary-400/40 px-2 py-1 text-xs text-primary-400 hover:bg-primary-400/10"
                onClick={() => handleTest(channel.id)}
                disabled={testing === channel.id}
              >
                {testing === channel.id ? (
                  <Loader2 size={12} className="animate-spin" />
                ) : (
                  '测试'
                )}
              </button>
              <button
                className="rounded border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
                onClick={() => handleDelete(channel.id, channel.name)}
              >
                <Trash2 size={12} />
              </button>
            </div>
          </div>
        ))}
        {channels.length === 0 && (
          <div className="py-8 text-center text-sm text-slate-500">
            暂无通知渠道，点击上方按钮添加
          </div>
        )}
      </div>
    </div>
  )
}
