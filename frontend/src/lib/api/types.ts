// Wire types shared by the API client. These mirror the backend DTOs in
// backend/internal/transport/http/dto. Keep them in sync (or generate them from
// the OpenAPI spec the backend can emit — see backend ADR 0007).

export type Platform = 'spotify' | 'apple_music' | 'youtube_music'

export interface Playlist {
  id: string
  platform: Platform
  name: string
  description: string
  trackCount: number
  imageUrl?: string
}

export interface ColorWeight {
  dimension: string
  hexColor: string
  weight: number
}

export type CoverStatus =
  | 'pending'
  | 'analyzing'
  | 'generating'
  | 'ready'
  | 'failed'

export interface Cover {
  id: string
  status: CoverStatus
  platform: Platform
  playlistId: string
  imageUrl?: string
  prompt?: string
  palette?: ColorWeight[]
  createdAt: string
}

export interface GenerateCoverRequest {
  platform: Platform
  playlistId: string
}
