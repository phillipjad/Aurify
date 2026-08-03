package domain

import "time"

// CoverStatus tracks the lifecycle of a cover-generation job.
type CoverStatus string

// Cover lifecycle states, from creation through generation to a final result.
const (
	CoverStatusPending    CoverStatus = "pending"
	CoverStatusAnalyzing  CoverStatus = "analyzing"
	CoverStatusGenerating CoverStatus = "generating"
	CoverStatusReady      CoverStatus = "ready"
	CoverStatusFailed     CoverStatus = "failed"
)

// Cover is a generated playlist/album cover together with the analysis that
// produced it. Covers are persisted per-user so they can be revisited. The
// Analysis is stored as a JSONB column (see docs/adr/0010-postgresql-storage.md).
type Cover struct {
	ID           string
	UserID       string
	Platform     DSPPlatform
	PlaylistID   string
	PlaylistName string
	Status       CoverStatus
	Prompt       string
	ImageURL     string
	Analysis     PlaylistAnalysis
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GeneratedImage is the rendered cover art before it has been stored anywhere.
//
// ContentType travels with the bytes because it is the provider that decides it:
// some image APIs return JPEG and others PNG, and the route that later serves the
// bytes has nothing else to go on.
type GeneratedImage struct {
	Bytes       []byte
	ContentType string
}
