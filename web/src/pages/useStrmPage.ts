import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI, mediaAPI } from '../api/library'
import { strmAPI, type GenerateSTRMResult } from '../api/strm'
import { confirmAction } from '../components/confirmAction'
import type { Library, Media } from '../types'
import {
  inferSTRMOutputRoot,
  playbackStatusText,
  preferredSTRMBaseURL,
  suggestedSTRMOutputDir,
  type CloudPlaybackMode,
} from './strmPageModel'

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

function isHTTPURL(value: string): boolean {
  return /^https?:\/\//i.test(value)
}

export function useStrmPage() {
  const [libraries, setLibraries] = useState<Library[]>([])

  const [generateLibraryID, setGenerateLibraryID] = useState('')
  const [baseURL, setBaseURL] = useState('')
  const [outputDir, setOutputDir] = useState('')
  const [outputRoot, setOutputRoot] = useState('')
  const [outputScope, setOutputScope] = useState('')
  const [outputDirTouched, setOutputDirTouched] = useState(false)
  const [settingsLoaded, setSettingsLoaded] = useState(false)
  const [cloudPlaybackMode, setCloudPlaybackMode] = useState<CloudPlaybackMode>('redirect_proxy')
  const [strmPlaybackEnabled, setStrmPlaybackEnabled] = useState(false)
  const [redirectProxyEnabled, setRedirectProxyEnabled] = useState(true)
  const [autoGenerate, setAutoGenerate] = useState(false)
  const [savingSettings, setSavingSettings] = useState(false)
  const [overwrite, setOverwrite] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [generateResult, setGenerateResult] = useState<GenerateSTRMResult | null>(null)

  const [libraryID, setLibraryID] = useState('')
  const [title, setTitle] = useState('')
  const [url, setURL] = useState('')
  const [importing, setImporting] = useState(false)

  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState<Media[]>([])
  const [drafts, setDrafts] = useState<Record<string, string>>({})

  useEffect(() => {
    libraryAPI.list().then(setLibraries).catch(() => undefined)
    adminAPI
      .listSettings()
      .then((rows) => {
        const settings = Object.fromEntries(rows.map((row) => [row.key, row.value]))
        const savedOutputDir = settings['strm.output_dir'] || ''
        const mode = settings['cloud.playback_mode']
        const nextMode =
          mode === 'strm' || mode === 'redirect_proxy'
            ? mode
            : settings['strm.enabled'] === 'true'
              ? 'strm'
              : 'redirect_proxy'

        setBaseURL(preferredSTRMBaseURL(settings['strm.base_url'] || settings['app.server_url'] || ''))
        setOutputDir(savedOutputDir)
        setOutputRoot(savedOutputDir)
        setOutputScope(settings['strm.output_scope'] || '')
        setOutputDirTouched(false)
        setCloudPlaybackMode(nextMode)
        setStrmPlaybackEnabled(
          settings['cloud.playback_strm_enabled'] != null
            ? settings['cloud.playback_strm_enabled'] === 'true'
            : settings['strm.enabled'] === 'true' || nextMode === 'strm',
        )
        setRedirectProxyEnabled(settings['cloud.playback_redirect_proxy_enabled'] !== 'false')
        setAutoGenerate(settings['strm.auto_generate_enabled'] === 'true')
        setSettingsLoaded(true)
      })
      .catch(() => setSettingsLoaded(true))
  }, [])

  useEffect(() => {
    if (!libraryID && libraries[0]) setLibraryID(libraries[0].id)
    if (!generateLibraryID && libraries[0]) setGenerateLibraryID(libraries[0].id)
  }, [libraries, libraryID, generateLibraryID])

  useEffect(() => {
    if (!settingsLoaded || outputDirTouched || !generateLibraryID) return
    const root = outputScope === 'library' ? inferSTRMOutputRoot(outputRoot, libraries) : outputRoot
    if (generateLibraryID === '*') {
      setOutputDir(root)
      return
    }
    const library = libraries.find((item) => item.id === generateLibraryID)
    if (library) setOutputDir(suggestedSTRMOutputDir(root, library))
  }, [settingsLoaded, outputDirTouched, outputRoot, outputScope, generateLibraryID, libraries])

  const onOutputDirChange = (value: string) => {
    setOutputDir(value)
    setOutputRoot(value)
    setOutputScope('')
    setOutputDirTouched(true)
  }

  const onGenerate = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedBaseURL = baseURL.trim()
    if (!generateLibraryID || !trimmedBaseURL) return
    if (!isHTTPURL(trimmedBaseURL)) {
      toast.error('域名必须以 http:// 或 https:// 开头')
      return
    }

    setGenerating(true)
    try {
      const result = await strmAPI.generate({
        library_id: generateLibraryID,
        base_url: trimmedBaseURL.replace(/\/+$/, ''),
        output_dir: outputDir.trim(),
        overwrite,
        enabled: autoGenerate,
        include_local: true,
      })
      const nextOutputDir = result.output_dir || outputDir
      setGenerateResult(result)
      setOutputDir(nextOutputDir)
      setOutputRoot(inferSTRMOutputRoot(nextOutputDir, libraries))
      setOutputScope(generateLibraryID === '*' ? 'all' : 'library')
      setOutputDirTouched(false)
      toast.success(`生成完成：新增 ${result.generated} · 更新 ${result.updated} · 跳过 ${result.skipped}`)
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '生成失败'))
    } finally {
      setGenerating(false)
    }
  }

  const saveSTRMSettings = async () => {
    setSavingSettings(true)
    try {
      const effectiveMode: CloudPlaybackMode =
        strmPlaybackEnabled && !redirectProxyEnabled
          ? 'strm'
          : !strmPlaybackEnabled && redirectProxyEnabled
            ? 'redirect_proxy'
            : cloudPlaybackMode
      await Promise.all([
        adminAPI.updateSetting('cloud.playback_mode', effectiveMode),
        adminAPI.updateSetting('cloud.playback_strm_enabled', String(strmPlaybackEnabled)),
        adminAPI.updateSetting('cloud.playback_redirect_proxy_enabled', String(redirectProxyEnabled)),
        adminAPI.updateSetting('strm.enabled', String(strmPlaybackEnabled)),
        adminAPI.updateSetting('strm.auto_generate_enabled', String(autoGenerate)),
      ])
      setCloudPlaybackMode(effectiveMode)
      toast.success(
        strmPlaybackEnabled || redirectProxyEnabled
          ? `播放设置已保存：优先 ${effectiveMode === 'strm' ? 'STRMURL' : '302/反代'}`
          : '播放设置已保存：云盘第三方播放已关闭',
      )
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '保存 STRM 开关失败'))
    } finally {
      setSavingSettings(false)
    }
  }

  const onImport = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedTitle = title.trim()
    const trimmedURL = url.trim()
    if (!libraryID || !trimmedTitle || !trimmedURL) return
    if (!isHTTPURL(trimmedURL)) {
      toast.error('URL 必须以 http:// 或 https:// 开头')
      return
    }

    setImporting(true)
    try {
      await strmAPI.importURL(libraryID, trimmedTitle, trimmedURL)
      toast.success(`已导入「${trimmedTitle}」`)
      setTitle('')
      setURL('')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '导入失败'))
    } finally {
      setImporting(false)
    }
  }

  const doSearch = async (event?: FormEvent) => {
    event?.preventDefault()
    const trimmedQuery = query.trim()
    if (!trimmedQuery) return
    setSearching(true)
    try {
      const result = await mediaAPI.search(trimmedQuery, 30)
      setResults(result.items ?? [])
    } catch {
      toast.error('搜索失败')
    } finally {
      setSearching(false)
    }
  }

  const onAttach = async (media: Media) => {
    const next = (drafts[media.id] ?? '').trim()
    if (!next) return
    if (!isHTTPURL(next)) {
      toast.error('URL 必须以 http:// 或 https:// 开头')
      return
    }

    try {
      await strmAPI.set(media.id, next)
      toast.success('已设置 STRM URL')
      setResults((rows) =>
        rows.map((item) => (item.id === media.id ? ({ ...item, container: 'strm' } as Media) : item)),
      )
      setDrafts((current) => ({ ...current, [media.id]: '' }))
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '设置失败'))
    }
  }

  const onDetach = async (media: Media) => {
    const ok = await confirmAction({
      title: '清除 STRM URL',
      message: `清除「${media.title}」的 STRM URL?`,
      confirmText: '清除',
    })
    if (!ok) return

    try {
      await strmAPI.clear(media.id)
      toast.success('已清除')
      setResults((rows) =>
        rows.map((item) =>
          item.id === media.id
            ? ({ ...item, container: item.container === 'strm' ? '' : item.container } as Media)
            : item,
        ),
      )
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '清除失败'))
    }
  }

  return {
    attach: {
      doSearch,
      drafts,
      onAttach,
      onDetach,
      query,
      results,
      searching,
      setDrafts,
      setQuery,
    },
    generate: {
      autoGenerate,
      baseURL,
      cloudPlaybackMode,
      generateLibraryID,
      generateResult,
      generating,
      onGenerate,
      outputDir,
      overwrite,
      playbackStatus: playbackStatusText(strmPlaybackEnabled, redirectProxyEnabled, cloudPlaybackMode),
      redirectProxyEnabled,
      saveSTRMSettings,
      savingSettings,
      setAutoGenerate,
      setBaseURL,
      setCloudPlaybackMode,
      setGenerateLibraryID,
      setOutputDir: onOutputDirChange,
      setOverwrite,
      setRedirectProxyEnabled,
      setStrmPlaybackEnabled,
      strmPlaybackEnabled,
    },
    importForm: {
      importing,
      libraryID,
      onImport,
      setLibraryID,
      setTitle,
      setURL,
      title,
      url,
    },
    libraries,
  }
}
