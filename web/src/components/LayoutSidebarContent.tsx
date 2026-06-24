import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { LogOut, Menu, X } from 'lucide-react'
import clsx from 'clsx'
import { LAYOUT_NAV_GROUPS, NAV_GROUP_PATHS, type LayoutNavItem } from './layoutNavigation'
import { SidebarGroup, SidebarLink } from './LayoutSidebarNav'

type LayoutSidebarContentProps = {
  isSidebarOpen: boolean
  isMobileDrawerOpen: boolean
  openGroups: Record<string, boolean>
  isAdmin: boolean
  username?: string
  can: (key: string) => boolean
  isRouteIn: (paths: string[]) => boolean
  onToggleGroup: (id: string) => void
  onToggleSidebar: () => void
  onCloseMobileDrawer: () => void
  onLogout: () => void
}

export function LayoutSidebarContent({
  isSidebarOpen,
  isMobileDrawerOpen,
  openGroups,
  isAdmin,
  username,
  can,
  isRouteIn,
  onToggleGroup,
  onToggleSidebar,
  onCloseMobileDrawer,
  onLogout,
}: LayoutSidebarContentProps) {
  const sidebarExpanded = isSidebarOpen || isMobileDrawerOpen
  const isItemVisible = (item: LayoutNavItem) =>
    (!item.adminOnly || isAdmin) && (!item.permission || can(item.permission))
  const visibleGroups = LAYOUT_NAV_GROUPS
    .filter((group) => !group.adminOnly || isAdmin)
    .map((group) => ({
      group,
      items: group.items.filter(isItemVisible),
    }))
    .filter(({ items }) => items.length > 0)

  return (
    <div className="flex h-full flex-col border-r border-[var(--app-border)] bg-[var(--app-panel)]">
      <div className="flex h-20 items-center justify-between border-b border-[var(--app-border)] px-6">
        <Link to="/" className="flex items-center gap-3">
          <img
            src="/brand/mediastationgo-logo.svg"
            alt="MediaStationGo"
            className="h-10 w-10 shrink-0 rounded-xl object-contain shadow-sm"
          />
          {sidebarExpanded && (
            <motion.span
              initial={{ opacity: 0, x: -10 }}
              animate={{ opacity: 1, x: 0 }}
              className="font-display text-lg font-extrabold tracking-tight text-[var(--app-text)]"
            >
              MediaStationGo
            </motion.span>
          )}
        </Link>

        <button
          onClick={onToggleSidebar}
          className="rounded-xl p-1.5 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-colors hidden lg:block"
        >
          <Menu size={18} />
        </button>

        <button
          onClick={onCloseMobileDrawer}
          className="rounded-xl p-1.5 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-colors block lg:hidden"
        >
          <X size={18} />
        </button>
      </div>

      <nav className="flex-1 overflow-y-auto px-4 py-5 space-y-2 scrollbar-hide">
        {visibleGroups.map(({ group, items }) => {
          const GroupIcon = group.icon
          return (
            <SidebarGroup
              key={group.id}
              id={group.id}
              icon={<GroupIcon size={18} />}
              label={group.label}
              collapsed={!sidebarExpanded}
              open={openGroups[group.id] ?? group.id === 'media'}
              active={isRouteIn(NAV_GROUP_PATHS[group.id])}
              onToggle={onToggleGroup}
            >
              {items.map((item) => {
                const ItemIcon = item.icon
                return (
                  <SidebarLink
                    key={item.to}
                    to={item.to}
                    icon={<ItemIcon size={16} />}
                    label={item.label}
                    end={item.end}
                    child
                  />
                )
              })}
            </SidebarGroup>
          )
        })}
      </nav>

      <div className="border-t border-[var(--app-border)] bg-[var(--app-panel-soft)] p-4">
        <button
          onClick={onLogout}
          className={clsx(
            'flex items-center gap-3.5 rounded-xl px-4 py-3 text-sm font-semibold transition-all duration-300 w-full group/logout',
            sidebarExpanded
              ? 'justify-start text-[var(--app-muted)] hover:bg-[var(--app-danger-soft)] hover:text-red-500'
              : 'justify-center text-[var(--app-muted)] hover:text-red-500',
          )}
          title={`安全登出 (${username ?? ''})`}
        >
          <LogOut size={18} className="transition-transform group-hover/logout:-translate-x-0.5" />
          {sidebarExpanded && <span>安全退出</span>}
        </button>
      </div>
    </div>
  )
}
