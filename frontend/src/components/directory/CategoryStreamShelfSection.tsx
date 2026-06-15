import { useQuery } from '@tanstack/react-query'
import type { Category } from '../../api'
import { getCategoryStreamsPage } from '../../api'
import { DirectorySection } from './DirectorySection'
import { ShowAllLink } from './ShowAllLink'
import { StreamShelf, StreamShelfSkeleton } from './StreamShelf'

interface CategoryStreamShelfSectionProps {
  category: Category
}

export function CategoryStreamShelfSection({ category }: CategoryStreamShelfSectionProps) {
  const query = useQuery({
    queryKey: ['category-streams-home', category.id, { limit: 12 }],
    queryFn: () => getCategoryStreamsPage(category.id, { limit: 12 }),
    staleTime: 30_000,
  })

  const streams = query.data?.items ?? []
  if (!query.isLoading && !streams.length) return null

  const browsePath = `/browse/category/${category.id}?name=${encodeURIComponent(category.name)}`

  return (
    <DirectorySection
      title={`Live in ${category.name}`}
      subtitle="Popular streams in this category"
      action={<ShowAllLink to={browsePath} />}
    >
      {query.isLoading ? (
        <StreamShelfSkeleton />
      ) : (
        <StreamShelf streams={streams} />
      )}
    </DirectorySection>
  )
}
