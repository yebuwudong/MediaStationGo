import { Plus, RefreshCw, Save, Trash2 } from 'lucide-react'

import type { Library, LibraryRoot } from '../types'
import type { RootDraft } from './adminLibraryPanelModel'
import { fallbackLibraryRoot } from './adminLibraryPanelModel'

type LibraryTableProps = {
  libs: Library[]
  newRootDraft: (libraryID: string) => RootDraft
  editableRootDraft: (libraryID: string, root: LibraryRoot) => RootDraft
  onNewRootChange: (libraryID: string, patch: Partial<RootDraft>) => void
  onEditableRootChange: (libraryID: string, root: LibraryRoot, patch: Partial<RootDraft>) => void
  onAddRoot: (libraryID: string) => void
  onSaveRoot: (libraryID: string, root: LibraryRoot) => void
  onScanRoot: (libraryID: string, root: LibraryRoot) => void
  onToggleRoot: (libraryID: string, root: LibraryRoot) => void
  onRemoveRoot: (library: Library, root: LibraryRoot) => void
  onScanLibrary: (library: Library) => void
  onRemoveLibrary: (library: Library) => void
}

export function AdminLibraryTable({ libs, ...actions }: LibraryTableProps) {
  return (
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
          {libs.map((library) => (
            <LibraryTableRow key={library.id} library={library} {...actions} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

type LibraryTableRowProps = Omit<LibraryTableProps, 'libs'> & {
  library: Library
}

function LibraryTableRow({ library, ...actions }: LibraryTableRowProps) {
  return (
    <tr className="border-t border-gray-200">
      <td className="py-2 text-ink-600">{library.name}</td>
      <td className="text-ink-100">
        <LibraryRootsCell library={library} {...actions} />
      </td>
      <td className="text-ink-100">{library.type}</td>
      <td className="space-x-2 py-2 text-right">
        <LibraryActionsCell library={library} {...actions} />
      </td>
    </tr>
  )
}

function LibraryRootsCell({ library, ...actions }: LibraryTableRowProps) {
  const roots = library.roots?.length ? library.roots : [fallbackLibraryRoot(library)]
  return (
    <div className="space-y-2">
      {roots.map((root) => (
        <ExistingRootEditor key={root.id || root.path} library={library} root={root} {...actions} />
      ))}
      <AddRootRow library={library} {...actions} />
    </div>
  )
}

type RootEditorProps = Omit<LibraryTableRowProps, 'library'> & {
  library: Library
  root: LibraryRoot
}

function ExistingRootEditor({ library, root, ...actions }: RootEditorProps) {
  const draft = actions.editableRootDraft(library.id, root)
  return (
    <div className="rounded border border-gray-200/70 p-2">
      <div className="grid gap-2 xl:grid-cols-[minmax(120px,0.8fr)_minmax(220px,2fr)_auto]">
        {root.id ? (
          <EditableRootFields library={library} root={root} draft={draft} {...actions} />
        ) : (
          <span className="min-w-0 break-all xl:col-span-2">{root.name ? `${root.name}：${root.path}` : root.path}</span>
        )}
        <RootActionButtons library={library} root={root} draft={draft} {...actions} />
      </div>
    </div>
  )
}

function EditableRootFields({ library, root, draft, onEditableRootChange }: RootEditorProps & { draft: RootDraft }) {
  return (
    <>
      <input
        className="input-base"
        placeholder="路径名称"
        value={draft.name ?? ''}
        onChange={(e) => onEditableRootChange(library.id, root, { name: e.target.value })}
      />
      <input
        className="input-base"
        placeholder="真实路径"
        value={draft.path}
        onChange={(e) => onEditableRootChange(library.id, root, { path: e.target.value })}
      />
    </>
  )
}

function RootActionButtons({ library, root, draft, ...actions }: RootEditorProps & { draft: RootDraft }) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {root.id && (
        <button
          className="rounded border border-primary-400/40 p-1 text-brand-500 hover:bg-primary-400/10"
          title="保存路径"
          onClick={() => actions.onSaveRoot(library.id, root)}
        >
          <Save size={14} />
        </button>
      )}
      <button
        className="rounded border border-primary-400/40 p-1 text-brand-500 hover:bg-primary-400/10"
        title="扫描路径"
        onClick={() => actions.onScanRoot(library.id, root)}
      >
        <RefreshCw size={14} />
      </button>
      {root.id && (
        <button className="rounded border border-gray-300 px-2 py-1 text-xs" onClick={() => actions.onToggleRoot(library.id, root)}>
          {draft.enabled ? '启用' : '禁用'}
        </button>
      )}
      {root.id && (
        <button
          className="rounded border border-red-400/40 p-1 text-red-400 hover:bg-red-400/10"
          title="删除路径"
          onClick={() => actions.onRemoveRoot(library, root)}
        >
          <Trash2 size={14} />
        </button>
      )}
    </div>
  )
}

function AddRootRow({ library, ...actions }: LibraryTableRowProps) {
  const draft = actions.newRootDraft(library.id)
  return (
    <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]">
      <input
        className="input-base"
        placeholder="路径名称"
        value={draft.name ?? ''}
        onChange={(e) => actions.onNewRootChange(library.id, { name: e.target.value })}
      />
      <input
        className="input-base"
        placeholder="新增路径"
        value={draft.path}
        onChange={(e) => actions.onNewRootChange(library.id, { path: e.target.value })}
      />
      <button className="rounded-lg border px-3 py-2 text-sm" onClick={() => actions.onAddRoot(library.id)}>
        <Plus size={14} />
      </button>
    </div>
  )
}

function LibraryActionsCell({ library, onScanLibrary, onRemoveLibrary }: LibraryTableRowProps) {
  return (
    <>
      <button
        className="rounded-lg border border-primary-400/40 px-2 py-1 text-xs text-brand-500 hover:bg-primary-400/10"
        onClick={() => onScanLibrary(library)}
      >
        扫描
      </button>
      <button
        className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
        onClick={() => onRemoveLibrary(library)}
      >
        <Trash2 size={12} />
      </button>
    </>
  )
}
