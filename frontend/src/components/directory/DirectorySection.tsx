import type { ReactNode } from 'react'

interface DirectorySectionProps {
  title: string
  subtitle?: string
  action?: ReactNode
  children: ReactNode
}

export function DirectorySection({ title, subtitle, action, children }: DirectorySectionProps) {
  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-xl font-bold tracking-tight text-white sm:text-2xl">{title}</h2>
          {subtitle ? <p className="mt-1 text-sm text-zinc-400">{subtitle}</p> : null}
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      {children}
    </section>
  )
}
