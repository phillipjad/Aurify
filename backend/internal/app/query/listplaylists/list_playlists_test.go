package listplaylists

import (
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

func names(playlists []domain.Playlist) []string {
	out := make([]string, len(playlists))
	for i, p := range playlists {
		out[i] = p.Name
	}
	return out
}

func sample() []domain.Playlist {
	return []domain.Playlist{
		{Name: "Morning Coffee", Description: "gentle acoustic", TrackCount: 42},
		{Name: "Gym Bangers", Description: "high energy", TrackCount: 120},
		{Name: "Rainy Day", Description: "mellow COFFEE house", TrackCount: 8},
	}
}

func TestFilterPlaylists(t *testing.T) {
	all := sample()

	if got := filterPlaylists(all, ""); len(got) != 3 {
		t.Fatalf("empty term should keep all, got %d", len(got))
	}

	// Case-insensitive, matches name or description ("coffee" hits Morning
	// Coffee by name and Rainy Day by description).
	got := names(filterPlaylists(all, "COFFEE"))
	want := []string{"Morning Coffee", "Rainy Day"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("filter coffee = %v, want %v", got, want)
	}

	if got := filterPlaylists(all, "zzz"); len(got) != 0 {
		t.Fatalf("no match should be empty, got %v", names(got))
	}
}

func TestSortPlaylists(t *testing.T) {
	byName := sample()
	sortPlaylists(byName, SortName)
	if got := names(byName); got[0] != "Gym Bangers" || got[2] != "Rainy Day" {
		t.Fatalf("sort by name = %v", got)
	}

	byTracks := sample()
	sortPlaylists(byTracks, SortTracks)
	if got := names(byTracks); got[0] != "Gym Bangers" || got[2] != "Rainy Day" {
		t.Fatalf("sort by tracks = %v (want most first)", got)
	}

	// An unknown key falls back to name ordering rather than leaving it unsorted.
	fallback := sample()
	sortPlaylists(fallback, SortKey("bogus"))
	if names(fallback)[0] != "Gym Bangers" {
		t.Fatalf("unknown sort should fall back to name, got %v", names(fallback))
	}
}

func TestPaginate(t *testing.T) {
	all := sample()

	if got := paginate(all, 2, 0); len(got) != 2 || got[0].Name != "Morning Coffee" {
		t.Fatalf("first page = %v", names(got))
	}
	if got := paginate(all, 2, 2); len(got) != 1 || got[0].Name != "Rainy Day" {
		t.Fatalf("second page = %v", names(got))
	}
	if got := paginate(all, 2, 99); len(got) != 0 {
		t.Fatalf("offset past the end should be empty, got %v", names(got))
	}
	// A zero/oversized limit clamps to the default rather than returning nothing.
	if got := paginate(all, 0, 0); len(got) != 3 {
		t.Fatalf("zero limit should clamp to default, got %d", len(got))
	}
}
