import { Field } from './NotifyChannelFormField'

type ProviderFieldsProps = {
  config: Record<string, string>
  updateConfig: (key: string, value: string) => void
}

export function TelegramFields({ config, updateConfig }: ProviderFieldsProps) {
  return (
    <>
      <Field label="Bot Token">
        <input
          required
          className="input-base"
          placeholder="123456:ABC-DEF…"
          value={config.bot_token ?? ''}
          onChange={(e) => updateConfig('bot_token', e.target.value)}
        />
      </Field>
      <Field label="管理员 Telegram ID">
        <input
          required
          className="input-base"
          placeholder="多个用逗号分隔，如 123456789,987654321"
          value={config.admin_user_ids ?? ''}
          onChange={(e) => updateConfig('admin_user_ids', e.target.value)}
        />
      </Field>
      <Field label="绑定群组 ID">
        <input
          className="input-base"
          placeholder="选填，如 -1001234567890；填写后群组成员可唤醒/绑定"
          value={config.group_chat_id ?? ''}
          onChange={(e) => updateConfig('group_chat_id', e.target.value)}
        />
      </Field>
      <Field label="绑定频道 ID">
        <input
          className="input-base"
          placeholder="选填，如 -1009876543210；填写后频道成员可唤醒/绑定"
          value={config.channel_chat_id ?? ''}
          onChange={(e) => updateConfig('channel_chat_id', e.target.value)}
        />
      </Field>
      <Field label="Telegram API 地址 (可选)">
        <input
          className="input-base"
          placeholder="默认 https://api.telegram.org；反代可填 https://tg.example.com"
          value={config.api_base_url ?? ''}
          onChange={(e) => updateConfig('api_base_url', e.target.value)}
        />
      </Field>
      <Field label="Telegram 代理 (可选)">
        <input
          className="input-base"
          placeholder="如 http://172.17.0.1:7890 或 socks5://172.17.0.1:1080"
          value={config.proxy_url ?? ''}
          onChange={(e) => updateConfig('proxy_url', e.target.value)}
        />
      </Field>
      <div className="rounded-2xl border border-primary-400/15 bg-primary-400/5 px-4 py-3 text-xs leading-6 text-ink-50">
        群组 ID、频道 ID 均为选填，可填一个、两个都填，也可以不填。管理功能始终仅管理员 Telegram ID 或已绑定的本地管理员可用；普通用户需要在已配置的群组或频道中，才能使用 <code>/start 用户名 密码</code> 绑定账号、切换隐藏成人媒体库和目录。不配置群组/频道时，普通用户不会被放行。若测试通知超时，可填写反代 API 地址或代理地址。
      </div>
    </>
  )
}

export function WechatFields({ config, updateConfig }: ProviderFieldsProps) {
  return (
    <Field label="SendKey">
      <input
        required
        className="input-base"
        placeholder="SCT…"
        value={config.sendkey ?? ''}
        onChange={(e) => updateConfig('sendkey', e.target.value)}
      />
    </Field>
  )
}

export function BarkFields({ config, updateConfig }: ProviderFieldsProps) {
  return (
    <>
      <Field label="设备 Key">
        <input
          required
          className="input-base"
          value={config.device_key ?? ''}
          onChange={(e) => updateConfig('device_key', e.target.value)}
        />
      </Field>
      <Field label="服务器地址 (可选)">
        <input
          className="input-base"
          placeholder="https://api.day.app"
          value={config.server ?? ''}
          onChange={(e) => updateConfig('server', e.target.value)}
        />
      </Field>
    </>
  )
}

export function WebhookFields({ config, updateConfig }: ProviderFieldsProps) {
  return (
    <>
      <Field label="URL">
        <input
          required
          className="input-base"
          placeholder="https://example.com/notify"
          value={config.url ?? ''}
          onChange={(e) => updateConfig('url', e.target.value)}
        />
      </Field>
      <Field label="Method">
        <select
          className="input-base"
          value={config.method ?? 'POST'}
          onChange={(e) => updateConfig('method', e.target.value)}
        >
          <option value="POST">POST</option>
          <option value="GET">GET</option>
        </select>
      </Field>
      <Field label="Headers (JSON)">
        <textarea
          rows={2}
          className="input-base font-mono text-xs"
          placeholder='{"Content-Type":"application/json"}'
          value={config.headers ?? ''}
          onChange={(e) => updateConfig('headers', e.target.value)}
        />
      </Field>
      <Field label="Body 模板 (支持 {{title}} {{message}})">
        <textarea
          rows={3}
          className="input-base font-mono text-xs"
          placeholder='{"title":"{{title}}","message":"{{message}}"}'
          value={config.body_template ?? ''}
          onChange={(e) => updateConfig('body_template', e.target.value)}
        />
      </Field>
    </>
  )
}

export function EmailFields({ config, updateConfig }: ProviderFieldsProps) {
  return (
    <>
      <Field label="SMTP 地址">
        <input
          required
          className="input-base"
          placeholder="smtp.gmail.com"
          value={config.smtp_host ?? ''}
          onChange={(e) => updateConfig('smtp_host', e.target.value)}
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="SMTP 端口">
          <input
            required
            className="input-base"
            placeholder="465"
            value={config.smtp_port ?? ''}
            onChange={(e) => updateConfig('smtp_port', e.target.value)}
          />
        </Field>
        <Field label="TLS">
          <select
            className="input-base"
            value={config.tls ?? 'true'}
            onChange={(e) => updateConfig('tls', e.target.value)}
          >
            <option value="true">启用</option>
            <option value="false">关闭</option>
          </select>
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="用户名">
          <input
            required
            className="input-base"
            value={config.username ?? ''}
            onChange={(e) => updateConfig('username', e.target.value)}
          />
        </Field>
        <Field label="密码">
          <input
            type="password"
            required
            className="input-base"
            value={config.password ?? ''}
            onChange={(e) => updateConfig('password', e.target.value)}
          />
        </Field>
      </div>
      <Field label="发件人">
        <input
          required
          className="input-base"
          placeholder="noreply@example.com"
          value={config.from ?? ''}
          onChange={(e) => updateConfig('from', e.target.value)}
        />
      </Field>
      <Field label="收件人 (多个用逗号分隔)">
        <input
          required
          className="input-base"
          placeholder="user@example.com"
          value={config.to ?? ''}
          onChange={(e) => updateConfig('to', e.target.value)}
        />
      </Field>
    </>
  )
}
