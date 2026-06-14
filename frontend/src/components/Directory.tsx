import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useLocation } from 'react-router-dom'
import {
  type Category,
  getCategoriesPage,
  getCategoryStreamsPage,
  getFollowedChannels,
  getStreamsPage,
  search,
} from '../api'
import { useAuth } from '../auth'
import { useThemeEffect, useUiSettings } from '../settings'
import {
  getStoredDirectorySort,
  setStoredDirectorySort,
  sortDirectoryStreams,
  type DirectorySort,
} from '../utils/directorySort'
import {
  sortCategories,
  type CategorySort,
} from '../utils/categorySort'
import {
  getStoredBrowseTab,
  setStoredBrowseTab,
  type BrowseTab,
} from '../utils/browseTabs'
import { buildHomeFeed, followedChannelToStream } from '../utils/homeFeed'
import { CategoryBrowseGrid } from './directory/CategoryBrowseGrid'
import { CategorySortBar } from './directory/CategorySortBar'
import { DirectoryLayout } from './directory/DirectoryLayout'
import { DirectorySection } from './directory/DirectorySection'
import { DirectorySortBar } from './directory/DirectorySortBar'
import { EmptyStreams } from './directory/EmptyStreams'
import { RandomLiveHero } from './directory/RandomLiveHero'
import {
  CategoryGridSkeleton,
  SkeletonGrid,
  StreamGrid,
  StreamShelf,
  StreamShelfSkeleton,
} from './directory/StreamShelf'

const PAGE_SIZE = 30
const CATEGORY_PAGE_SIZE = 42

function ShowMoreButton({
  loading,
  onClick,
}: {
  loading: boolean
  onClick: () => void
}) {
  return (
    <div className="flex justify-center pt-2">
      <button
        type="button"
        onClick={onClick}
        disabled={loading}
        className="rounded-md bg-[#9147ff] px-6 py-2.5 text-sm font-bold text-white transition hover:bg-[#772ce8] disabled:opacity-60"
      >
        {loading ? 'Loading...' : 'Show more'}
      </button>
    </div>
  )
}

function BrowseTabButton({
  active,
  children,
  onClick,
}: {
  active: boolean
  children: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={`rounded-md px-3 py-1.5 text-sm font-bold transition ${
        active
          ? 'bg-[#9147ff] text-white'
          : 'border border-[#3a3a3d] bg-[#18181b] text-zinc-300 hover:border-[#53535a] hover:bg-[#1f1f23] hover:text-white'
      }`}
    >
      {children}
    </button>
  )
}

function HomeBrowseTabs({
  activeTab,
  onChange,
}: {
  activeTab: BrowseTab
  onChange: (tab: BrowseTab) => void
}) {
  return (
    <div className="flex items-center gap-2" role="tablist" aria-label="Browse">
      <BrowseTabButton active={activeTab === 'categories'} onClick={() => onChange('categories')}>
        Categories
      </BrowseTabButton>
      <BrowseTabButton active={activeTab === 'live'} onClick={() => onChange('live')}>
        Live Channels
      </BrowseTabButton>
    </div>
  )
}

