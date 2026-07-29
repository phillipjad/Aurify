import type { Platform } from '@/lib/api/types'

// Constants only — no components — so importing these never costs Fast Refresh
// in the modules that render them.

export const PLATFORMS: readonly Platform[] = ['spotify', 'apple_music', 'youtube_music']

const PLATFORM_LABELS: Record<Platform, string> = {
  spotify: 'Spotify',
  apple_music: 'Apple Music',
  youtube_music: 'YouTube Music',
}

export function platformLabel(platform: Platform): string {
  return PLATFORM_LABELS[platform]
}

/** Narrow an untrusted value (a URL parameter) to a platform we actually serve. */
export function toPlatform(value: unknown): Platform | undefined {
  return PLATFORMS.find((platform) => platform === value)
}

/**
 * Copy for the `connect_error` codes the API's OAuth callback redirects with
 * (see backend/internal/transport/http/handlers/auth.go). The codes are coarse
 * on purpose — the precise reason a handshake failed is reconnaissance — so this
 * says what to do next rather than what went wrong.
 */
export function connectErrorMessage(code: string, label: string): string {
  switch (code) {
    case 'cancelled':
      return `You didn’t finish giving Aurify access to ${label}.`
    case 'expired':
      return `That took a little too long. Start the ${label} connection again.`
    case 'session':
      return `Your session expired while you were away. Sign in, then reconnect ${label}.`
    default:
      return `Aurify couldn’t finish connecting ${label}. Try again.`
  }
}
