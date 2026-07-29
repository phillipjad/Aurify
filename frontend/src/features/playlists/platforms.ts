import type { PlaylistSort } from '@/lib/api/queries'
import type { Platform } from '@/lib/api/types'

// Constants only — no components — so importing these never costs Fast Refresh
// in the modules that render them.

export const PLATFORMS: readonly Platform[] = ['spotify', 'apple_music', 'youtube_music']

const PLAYLIST_SORTS: readonly PlaylistSort[] = ['name', 'tracks']

/** The platform shown when neither the URL nor storage names one. */
export const DEFAULT_PLATFORM: Platform = 'spotify'

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

/** Narrow an untrusted value to an ordering the playlists endpoint accepts. */
export function toPlaylistSort(value: unknown): PlaylistSort | undefined {
  return PLAYLIST_SORTS.find((sort) => sort === value)
}

/**
 * The platform the user last chose, remembered across visits.
 *
 * The URL is the source of truth for view state, but the header's Playlists link
 * is a plain navigation carrying no search parameters, so without this every trip
 * through the nav bar landed back on the default. This is the fallback for that
 * case only: a platform named in the URL always wins, which keeps a shared link
 * showing what it says.
 *
 * Storage access is guarded because it throws outright in some privacy modes, and
 * a preference is never worth failing a render over.
 */
const LAST_PLATFORM_KEY = 'aurify.playlists.platform'

export function readLastPlatform(): Platform | undefined {
  try {
    return toPlatform(localStorage.getItem(LAST_PLATFORM_KEY))
  } catch {
    return undefined
  }
}

export function rememberLastPlatform(platform: Platform) {
  try {
    localStorage.setItem(LAST_PLATFORM_KEY, platform)
  } catch {
    // A remembered preference is a nicety, not a requirement.
  }
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
