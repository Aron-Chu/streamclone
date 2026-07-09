import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../auth'
import BrandLogo from '../BrandLogo'
import { DirectorySearchField } from '../ChannelSearchInput'
import ChannelRail from '../ChannelRail'
import LocalTokenImportButton from '../LocalTokenImportButton'
import ServiceStatusBanner from '../ServiceStatusBanner'
import StackStatusButton from '../StackStatusButton'
import SettingsButton from '../SettingsPanel'

function HeaderAuth() {
  const auth = useAuth()
  if (auth.isAuthenticated) {
    return (
      <div className="flex shrink-0 items-center gap-2">
        <div className="hidden min-w-0 text-right sm:block">
          <div className="max-w-32 truncate text-xs font-black text-white">{auth.user?.displayName || auth.user?.display_name || auth.user?.login}</div>
          <div className="text-[11px] font-semibold text-emerald-300">Connected</div>
        </div>
        <button onClick={auth.logout} className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-200 transition hover:bg-white/10">
          Log out
        </button>
      </div>
    )
  }
  return (
    <div className="flex shrink-0 items-center gap-2">
      <LocalTokenImportButton compact />
    </div>
  )
}

interface DirectoryLayoutProps {
  children: ReactNode
  searchValue?: string
  onSearchChange?: (value: string) => void
  headerSubtitle?: string
  showBrowseLink?: boolean
  browseActive?: boolean
}

export function DirectoryLayout({
  children,
  searchValue,
  onSearchChange,
  headerSubtitle = 'Live directory',
  showBrowseLink = false,
  browseActive = false,
}: DirectoryLayoutProps) {
  const [mobileRailOpen, setMobileRailOpen] = useState(false)
  const [railCollapsed, setRailCollapsed] = useState(false)

  return (
    <main className="min-h-screen overflow-hidden bg-[#0A0A0D] text-zinc-100">
      <div className="pointer-events-none fixed inset-0 bg-[linear-gradient(135deg,rgba(139,92,246,.08),transparent_28%)]" />
      <div className="relative flex min-h-screen">
        <ChannelRail
          collapsed={railCollapsed}
          mobileOpen={mobileRailOpen}
          onToggleCollapsed={() => setRailCollapsed(v => !v)}
          onCloseMobile={() => setMobileRailOpen(false)}
        />
        <div className="min-w-0 flex-1 overflow-hidden">
          <div className="mx-auto flex min-h-screen w-full max-w-[1600px] flex-col px-4 py-5 sm:px-6 lg:px-8">
            <header className="sticky top-0 z-20 -mx-4 border-b border-[#1C1C21] bg-[#0E0E11]/90 px-4 py-4 backdrop-blur-xl sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex items-center justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <button onClick={() => setMobileRailOpen(true)} className="rounded border border-[#3a3a3d] bg-[#18181b] px-3 py-2 text-sm font-black text-white lg:hidden">
                      Menu
                    </button>
                    <Link to="/" className="flex items-center gap-3">
                      <BrandLogo subtitle={headerSubtitle} />
                    </Link>
                    {showBrowseLink ? (
                      <Link
                        to="/browse"
                        className={`hidden text-sm font-bold transition sm:inline ${
                          browseActive ? 'text-[#A970FF]' : 'text-zinc-400 hover:text-white'
                        }`}
                      >
                        Browse
                      </Link>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2 lg:hidden">
                    <StackStatusButton />
                    <SettingsButton />
                    <HeaderAuth />
                  </div>
                </div>
                <div className="flex w-full items-center gap-3 lg:max-w-3xl">
                  {onSearchChange && searchValue !== undefined ? (
                    <DirectorySearchField
                      value={searchValue}
                      onChange={onSearchChange}
                    />
                  ) : (
                    <div className="min-w-0 flex-1" />
                  )}
                  <div className="hidden lg:block">
                    <div className="flex items-center gap-2">
                      <StackStatusButton />
                      <SettingsButton />
                      <HeaderAuth />
                    </div>
                  </div>
                </div>
              </div>
            </header>
            <ServiceStatusBanner />

            <section className="flex flex-1 flex-col gap-8 py-6">
              {children}
            </section>
          </div>
        </div>
      </div>
    </main>
  )
}
