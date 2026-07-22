import { useState, type ComponentProps, type ReactNode } from 'react'

type ImageWithFallbackProps = Omit<ComponentProps<'img'>, 'src'> & {
  src?: string
  /** Rendered when there is no src, or the image fails to load. */
  fallback: ReactNode
}

/**
 * An <img> that degrades to a supplied placeholder instead of the browser's
 * broken-image glyph — for both a missing URL and one that 404s at load time.
 */
export function ImageWithFallback({ src, fallback, alt, ...props }: ImageWithFallbackProps) {
  const [failed, setFailed] = useState(false)

  if (!src || failed) return <>{fallback}</>

  return <img src={src} alt={alt} onError={() => setFailed(true)} {...props} />
}
