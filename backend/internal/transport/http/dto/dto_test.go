package dto_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
)

// The generation prompt is internal (ADR 0022). It is kept in Postgres because
// it is the only thing that explains a provider refusal after the fact, which
// means the domain carries it and one careless field would put it back on the
// wire. This asserts against the encoded bytes rather than the struct, so it
// still holds if someone adds the field back with a json tag.
func TestNoResponseCarriesTheGenerationPrompt(t *testing.T) {
	const secret = "a hazy sunlit room, shot on expired film"

	cover := &domain.Cover{
		ID:           "cvr_1",
		Status:       domain.CoverStatusReady,
		Platform:     domain.PlatformSpotify,
		PlaylistID:   "pl_1",
		PlaylistName: "Sunday Morning Reset",
		ImageURL:     "/api/v1/covers/rev_1/image",
		RunCount:     1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Revisions: []domain.CoverRevision{{
			ID:          "rev_1",
			Number:      31,
			Status:      domain.CoverStatusReady,
			Prompt:      secret,
			ImageURL:    "/api/v1/covers/rev_1/image",
			CompletedAt: time.Now().UTC(),
		}},
	}

	encoded, err := json.Marshal(dto.NewCoverResponse(cover))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if body := string(encoded); strings.Contains(body, secret) || strings.Contains(body, `"prompt"`) {
		t.Errorf("the prompt reached the client:\n%s", body)
	}
}

// The number is what the client labels a run with, so it has to survive the
// mapping. An int that silently stayed zero would render every run as
// "Revision #0".
func TestRevisionNumberIsCarriedToTheClient(t *testing.T) {
	cover := &domain.Cover{
		Revisions: []domain.CoverRevision{{ID: "rev_1", Number: 31, Status: domain.CoverStatusReady}},
	}

	got := dto.NewCoverResponse(cover)
	if len(got.Revisions) != 1 {
		t.Fatalf("revisions = %d, want 1", len(got.Revisions))
	}
	if got.Revisions[0].Number != 31 {
		t.Errorf("number = %d, want 31", got.Revisions[0].Number)
	}
}
