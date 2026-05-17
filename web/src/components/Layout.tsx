import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router-dom'
import {
  Activity,
  Bell,
  Cast,
  Clock,
  CloudDownload,
  Compass,
  Copy,
  Film,
  FolderTree,
  Globe,
  HardDrive,
  Heart,
  Home,
  GalleryHorizontalEnd,
  Link2,
  ListChecks,
  ListMusic,
  LogOut,
  MessageSquare,
  Rss,
  Search,
  Server,
  Settings,
  Sliders,
  Sparkles,
  Cloud,
  Trash2,
  UserCog,
  Wrench,
  Library as LibraryIcon,
  User as UserIcon,
} from 'lucide-react'
import clsx from 'clsx'

import { AppFooter } from './AppFooter'
import { libraryAPI } from '../api/library'
import { useAuthStore } from '../stores/auth'
import type { Library } from '../types'

export function Layout() {
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const [libraries, setLibraries] = useState<Library[]>([])

  useEffect(() => {
    libraryAPI.list().then(setLibraries).catch(() => undefined)
  }, [])

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex min-h-full">
      {/* ── Sidebar group — collapsed 64px, expands to 240px on hover ── */}
      <div className="group relative z-30 shrink-0">
        <aside
          className={clsx(
            'flex h-full flex-col border-r border-cream-900/15 bg-surface-600',
            'w-16 transition-all duration-300 ease-out',
            'group-hover:w-60',
          )}
        >
          {/* Logo */}
          <Link to="/" className="flex items-center gap-3 px-4 py-5">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-brand-500/15">
              <Film className="h-4 w-4 text-brand-400" />
            </div>
            <span
              className={clsx(
                'font-display text-lg font-bold tracking-tight text-cream-100',
                'overflow-hidden whitespace-nowrap opacity-0 transition-opacity duration-200',
                'group-hover:opacity-100',
              )}
            >
              MediaStation
            </span>
          </Link>

          {/* Navigation */}
          <nav className="flex-1 space-y-0.5 overflow-y-auto px-2 pb-4">
            <SidebarLink to="/" icon={<Home size={18} />} label="首页" end />

            <SectionHeader label="浏览" />
            <SidebarLink to="/discover" icon={<Compass size={18} />} label="发现" />
            <SidebarLink to="/search" icon={<Search size={18} />} label="搜索" />
            <SidebarLink to="/favourites" icon={<Heart size={18} />} label="收藏" />
            <SidebarLink to="/playlists" icon={<ListMusic size={18} />} label="播放列表" />
            <SidebarLink to="/history" icon={<Clock size={18} />} label="观看历史" />
            <SidebarLink to="/poster-wall" icon={<GalleryHorizontalEnd size={18} />} label="海报墙" />
            <SidebarLink to="/ai" icon={<Sparkles size={18} />} label="AI 助手" />

            <SectionHeader label="媒体库" />
            {libraries.length === 0 && (
              <div className="hidden px-3 py-1.5 text-xs text-cream-500/50 group-hover:block">
                暂无媒体库
              </div>
            )}
            {libraries.map((lib) => (
              <SidebarLink
                key={lib.id}
                to={`/library/${lib.id}`}
                icon={<LibraryIcon size={18} />}
                label={lib.name}
              />
            ))}

            <SectionHeader label="自动化" />
            <SidebarLink to="/downloads" icon={<CloudDownload size={18} />} label="下载" />
            <SidebarLink to="/subscriptions" icon={<Rss size={18} />} label="RSS 订阅" />
            <SidebarLink to="/dlna" icon={<Cast size={18} />} label="DLNA 投屏" />
            <SidebarLink to="/site-search" icon={<Search size={18} />} label="站点搜索" />

            <SectionHeader label="账号" />
            <SidebarLink to="/profile" icon={<UserIcon size={18} />} label="个人资料" />
            <SidebarLink to="/play-profiles" icon={<UserCog size={18} />} label="观影 Profile" />

            {user?.role === 'admin' && (
              <>
                <SectionHeader label="管理" />
                <SidebarLink to="/admin" icon={<Settings size={18} />} label="管理后台" />
                <SidebarLink to="/tasks" icon={<ListChecks size={18} />} label="实时任务" />
                <SidebarLink to="/stats" icon={<Activity size={18} />} label="运行状态" />
                <SidebarLink to="/sites" icon={<Globe size={18} />} label="站点管理" />
                <SidebarLink to="/notify-channels" icon={<Bell size={18} />} label="通知渠道" />
                <SidebarLink to="/download-clients" icon={<Server size={18} />} label="下载器" />
                <SidebarLink to="/scheduler" icon={<Clock size={18} />} label="定时任务" />
                <SidebarLink to="/storage" icon={<HardDrive size={18} />} label="存储" />
                <SidebarLink to="/storage-config" icon={<Cloud size={18} />} label="外部存储" />
                <SidebarLink to="/files" icon={<FolderTree size={18} />} label="文件浏览" />
                <SidebarLink to="/duplicates" icon={<Copy size={18} />} label="重复文件" />
                <SidebarLink to="/strm" icon={<Link2 size={18} />} label="STRM 管理" />
                <SidebarLink to="/tools" icon={<Wrench size={18} />} label="运维工具" />
                <SidebarLink to="/assistant" icon={<MessageSquare size={18} />} label="AI 对话" />
                <SidebarLink to="/settings" icon={<Sliders size={18} />} label="系统设置" />
                <SidebarLink to="/recycle" icon={<Trash2 size={18} />} label="回收站" />
              </>
            )}
          </nav>

          {/* Bottom: logout */}
          <div className="border-t border-cream-900/15 px-3 py-3">
            <button
              onClick={handleLogout}
              className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-sm text-cream-400 transition hover:bg-cream-900/10 hover:text-cream-200"
              title={`退出登录 (${user?.username})`}
            >
              <LogOut size={16} />
              <span className="overflow-hidden whitespace-nowrap opacity-0 transition-opacity duration-200 group-hover:opacity-100">
                退出
              </span>
            </button>
          </div>
        </aside>
      </div>

      {/* ── Main content ── */}
      <main className="flex flex-1 flex-col overflow-y-auto">
        <div className="flex-1 px-6 py-8">
          <Outlet />
        </div>
        <AppFooter className="py-4" />
      </main>
    </div>
  )
}

/** Section divider — visible only when sidebar is expanded */
function SectionHeader({ label }: { label: string }) {
  return (
    <div className="mt-4 mb-1 overflow-hidden whitespace-nowrap px-3 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
      <span className="text-[10px] font-medium uppercase tracking-[0.15em] text-cream-500/50">
        {label}
      </span>
    </div>
  )
}

/** Single nav link — icon always visible, label fades in on expand */
function SidebarLink({
  to,
  icon,
  label,
  end,
}: {
  to: string
  icon: React.ReactNode
  label: string
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        clsx(
          'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
          isActive
            ? 'bg-brand-500/10 text-brand-400'
            : 'text-cream-400 hover:bg-cream-900/10 hover:text-cream-200',
        )
      }
    >
      <span className="flex shrink-0 items-center justify-center w-5 h-5">{icon}</span>
      <span
        className={clsx(
          'overflow-hidden whitespace-nowrap truncate',
          'opacity-0 transition-opacity duration-200',
          'group-hover:opacity-100',
        )}
      >
        {label}
      </span>
    </NavLink>
  )
}
