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
