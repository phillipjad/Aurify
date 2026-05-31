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
// produced it. Covers are persisted per-user so they can be revisited.
type Cover struct {
	ID           string           `bson:"_id,omitempty"`
	UserID       string           `bson:"user_id"`
	Platform     DSPPlatform      `bson:"platform"`
	PlaylistID   string           `bson:"playlist_id"`
	PlaylistName string           `bson:"playlist_name"`
	Status       CoverStatus      `bson:"status"`
	Prompt       string           `bson:"prompt"`
	ImageURL     string           `bson:"image_url"`
	Analysis     PlaylistAnalysis `bson:"analysis"`
	Error        string           `bson:"error,omitempty"`
	CreatedAt    time.Time        `bson:"created_at"`
	UpdatedAt    time.Time        `bson:"updated_at"`
}