export default function Directory() {
  const [q, setQ] = useState('')
  const [selectedCategory, setSelectedCategory] = useState<Category | null>(null)
  const settings = useUiSettings(s => s.settings)
  const auth = useAuth()
  const location = useLocation()
  const query = q.trim()
  const homeMode = !query && !selectedCategory
  useThemeEffect(settings.theme)

  const [sort, setSort] = useState<DirectorySort>(() => getStoredDirectorySort())
  const [categorySort, setCategorySort] = useState<CategorySort>('viewers')
  const [browseTab, setBrowseTab] = useState<BrowseTab>(() => getStoredBrowseTab())

  const handleSortChange = (value: DirectorySort) => {
    setSort(value)
    setStoredDirectorySort(value)
  }

  const handleBrowseTabChange = (value: BrowseTab) => {
    setBrowseTab(value)
    setStoredBrowseTab(value)
  }

  const handleSearchChange = (value: string) => {
    setQ(value)
    if (value.trim()) setSelectedCategory(null)
  }

  useEffect(() => {
    const state = location.state as {
      directorySearch?: string
      directoryCategoryId?: string
      directoryCategoryName?: string
    } | null
    if (!state) return
    if (state.directorySearch) {
      setQ(state.directorySearch)
      setSelectedCategory(null)
    } else if (state.directoryCategoryId && state.directoryCategoryName) {
      setQ('')
      setSelectedCategory({
        id: state.directoryCategoryId,
        name: state.directoryCategoryName,
        thumbnailUrl: '',
      })
    } else {
      return
    }
    window.history.replaceState({}, '', location.pathname)
  }, [location.pathname, location.state])

  const streamsQuery = useInfiniteQuery({
    queryKey: ['streams', { limit: PAGE_SIZE }],
    queryFn: ({ pageParam }) => getStreamsPage({ limit: PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: homeMode,
  })

  const categoriesQuery = useInfiniteQuery({
    queryKey: ['categories-browse-home', { limit: CATEGORY_PAGE_SIZE }],
    queryFn: ({ pageParam }) => getCategoriesPage({ limit: CATEGORY_PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: homeMode && browseTab === 'categories',
  })

  const followed = useQuery({
    queryKey: ['followed', auth.isAuthenticated],
    queryFn: () => getFollowedChannels(auth.isAuthenticated),
    enabled: homeMode,
    retry: false,
    staleTime: 30_000,
  })

  const searchResults = useQuery({
    queryKey: ['search', query],
    queryFn: () => search(query),
    enabled: query.length > 0,
  })

  const categoryStreamsQuery = useInfiniteQuery({
    queryKey: ['category-streams', selectedCategory?.id, { limit: PAGE_SIZE }],
    queryFn: ({ pageParam }) =>
      getCategoryStreamsPage(selectedCategory!.id, { limit: PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: Boolean(selectedCategory && !query),
  })

  const topStreams = useMemo(
    () => streamsQuery.data?.pages.flatMap(p => p.items) ?? [],
    [streamsQuery.data?.pages],
  )

  const homeFeed = useMemo(() => buildHomeFeed({
    followedChannels: followed.data ?? [],
    topStreams,
    recommendationLimit: 12,
  }), [followed.data, topStreams])

  const browseCategories = useMemo(
    () => categoriesQuery.data?.pages.flatMap(p => p.items) ?? [],
    [categoriesQuery.data?.pages],
  )

  const shownBrowseCategories = useMemo(
    () => sortCategories(browseCategories, categorySort),
    [browseCategories, categorySort],
  )

  const shownBrowseStreams = useMemo(
    () => sortDirectoryStreams(topStreams, sort),
    [topStreams, sort],
  )

  const rawSearchStreams = searchResults.data?.streams ?? []
  const shownSearchStreams = useMemo(
    () => sortDirectoryStreams(rawSearchStreams, sort),
    [rawSearchStreams, sort],
  )

  const selectedCategoryStreams = useMemo(
    () => categoryStreamsQuery.data?.pages.flatMap(p => p.items) ?? [],
    [categoryStreamsQuery.data?.pages],
  )

  const shownCategoryStreams = useMemo(
    () => sortDirectoryStreams(selectedCategoryStreams, sort),
    [selectedCategoryStreams, sort],
  )

  const loading = query ? searchResults.isLoading : streamsQuery.isLoading
  const error = query ? searchResults.error : streamsQuery.error

  return (
    <DirectoryLayout
      searchValue={q}
      onSearchChange={handleSearchChange}
    >
      {homeMode ? (
        <>
          <RandomLiveHero />

          {followed.isLoading ? (
            <DirectorySection title="Following Live" subtitle="Channels from your Twitch and Streamclone follows">
              <StreamShelfSkeleton />
            </DirectorySection>
          ) : homeFeed.followingLive.length ? (
            <DirectorySection title="Following Live" subtitle="Channels from your Twitch and Streamclone follows">
              <StreamShelf streams={homeFeed.followingLive.map(followedChannelToStream)} />
            </DirectorySection>
          ) : null}

          <DirectorySection
            title="Recommended Live Channels"
            subtitle="Top live channels outside your follows"
          >
            {error ? (
              <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
                Metadata service is not responding yet.
              </div>
            ) : loading ? (
              <StreamShelfSkeleton />
            ) : homeFeed.recommendedLive.length ? (
              <StreamShelf streams={homeFeed.recommendedLive} />
            ) : (
              <EmptyStreams />
            )}
          </DirectorySection>

          <DirectorySection
            title="Browse"
            subtitle={browseTab === 'categories' ? 'Categories sorted by live viewers' : 'Live channels sorted by viewers'}
            action={
              <div className="flex flex-col items-end gap-2">
                <div className="flex flex-wrap items-center gap-3">
                  <HomeBrowseTabs activeTab={browseTab} onChange={handleBrowseTabChange} />
                  {browseTab === 'categories' ? (
                    <CategorySortBar sort={categorySort} onSortChange={setCategorySort} />
                  ) : (
                    <DirectorySortBar sort={sort} onSortChange={handleSortChange} />
                  )}
                </div>
                {browseTab === 'categories' && categorySort === 'name' ? (
                  <p className="text-xs text-zinc-500">Name sort applies to loaded categories only</p>
                ) : null}
              </div>
            }
          >
            {browseTab === 'categories' ? (
              categoriesQuery.error ? (
                <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
                  Metadata service is not responding yet.
                </div>
              ) : categoriesQuery.isLoading ? (
                <CategoryGridSkeleton />
              ) : shownBrowseCategories.length ? (
                <>
                  <CategoryBrowseGrid
                    categories={shownBrowseCategories}
                    onSelect={category => {
                      setQ('')
                      setSelectedCategory(category)
                    }}
                  />
                  {categoriesQuery.hasNextPage ? (
                    <ShowMoreButton
                      loading={categoriesQuery.isFetchingNextPage}
                      onClick={() => categoriesQuery.fetchNextPage()}
                    />
                  ) : null}
                </>
              ) : (
                <div className="grid min-h-64 place-items-center rounded-md bg-[#18181b] px-6 text-center">
                  <div>
                    <div className="text-lg font-bold text-white">No categories loaded yet</div>
                    <div className="mt-2 text-sm text-zinc-400">Categories will appear when metadata finishes loading.</div>
                  </div>
                </div>
              )
            ) : error ? (
              <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
                Metadata service is not responding yet.
              </div>
            ) : loading ? (
              <SkeletonGrid />
            ) : shownBrowseStreams.length ? (
              <>
                <StreamGrid streams={shownBrowseStreams} />
                {streamsQuery.hasNextPage ? (
                  <ShowMoreButton
                    loading={streamsQuery.isFetchingNextPage}
                    onClick={() => streamsQuery.fetchNextPage()}
                  />
                ) : null}
              </>
            ) : (
              <EmptyStreams />
            )}
          </DirectorySection>
        </>
      ) : selectedCategory && !query ? (
        <DirectorySection
          title={selectedCategory.name}
          subtitle="Category streams"
          action={
            <div className="flex flex-wrap items-center gap-3">
              <button
                type="button"
                onClick={() => setSelectedCategory(null)}
                className="rounded-md border border-[#3a3a3d] bg-[#18181b] px-3 py-1.5 text-sm font-semibold text-zinc-200 transition hover:border-[#53535a] hover:bg-[#1f1f23]"
              >
                All categories
              </button>
              <DirectorySortBar sort={sort} onSortChange={handleSortChange} />
            </div>
          }
        >
          {categoryStreamsQuery.error ? (
            <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
              Metadata service is not responding yet.
            </div>
          ) : categoryStreamsQuery.isLoading ? (
            <SkeletonGrid />
          ) : shownCategoryStreams.length ? (
            <>
              <StreamGrid streams={shownCategoryStreams} />
              {categoryStreamsQuery.hasNextPage ? (
                <ShowMoreButton
                  loading={categoryStreamsQuery.isFetchingNextPage}
                  onClick={() => categoryStreamsQuery.fetchNextPage()}
                />
              ) : null}
            </>
          ) : (
            <EmptyStreams categoryName={selectedCategory.name} />
          )}
        </DirectorySection>
      ) : (
        <DirectorySection
          title={`Search: ${query}`}
          subtitle="Streams matching your search"
          action={<DirectorySortBar sort={sort} onSortChange={handleSortChange} />}
        >
          {error ? (
            <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
              Metadata service is not responding yet.
            </div>
          ) : loading ? (
            <SkeletonGrid />
          ) : shownSearchStreams.length ? (
            <StreamGrid streams={shownSearchStreams} />
          ) : (
            <EmptyStreams query={query} />
          )}
        </DirectorySection>
      )}
    </DirectoryLayout>
  )
}
