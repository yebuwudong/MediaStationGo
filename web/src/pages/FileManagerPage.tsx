import { useCallback, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'

import { filesAPI, type FileEntry, type FileListing } from '../api/files'
import { libraryAPI } from '../api/library'
import { toolsAPI } from '../api/tools'
import { confirmAction } from '../components/confirmAction'
import type { Library } from '../types'
import { settingOn } from './autoOrganizeModel'
import { AutoOrganizeSettingsPanel } from './AutoOrganizeSettingsPanel'
import { FileBrowserRoots } from './FileBrowserRoots'
import { FileEntriesTable } from './FileEntriesTable'
import { FileManagerToolbar } from './FileManagerToolbar'
import { FileOperationsPanel } from './FileOperationsPanel'
import { ManualOrganizePanel } from './ManualOrganizePanel'
import { useAutoOrganizeSettings } from './useAutoOrganizeSettings'
import { useFileOperations } from './useFileOperations'
import {
  formatScanSummary,
  formatScrapeSummary,
  isCloudLibraryPath,
  summarizeOrganizeResults,
  type OrganizePreviewItem,
} from './fileManagerModel'

// FileManagerPage provides a focused local storage view:
// browse allowed roots, optionally recurse, and perform safe local operations.
export function FileManagerPage() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [path, setPath] = useState('')
  const [data, setData] = useState<FileListing | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [recursive, setRecursive] = useState(false)
  const [organizeLibraryID, setOrganizeLibraryID] = useState('')
  const [organizeDestPath, setOrganizeDestPath] = useState('')
  const [organizeTransferMode, setOrganizeTransferMode] = useState('hardlink')
  const [organizeMediaType, setOrganizeMediaType] = useState('auto')
  const [scanAfter, setScanAfter] = useState(true)
  const [scrapeAfter, setScrapeAfter] = useState(true)
  const [organizeBusy, setOrganizeBusy] = useState('')
  const [previewItems, setPreviewItems] = useState<OrganizePreviewItem[]>([])
  const autoOrganize = useAutoOrganizeSettings({ onScrapeAfterChange: setScrapeAfter })

  const currentDir = useMemo(() => {
    if (data?.path) return data.path
    return path
  }, [data?.path, path])
  const localLibraries = useMemo(
    () => libraries.filter((library) => !isCloudLibraryPath(library.path)),
    [libraries],
  )
  const autoMoveKeepsSeeding = autoOrganize.moveKeepsSeeding
  const manualMoveKeepsSeeding = organizeTransferMode === 'move' && settingOn(autoOrganize.config.keepSeeding)

  const refresh = useCallback(() => {
    setLoading(true)
    setError('')
    filesAPI
      .list(path, recursive ? 5000 : 1000, recursive)
      .then(setData)
      .catch((err: unknown) => {
        const msg =
          (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          '加载失败'
        setError(msg)
      })
      .finally(() => setLoading(false))
  }, [path, recursive])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    libraryAPI.list({ includeHidden: true }).then(setLibraries).catch(() => undefined)
  }, [])

  const fileOperations = useFileOperations({ currentDir, path, refresh })

  useEffect(() => {
    if (!organizeLibraryID) return
    const lib = localLibraries.find((item) => item.id === organizeLibraryID)
    if (!lib) {
      setOrganizeLibraryID('')
      return
    }
    setOrganizeDestPath(lib.path)
    setOrganizeMediaType(lib.type || 'auto')
  }, [localLibraries, organizeLibraryID])

  const enter = (e: FileEntry) => {
    if (e.is_dir) setPath(e.path)
  }

  const organizeSources = fileOperations.selectedPaths.length > 0 ? fileOperations.selectedPaths : [fileOperations.selected?.path || currentDir].filter(Boolean)
  const organizeSource = organizeSources.length === 1 ? organizeSources[0] : `${organizeSources.length} 个已选项目`
  const organizeReady = organizeSources.length > 0 && Boolean(organizeDestPath.trim())

  const runManualOrganize = async (dryRun: boolean) => {
    if (!organizeReady) {
      toast.error('请选择来源文件/文件夹，并设置目标媒体库或目的路径')
      return
    }
    if (!dryRun) {
      const ok = await confirmAction({
        title: '确认整理入库',
        message: `来源：${organizeSources.join('\n')}\n目标：${organizeDestPath}\n方式：${organizeTransferMode}${scanAfter ? '\n整理完成后会扫描入库。' : ''}${scanAfter && scrapeAfter ? '\n扫描后会自动刮削。' : ''}`,
        confirmText: '开始整理',
      })
      if (!ok) return
    }
    setOrganizeBusy(dryRun ? 'preview' : 'run')
    try {
      const results = []
      for (const sourcePath of organizeSources) {
        results.push(await toolsAPI.organizeDirectory({
          source_path: sourcePath,
          dest_path: organizeDestPath.trim(),
          transfer_mode: organizeTransferMode,
          media_type: organizeMediaType === 'auto' ? undefined : organizeMediaType,
          scan_after: !dryRun && scanAfter,
          scrape_after: !dryRun && scanAfter && scrapeAfter,
          library_id: !dryRun && scanAfter && organizeLibraryID ? organizeLibraryID : undefined,
          dry_run: dryRun,
        }))
      }
      const summary = summarizeOrganizeResults(results)
      setPreviewItems(summary.preview)
      if (summary.total === 0) {
        toast(`未发现可整理视频：${organizeSource}`, {
          icon: '!',
          duration: 6000,
        })
        return
      }
      if (dryRun) {
        toast.success(`预览完成：新增 ${summary.organized} · 替换 ${summary.replaced} · 纠偏 ${summary.reclassified} · 跳过 ${summary.skipped}`)
        return
      }
      const scanText = scanAfter ? formatScanSummary(summary.scans) : ''
      const scrapeText = scanAfter && scrapeAfter ? formatScrapeSummary(summary.scrapes) : ''
      toast.success(`整理完成：新增 ${summary.organized} · 替换 ${summary.replaced} · 纠偏 ${summary.reclassified} · 跳过 ${summary.skipped}${scanText}${scrapeText}`)
      fileOperations.setSelectedPaths([])
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '整理失败')
    } finally {
      setOrganizeBusy('')
    }
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="font-display text-3xl font-bold text-ink-600">文件管理</h1>
        <p className="text-sm text-ink-50">
          在下载目录中选择文件夹或视频，直接设置目标并整理入库。
        </p>
      </header>

      <FileManagerToolbar
        currentPath={data?.path}
        parentPath={data?.parent}
        recursive={recursive}
        onRoot={() => setPath('')}
        onParent={setPath}
        onRefresh={refresh}
        onRecursiveChange={setRecursive}
      />

      <AutoOrganizeSettingsPanel
        config={autoOrganize.config}
        currentDir={currentDir}
        activeTab={autoOrganize.activeTab}
        dirty={autoOrganize.dirty}
        loading={autoOrganize.loading}
        saving={autoOrganize.saving}
        running={autoOrganize.running}
        moveKeepsSeeding={autoMoveKeepsSeeding}
        onRefresh={autoOrganize.refresh}
        onSave={() => void autoOrganize.save()}
        onRunNow={autoOrganize.runNow}
        onTabChange={autoOrganize.setActiveTab}
        onConfigChange={autoOrganize.changeConfig}
      />

      {data?.path && (
        <ManualOrganizePanel
          organizeSource={organizeSource}
          selectedCount={fileOperations.selectedPaths.length}
          localLibraries={localLibraries}
          organizeLibraryID={organizeLibraryID}
          organizeDestPath={organizeDestPath}
          organizeMediaType={organizeMediaType}
          organizeTransferMode={organizeTransferMode}
          manualMoveKeepsSeeding={manualMoveKeepsSeeding}
          scanAfter={scanAfter}
          scrapeAfter={scrapeAfter}
          organizeReady={organizeReady}
          organizeBusy={organizeBusy}
          previewItems={previewItems}
          onClearSelected={() => fileOperations.setSelectedPaths([])}
          onLibraryChange={setOrganizeLibraryID}
          onDestPathChange={setOrganizeDestPath}
          onMediaTypeChange={setOrganizeMediaType}
          onTransferModeChange={setOrganizeTransferMode}
          onScanAfterChange={setScanAfter}
          onScrapeAfterChange={setScrapeAfter}
          onPreview={() => runManualOrganize(true)}
          onRun={() => runManualOrganize(false)}
        />
      )}

      {data?.path && (
        <FileOperationsPanel
          selected={fileOperations.selected}
          folderName={fileOperations.folderName}
          renameTo={fileOperations.renameTo}
          destPath={fileOperations.destPath}
          transferMode={fileOperations.transferMode}
          busy={fileOperations.busy}
          onFolderNameChange={fileOperations.setFolderName}
          onRenameToChange={fileOperations.setRenameTo}
          onDestPathChange={fileOperations.setDestPath}
          onTransferModeChange={fileOperations.setTransferMode}
          onCreateFolder={fileOperations.createFolder}
          onRenameSelected={fileOperations.renameSelected}
          onDeleteSelected={fileOperations.deleteSelected}
          onTransferSelected={fileOperations.transferSelected}
        />
      )}

      {loading && <p className="text-sand-500">加载中…</p>}
      {error && <div className="glass-panel !border-red-400/40 text-red-400">{error}</div>}

      {!loading && data && !data.path && data.roots && (
        <FileBrowserRoots roots={data.roots} onOpen={setPath} />
      )}

      {!loading && data?.entries && data.entries.length > 0 && (
        <FileEntriesTable
          basePath={data.path || ''}
          entries={data.entries}
          recursive={recursive}
          selectedPath={fileOperations.selected?.path}
          selectedPaths={fileOperations.selectedPaths}
          onSelectAll={(checked) => fileOperations.setSelectedPaths(checked ? data.entries?.map((entry) => entry.path) ?? [] : [])}
          onToggleSelectedPath={fileOperations.toggleSelectedPath}
          onEnter={enter}
          onChoose={fileOperations.choose}
        />
      )}

      {!loading && data?.entries && data.entries.length === 0 && <p className="text-ink-50">空目录。</p>}
    </div>
  )
}
