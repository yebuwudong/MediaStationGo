import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Menu, MessageSquareText, Search, Sparkles } from 'lucide-react'
import clsx from 'clsx'
import { AppFooter } from './AppFooter'
import { useAuthStore } from '../stores/auth'
import { usePlayProfileStore } from '../stores/playProfile'
import { LayoutSearchBox } from './LayoutSearchBox'
import { LayoutSidebarContent } from './LayoutSidebarContent'
import { LayoutThemeToggle } from './LayoutThemeToggle'
import { LayoutUserMenu } from './LayoutUserMenu'
import { useLayoutSearch } from './useLayoutSearch'
import { useLayoutPermissions } from './useLayoutPermissions'
import { useLayoutProfiles } from './useLayoutProfiles'
import { useLayoutSidebar } from './useLayoutSidebar'
import { useThemeMode } from './useThemeMode'

export function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const activeProfileId = usePlayProfileStore((s) => s.activeProfileId)
  const setActiveProfile = usePlayProfileStore((s) => s.setActiveProfile)
  const theme = useThemeMode()
  const search = useLayoutSearch({
    pathname: location.pathname,
    locationSearch: location.search,
    navigate,
  })
  const { can, isAdmin } = useLayoutPermissions(user)
  const {
    isMobileDrawerOpen,
    isRouteIn,
    isSidebarOpen,
    openGroups,
    setIsMobileDrawerOpen,
    setIsSidebarOpen,
    toggleGroup,
  } = useLayoutSidebar(location.pathname)
  const {
    activeProfile,
    isProfileOpen,
    profiles,
    setIsProfileOpen,
    switchProfile,
    useDefaultProfile,
  } = useLayoutProfiles({ activeProfileId, setActiveProfile, user })

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const sidebarContent = (
    <LayoutSidebarContent
      isSidebarOpen={isSidebarOpen}
      isMobileDrawerOpen={isMobileDrawerOpen}
      openGroups={openGroups}
      isAdmin={isAdmin}
      username={user?.username}
      can={can}
      isRouteIn={isRouteIn}
      onToggleGroup={toggleGroup}
      onToggleSidebar={() => setIsSidebarOpen((current) => !current)}
      onCloseMobileDrawer={() => setIsMobileDrawerOpen(false)}
      onLogout={handleLogout}
    />
  )

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[var(--app-bg)] text-[var(--app-text)] font-body select-none">
      
      {/* 1. Desktop Persistent Sidebar */}
      <aside className={clsx(
        "hidden lg:flex flex-col h-full shrink-0 transition-all duration-300 ease-out",
        isSidebarOpen ? "w-64" : "w-20"
      )}>
        {sidebarContent}
      </aside>

      {/* 2. Mobile & Tablet Sidebar Drawer (Overlay) */}
      <AnimatePresence>
        {isMobileDrawerOpen && (
          <div className="fixed inset-0 z-50 flex lg:hidden">
            {/* Backdrop sheet */}
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={() => setIsMobileDrawerOpen(false)}
              className="fixed inset-0 bg-black/15 backdrop-blur-sm"
            />
            {/* Drawer body container */}
            <motion.div
              initial={{ x: '-100%' }}
              animate={{ x: 0 }}
              exit={{ x: '-100%' }}
              transition={{ type: 'spring', damping: 25, stiffness: 220 }}
              className="relative flex w-64 max-w-xs flex-col h-full z-10 shadow-xl"
            >
              {sidebarContent}
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* 3. Main Workspace Container */}
      <div className="flex flex-1 flex-col min-w-0 overflow-hidden">
        
        {/* Top Header Bar */}
        <header className="flex h-20 shrink-0 items-center justify-between border-b border-[var(--app-border)] bg-[var(--app-header-bg)] px-4 backdrop-blur-md z-30 md:px-8">
          
          {/* Header Left Area: Hamburger and Search */}
          <div className="flex items-center gap-3 flex-1 max-w-lg md:gap-4">
            {/* Hamburger button shown on Mobile & Tablet */}
            <button 
              onClick={() => setIsMobileDrawerOpen(true)}
              className="rounded-xl border border-[var(--app-border)] p-2.5 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-colors lg:hidden"
            >
              <Menu size={18} />
            </button>

            <LayoutSearchBox
              query={search.query}
              focused={search.focused}
              loading={search.loading}
              error={search.error}
              cards={search.cards}
              total={search.total}
              onQueryChange={search.setQuery}
              onFocusedChange={search.setFocused}
              onSubmit={search.submit}
            />
          </div>

          {/* Header Right Area: Actions, Notifications, Profile Dropdown */}
          <div className="flex shrink-0 items-center gap-2 sm:gap-3 md:gap-4">
            {/* Quick search button shown ONLY on Mobile screens */}
            <Link 
              to="/search" 
              className="rounded-xl border border-[var(--app-border)] p-2.5 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-colors sm:hidden"
            >
              <Search size={18} />
            </Link>

            {/* Quick Discover shortcut button */}
            {can('can_view_discover') && (
              <Link
                to="/discover"
                className="hidden md:flex items-center gap-2 rounded-xl border border-[var(--app-border)] px-4 py-2.5 text-xs font-bold text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-all"
              >
                <Sparkles size={14} className="text-brand-500" />
                <span>发现新片</span>
              </Link>
            )}

            {/* Notification channel settings */}
            {isAdmin && (
              <Link
                to="/notify-channels"
                title="通知配置"
                aria-label="打开通知配置"
                className="relative rounded-xl border border-[var(--app-border)] p-2.5 text-[var(--app-muted)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] transition-all"
              >
                <MessageSquareText size={18} />
              </Link>
            )}

            {/* Horizontal divider lines */}
            <LayoutThemeToggle mode={theme.mode} onChange={theme.setMode} />

            <span className="hidden h-6 w-px bg-[var(--app-border)] sm:block" />

            <LayoutUserMenu
              user={user}
              isOpen={isProfileOpen}
              profiles={profiles}
              activeProfileId={activeProfileId}
              activeProfile={activeProfile}
              onToggle={() => setIsProfileOpen((open) => !open)}
              onClose={() => setIsProfileOpen(false)}
              onUseDefaultProfile={useDefaultProfile}
              onSwitchProfile={switchProfile}
              onLogout={() => {
                setIsProfileOpen(false)
                logout()
                navigate('/login')
              }}
            />
          </div>
        </header>

        {/* ── Dynamic Content Workspace Scroll Area ── */}
        <main className="flex-1 overflow-y-auto px-4 py-6 md:px-8 md:py-10">
          <div className="max-w-7xl mx-auto">
            <AnimatePresence mode="wait">
              <motion.div
                key={location.pathname}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -6 }}
                transition={{ duration: 0.25, ease: 'easeOut' }}
              >
                <Outlet />
              </motion.div>
            </AnimatePresence>
          </div>
        </main>
        
        {/* Absolute Footer Frame */}
        <AppFooter className="border-t border-[var(--app-border)] bg-[var(--app-panel)] py-5 text-center text-xs text-[var(--app-muted)]" />
      </div>
    </div>
  )
}
