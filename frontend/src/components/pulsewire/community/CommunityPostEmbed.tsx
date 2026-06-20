import { withTwitchEmbedParent } from '../../../utils/twitchEmbed'

const IFRAME_EMBED_HOSTS = new Set([
  'www.youtube.com',
  'youtube.com',
  'youtu.be',
  'player.twitch.tv',
  'clips.twitch.tv',
  'www.tiktok.com',
  'tiktok.com',
  'www.redditmedia.com',
  'redditmedia.com',
])

function safeIframeSrc(value?: string) {
  if (!value) return ''
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol)) return ''
    if (!IFRAME_EMBED_HOSTS.has(url.hostname.toLowerCase())) return ''
    return withTwitchEmbedParent(url.toString())
  } catch {
    return ''
  }
}

type Props = {
  embedUrl?: string
  embedHtml?: string
  linkedPlatform?: string
  title?: string
  className?: string
}

export default function CommunityPostEmbed({ embedUrl, embedHtml, linkedPlatform, title, className = '' }: Props) {
  const iframeSrc = safeIframeSrc(embedUrl)
  const html = embedHtml?.trim()

  if (iframeSrc) {
    return (
      <div className={`aspect-video bg-black ${className}`}>
        <iframe
          data-testid="community-embed"
          src={iframeSrc}
          title={title || linkedPlatform || 'Linked embed'}
          className="h-full w-full"
          loading="lazy"
          sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
          allow="accelerometer; autoplay; clipboard-write; encrypted-media; picture-in-picture"
        />
      </div>
    )
  }

  if (html) {
    return (
      <div
        data-testid="community-embed"
        className={`overflow-hidden bg-[#0C0C0F] px-3 py-2 text-sm text-[#D6D6DE] [&_iframe]:max-w-full [&_blockquote]:border-l-2 [&_blockquote]:border-[#FF4500]/50 [&_blockquote]:pl-3 ${className}`}
        // Server-side sanitizeEmbedHTML strips scripts and unsafe attrs before storage.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }

  return null
}
