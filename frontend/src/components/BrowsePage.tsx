import { useEffect, useMemo, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  getCategoriesPage,
  getCategoryStreamsPage,
  getStreamsPage,
} from '../api'
import { useThemeEffect, useUiSettings } from '../settings'
import {
  browseTabFromPathname,
  setStoredBrowseTab,
  type BrowseTab,
} from '../utils/browseTabs'
import {
  sortCategories,
  type CategorySort,
} from '../utils/categorySort'
import {
  getStoredDirectorySort,
  setStoredDirectorySort,
  sortDirectoryStreams,
  type DirectorySort,
} from '../utils/directorySort'
import { BrowseTabBar } from './directory/BrowseTabBar'
import { CategoryBrowseGrid } from './directory/CategoryBrowseGrid'
import { CategorySortBar } from './directory/CategorySortBar'
import { DirectoryLayout } from './directory/DirectoryLayout'
import { DirectorySection } from './directory/DirectorySection'
import { DirectorySortBar } from './directory/DirectorySortBar'
import { EmptyStreams } from './directory/EmptyStreams'
import {
  CategoryGridSkeleton,
  SkeletonGrid,
  StreamGrid,
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

export default function BrowsePage() {
  const { categoryId } = useParams<{ categoryId?: string }>()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const settings = useUiSettings(s => s.settings)
  useThemeEffect(settings.theme)

  const categoryMode = Boolean(categoryId)
  const pathnameTab = browseTabFromPathname(location.pathname)
  const activeTab: BrowseTab = categoryMode ? 'categories' : (pathnameTab ?? 'categories')

  const [sort, setSort] = useState<DirectorySort>(() => getStoredDirectorySort())
  const [categorySort, setCategorySort] = useState<CategorySort>('viewers')
  const searchQuery = (searchParams.get('q') ?? '').trim()

  useEffect(() => {
    if (!categoryMode && pathnameTab) {
      setStoredBrowseTab(pathnameTab)
    }
  }, [categoryMode, pathnameTab])

  const handleSortChange = (value: DirectorySort) => {
    setSort(value)
    setStoredDirectorySort(value)
  }

  const browseCategoriesQuery = useInfiniteQuery({
    queryKey: ['categories-browse', { limit: CATEGORY_PAGE_SIZE }],
    queryFn: ({ pageParam }) => getCategoriesPage({ limit: CATEGORY_PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: !categoryMode && activeTab === 'categories',
  })

  const streamsQuery = useInfiniteQuery({
    queryKey: ['streams-browse', { limit: PAGE_SIZE, q: searchQuery || undefined }],
    queryFn: ({ pageParam }) => getStreamsPage({ limit: PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: !categoryMode && activeTab === 'live',
  })

  const categoryStreamsQuery = useInfiniteQuery({
    queryKey: ['category-streams-browse', categoryId, { limit: PAGE_SIZE }],
    queryFn: ({ pageParam }) =>
      getCategoryStreamsPage(categoryId!, { limit: PAGE_SIZE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: lastPage => (lastPage.cursor ? lastPage.cursor : undefined),
    enabled: categoryMode,
  })

  const browseCategories = useMemo(
    () => browseCategoriesQuery.data?.pages.flatMap(p => p.items) ?? [],
    [browseCategoriesQuery.data?.pages],
  )
  const shownCategories = useMemo(
    () => sortCategories(browseCategories, categorySort),
    [browseCategories, categorySort],
  )

  const liveStreams = useMemo(
    () => streamsQuery.data?.pages.flatMap(p => p.items) ?? [],
    [streamsQuery.data?.pages],
  )
  const shownLiveStreams = useMemo(() => {
    const sorted = sortDirectoryStreams(liveStreams, sort)
    if (!searchQuery) return sorted
    const needle = searchQuery.toLowerCase()
    return sorted.filter(
      stream =>
        stream.login.toLowerCase().includes(needle)
        || (stream.displayName ?? '').toLowerCase().includes(needle)
        || (stream.title ?? '').toLowerCase().includes(needle),
    )
  }, [liveStreams, sort, searchQuery])

  const categoryStreams = useMemo(
    () => categoryStreamsQuery.data?.pages.flatMap(p => p.items) ?? [],
    [categoryStreamsQuery.data?.pages],
  )
  const shownCategoryStreams = useMemo(
    () => sortDirectoryStreams(categoryStreams, sort),
    [categoryStreams, sort],
  )

  const categoryNameFromQuery = searchParams.get('name')?.trim() || null
  const categoryTitle = categoryNameFromQuery || categoryId || 'Category'

  const handleCategorySelect = (category: { id: string; name: string }) => {
    navigate(`/browse/category/${category.id}?name=${encodeURIComponent(category.name)}`)
  }

  return (
    <DirectoryLayout headerSubtitle="Browse" showBrowseLink browseActive>
      {categoryMode ? (
        <DirectorySection
          title={categoryTitle}
          subtitle="Category streams"
          action={
            <div className="flex flex-wrap items-center gap-3">
              <Link
                to="/browse"
                className="rounded-md border border-[#3a3a3d] bg-[#18181b] px-3 py-1.5 text-sm font-semibold text-zinc-200 transition hover:border-[#53535a] hover:bg-[#1f1f23]"
              >
                All categories
              </Link>
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
            <EmptyStreams categoryName={categoryTitle} />
          )}
        </DirectorySection>
      ) : (
        <DirectorySection
          title="Browse"
          subtitle={
            activeTab === 'categories'
              ? 'Categories sorted by live viewers'
              : searchQuery
                ? `Live channels matching "${searchQuery}"`
                : 'Live channels sorted by viewers'
          }
          action={
            <div className="flex flex-col items-end gap-2">
              <div className="flex flex-wrap items-center gap-3">
                <BrowseTabBar activeTab={activeTab} />
                {activeTab === 'categories' ? (
                  <CategorySortBar sort={categorySort} onSortChange={setCategorySort} />
                ) : (
                  <DirectorySortBar sort={sort} onSortChange={handleSortChange} />
                )}
              </div>
              {activeTab === 'categories' && categorySort === 'name' ? (
                <p className="text-xs text-zinc-500">Name sort applies to loaded categories only</p>
              ) : null}
            </div>
          }
        >
          {activeTab === 'categories' ? (
            browseCategoriesQuery.error ? (
              <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
                Metadata service is not responding yet.
              </div>
            ) : browseCategoriesQuery.isLoading ? (
              <CategoryGridSkeleton />
            ) : shownCategories.length ? (
              <>
                <CategoryBrowseGrid categories={shownCategories} onSelect={handleCategorySelect} />
                {browseCategoriesQuery.hasNextPage ? (
                  <ShowMoreButton
                    loading={browseCategoriesQuery.isFetchingNextPage}
                    onClick={() => browseCategoriesQuery.fetchNextPage()}
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
          ) : streamsQuery.error ? (
            <div className="rounded-md border border-red-400/30 bg-red-500/10 p-5 text-sm font-semibold text-red-100">
              Metadata service is not responding yet.
            </div>
          ) : streamsQuery.isLoading ? (
            <SkeletonGrid />
          ) : shownLiveStreams.length ? (
            <>
              <StreamGrid streams={shownLiveStreams} />
              {streamsQuery.hasNextPage ? (
                <ShowMoreButton
                  loading={streamsQuery.isFetchingNextPage}
                  onClick={() => streamsQuery.fetchNextPage()}
                />
              ) : null}
            </>
          ) : (
            <EmptyStreams query={searchQuery || undefined} />
          )}
        </DirectorySection>
      )}
    </DirectoryLayout>
  )
}
