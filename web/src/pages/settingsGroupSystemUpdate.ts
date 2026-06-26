import type { SettingGroup } from './settingsGroupTypes'

export const systemUpdateSettingsGroup: SettingGroup = {
  key: 'system-update',
  label: '系统更新',
  description: 'Docker 部署可在这里检查并拉取最新版镜像。',
  items: [
    {
      key: 'system.update.image',
      label: '应用镜像',
      type: 'text',
      defaultValue: 'ghcr.io/shukebta/mediastation-go:latest',
      placeholder: 'ghcr.io/shukebta/mediastation-go:latest',
      hint: '用于检查远端摘要；保持 latest 即可跟随主分支镜像。',
    },
    {
      key: 'system.update.watchtower_image',
      label: 'Watchtower 镜像',
      type: 'text',
      defaultValue: 'containrrr/watchtower:latest',
      placeholder: 'containrrr/watchtower:latest',
      hint: '默认使用一次性 Watchtower 更新当前容器。',
    },
    {
      key: 'system.update.command',
      label: '自定义更新命令',
      type: 'textarea',
      placeholder:
        'docker run --rm -v /var/run/docker.sock:/var/run/docker.sock {{watchtower_image}} --run-once --cleanup {{container}}',
      hint: '留空时使用默认命令。支持 {{image}}、{{watchtower_image}}、{{container}}、{{container_id}}、{{container_name}}。',
    },
  ],
}
