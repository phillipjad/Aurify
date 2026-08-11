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

// Terminal reports whether the status will never change again without a new
// user action.
func (s CoverStatus) Terminal() bool {
	return s == CoverStatusReady || s == CoverStatusFailed
}

// Cover is one playlist's artwork. The playlist is the identity — a cover is a
// rendering *of* a playlist — and each generation is a CoverRevision of it (see
// docs/adr/0022-covers-by-playlist.md).
//
// Status, Error and the timestamps describe the newest run, including one still
// in flight. ImageURL and Analysis describe the *current artwork*, which is the
// newest revision that reached "ready": during a regeneration that is still the
// previous run's, which is what keeps the tile from blanking.
type Cover struct {
	ID           string
	UserID       string
	Platform     DSPPlatform
	PlaylistID   string
	PlaylistName string
	Status       CoverStatus
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// The current artwork. Read-only: they are stored on the revision, and a
	// Save never writes them back.
	ImageURL string
	Analysis PlaylistAnalysis
	// RunCount is how many successful runs this playlist has, matching the
	// default view of Revisions.
	RunCount int
	// Revisions is the run history, newest first, and only populated by the
	// reads that ask for it.
	Revisions []CoverRevision
}

// CoverRevision is one finished generation run.
//
// Rows exist only for terminal runs: a revision is written once, on ready or
// failed, so in-flight lifecycle lives solely on the Cover. A run interrupted by
// a crash or scale-down therefore leaves none, because nothing was produced.
type CoverRevision struct {
	ID      string
	CoverID string
	// Number is this run's place in its cover's history, counting from 1 and
	// including runs that failed. Assigned by the store on insert and never
	// reused, so deleting a run leaves a gap rather than renumbering the rest.
	Number int
	// Status is terminal: ready or failed.
	Status CoverStatus
	// Prompt is internal. It is kept because it is the only thing that explains
	// a provider refusal after the fact, and it is deliberately absent from
	// every API response (see docs/adr/0022-covers-by-playlist.md).
	Prompt      string
	ImageURL    string
	Analysis    PlaylistAnalysis
	Error       string
	CompletedAt time.Time
}

// CoverCursor is a keyset position in a user's cover list: everything strictly
// older than (UpdatedAt, ID). The zero value means "from the top".
//
// Keyset rather than an offset because a regeneration reorders a playlist to the
// front mid-scroll, which makes an offset duplicate or skip tiles. ID is the
// tiebreaker, since UpdatedAt can collide.
type CoverCursor struct {
	UpdatedAt time.Time
	ID        string
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
