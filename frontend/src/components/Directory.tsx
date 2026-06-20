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
import { PULSE_WIRE_ENABLED } from '../config'
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
import { buildHomeFeed, followedChannelToStream } from '../utils/homeFeed'
import { CategoryShowcaseRow } from './directory/CategoryShowcaseRow'
import { CategoryStreamShelfSection } from './directory/CategoryStreamShelfSection'
import { DirectoryLayout } from './directory/DirectoryLayout'
import { DirectorySection } from './directory/DirectorySection'
import { DirectorySortBar } from './directory/DirectorySortBar'
import { EmptyStreams } from './directory/EmptyStreams'
import { RandomLiveHero } from './directory/RandomLiveHero'
import { ShowAllLink } from './directory/ShowAllLink'
import {
  CategoryGridSkeleton,
  SkeletonGrid,
  StreamGrid,
  StreamShelf,
  StreamShelfSkeleton,
} from './directory/StreamShelf'

const PAGE_SIZE = 30
const CATEGORY_PAGE_SIZE = 42
const HOME_CATEGORY_SHELVES = 5
const HOME_CATEGORY_SHOWCASE = 16

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
  const [categorySort] = useState<CategorySort>('viewers')

  const handleSortChange = (value: DirectorySort) => {
    setSort(value)
    setStoredDirectorySort(value)
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
    enabled: homeMode,
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

  const homeCategoryShelves = useMemo(
    () => shownBrowseCategories.slice(0, HOME_CATEGORY_SHELVES),
    [shownBrowseCategories],
  )

  const selectedCategoryStreams = useMemo(
    () => categoryStreamsQuery.data?.pages.flatMap(p => p.items) ?? [],
    [categoryStreamsQuery.data?.pages],
  )

  const shownCategoryStreams = useMemo(
    () => sortDirectoryStreams(selectedCategoryStreams, sort),
    [selectedCategoryStreams, sort],
  )

  const rawSearchStreams = searchResults.data?.streams ?? []
  const shownSearchStreams = useMemo(
    () => sortDirectoryStreams(rawSearchStreams, sort),
    [rawSearchStreams, sort],
  )

  const loading = query ? searchResults.isLoading : streamsQuery.isLoading
  const error = query ? searchResults.error : streamsQuery.error

  return (
    <DirectoryLayout
      searchValue={q}
      onSearchChange={handleSearchChange}
      showBrowseLink
      showNetworkLink
      showPulseWireLink={PULSE_WIRE_ENABLED}
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
            title="Categories"
            subtitle="Browse games and categories"
            action={<ShowAllLink to="/browse" />}
          >
            {categoriesQuery.error ? (
              <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
                Metadata service is not responding yet.
              </div>
            ) : categoriesQuery.isLoading ? (
              <CategoryGridSkeleton />
            ) : shownBrowseCategories.length ? (
              <CategoryShowcaseRow categories={shownBrowseCategories.slice(0, HOME_CATEGORY_SHOWCASE)} />
            ) : (
              <div className="grid min-h-32 place-items-center rounded-md bg-[#18181b] px-6 text-center">
                <div className="text-sm text-zinc-400">Categories will appear when metadata finishes loading.</div>
              </div>
            )}
          </DirectorySection>

          {homeCategoryShelves.map(category => (
            <CategoryStreamShelfSection key={category.id} category={category} />
          ))}
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
