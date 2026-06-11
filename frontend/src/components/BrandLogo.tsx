type BrandLogoProps = {
  size?: 'sm' | 'md' | 'lg'
  showText?: boolean
  subtitle?: string
  className?: string
}

const sizePx = { sm: 32, md: 40, lg: 48 } as const

export default function BrandLogo({
  size = 'md',
  showText = true,
  subtitle,
  className = '',
}: BrandLogoProps) {
  const px = sizePx[size]
  return (
    <div className={`flex min-w-0 items-center gap-3 ${className}`}>
      <img
        src="/logo.svg"
        alt=""
        width={px}
        height={px}
        className="shrink-0 rounded-lg shadow-lg shadow-violet-950/40"
        aria-hidden
      />
      {showText ? (
        <div className="min-w-0">
          <div className="truncate text-lg font-black tracking-tight text-white">Streamclone</div>
          {subtitle ? (
            <div className="truncate text-xs font-semibold uppercase tracking-[0.18em] text-cyan-200/80">
              {subtitle}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
