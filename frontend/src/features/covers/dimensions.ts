export interface Dimension {
  name: string
  hex: string
  signal: string
}

// The dimensions Aurify weighs, with the base color each contributes to a
// cover's palette. Mirrors `palette` in backend/internal/analysis/weights.go.
// Keep the names and hexes in sync when dimensions are added or retuned.
export const DIMENSIONS: Dimension[] = [
  { name: 'energetic', hex: '#FF5A36', signal: 'energy + liveness' },
  { name: 'danceable', hex: '#FFB23E', signal: 'danceability' },
  { name: 'euphoric', hex: '#FFE15D', signal: 'valence + lyric polarity' },
  { name: 'organic', hex: '#7FB069', signal: 'acousticness' },
  { name: 'introspective', hex: '#4F86C6', signal: 'instrumentalness' },
  { name: 'melancholic', hex: '#5C4D7D', signal: 'low valence + negative lyrics' },
  { name: 'intimate', hex: '#C46BAE', signal: 'speechiness' },
]
