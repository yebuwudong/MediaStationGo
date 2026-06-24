import { FormEvent, useState, type ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'

import { playProfilesAPI, type PlayProfileInput } from '../api/play_profiles'
import type { Library, PlayProfile } from '../types'

type ProfileFormModalProps = {
  editing: PlayProfile | null
  libraries: Library[]
  defaultUserID: string
  onClose: () => void
  onSaved: () => void | Promise<void>
}

export function ProfileFormModal({
  editing,
  libraries,
  defaultUserID,
  onClose,
  onSaved,
}: ProfileFormModalProps) {
  const [form, setForm] = useState<PlayProfileInput>(() => ({
    user_id: editing?.user_id ?? defaultUserID,
    name: editing?.name ?? '',
    is_default: editing?.is_default ?? false,
    content_rating_limit: editing?.content_rating_limit ?? '',
    allow_adult: editing?.allow_adult ?? false,
    require_pin: editing?.require_pin ?? false,
    pin: '',
    preferred_subtitle_lang: editing?.preferred_subtitle_lang ?? '',
    preferred_audio_lang: editing?.preferred_audio_lang ?? '',
    autoplay_next: editing?.autoplay_next ?? true,
    skip_intro: editing?.skip_intro ?? false,
    allowed_library_ids: editing?.allowed_library_ids ?? [],
  }))
  const [saving, setSaving] = useState(false)

  const update = (patch: Partial<PlayProfileInput>) => setForm((current) => ({ ...current, ...patch }))

  const toggleLib = (id: string) => {
    const next = form.allowed_library_ids.includes(id)
      ? form.allowed_library_ids.filter((item) => item !== id)
      : [...form.allowed_library_ids, id]
    update({ allowed_library_ids: next })
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      if (editing) {
        await playProfilesAPI.update(editing.id, form)
      } else {
        await playProfilesAPI.create(form)
      }
      toast.success('已保存')
      await onSaved()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="glass-panel w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="mb-4 font-display text-xl font-semibold text-ink-600">
          {editing ? '编辑 Profile' : '创建 Profile'}
        </h2>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label="名称">
            <input
              required
              className="input-base"
              maxLength={50}
              placeholder="如:儿童模式、影院模式"
              value={form.name}
              onChange={(event) => update({ name: event.target.value })}
            />
          </Field>

          <Toggle
            label="设为默认 Profile"
            hint="登录后默认使用"
            checked={form.is_default}
            onChange={(value) => update({ is_default: value })}
          />

          <Field label="内容分级限制">
            <select
              className="input-base"
              value={form.content_rating_limit ?? ''}
              onChange={(event) => update({ content_rating_limit: event.target.value })}
            >
              <option value="">不限制</option>
              <option value="G">G</option>
              <option value="PG">PG</option>
              <option value="PG-13">PG-13</option>
              <option value="R">R</option>
              <option value="NC-17">NC-17</option>
            </select>
          </Field>

          <Toggle
            label="允许成人内容"
            hint="开启后可访问 NSFW 媒体"
            checked={form.allow_adult}
            onChange={(value) => update({ allow_adult: value })}
          />

          <Toggle
            label="切换时需要 PIN"
            hint="切换到此 Profile 需输入 PIN 码"
            checked={form.require_pin}
            onChange={(value) => update({ require_pin: value })}
          />
          {form.require_pin && (
            <Field label="PIN 码 (4-8 位)">
              <input
                type="password"
                className="input-base"
                maxLength={8}
                placeholder={editing ? '留空保持不变' : '设置 PIN'}
                value={form.pin ?? ''}
                onChange={(event) => update({ pin: event.target.value })}
              />
            </Field>
          )}

          <div className="grid grid-cols-2 gap-3">
            <Field label="首选字幕">
              <select
                className="input-base"
                value={form.preferred_subtitle_lang ?? ''}
                onChange={(event) => update({ preferred_subtitle_lang: event.target.value })}
              >
                <option value="">跟随系统</option>
                <option value="zh">中文</option>
                <option value="zh-CN">简体中文</option>
                <option value="en">English</option>
                <option value="ja">日语</option>
              </select>
            </Field>
            <Field label="首选音轨">
              <select
                className="input-base"
                value={form.preferred_audio_lang ?? ''}
                onChange={(event) => update({ preferred_audio_lang: event.target.value })}
              >
                <option value="">跟随系统</option>
                <option value="zh">中文</option>
                <option value="ja">日语</option>
                <option value="en">English</option>
              </select>
            </Field>
          </div>

          <Toggle
            label="自动播放下一集"
            checked={form.autoplay_next}
            onChange={(value) => update({ autoplay_next: value })}
          />
          <Toggle
            label="自动跳过片头"
            checked={form.skip_intro}
            onChange={(value) => update({ skip_intro: value })}
          />

          <Field label="允许访问的媒体库 (空 = 全部)">
            <div className="flex flex-wrap gap-2">
              {libraries.map((library) => (
                <button
                  key={library.id}
                  type="button"
                  onClick={() => toggleLib(library.id)}
                  className={
                    'rounded-full border px-3 py-1 text-xs ' +
                    (form.allowed_library_ids.includes(library.id)
                      ? 'border-primary-400/60 bg-primary-400/10 text-brand-500'
                      : 'border-gray-200 text-ink-50 hover:text-white')
                  }
                >
                  {library.name}
                </button>
              ))}
              {libraries.length === 0 && (
                <span className="text-xs text-sand-500">暂无媒体库</span>
              )}
            </div>
          </Field>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50"
            >
              取消
            </button>
            <button type="submit" disabled={saving} className="neon-button">
              {saving && <Loader2 size={16} className="animate-spin" />}
              保存
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-ink-100">{label}</span>
      {children}
    </label>
  )
}

function Toggle({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint?: string
  checked: boolean
  onChange: (value: boolean) => void
}) {
  return (
    <label className="flex cursor-pointer items-center justify-between gap-3 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2">
      <div>
        <div className="text-sm text-ink-600">{label}</div>
        {hint && <div className="text-xs text-ink-50">{hint}</div>}
      </div>
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-4 w-4 accent-primary-400"
      />
    </label>
  )
}
