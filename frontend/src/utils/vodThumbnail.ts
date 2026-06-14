export function vodThumbnailUrl(value: string | undefined, width = 160, height = 90) {
  if (!value) return ''
  return value
    .replace(/%\{width\}/g, String(width))
    .replace(/%\{height\}/g, String(height))
    .replace(/\{width\}/g, String(width))
    .replace(/\{height\}/g, String(height))
}
