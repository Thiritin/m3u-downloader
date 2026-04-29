package plex

import (
	"path/filepath"
	"testing"
)

// fp normalizes a forward-slash path to the host OS separator so tests pass
// on both Unix and Windows.
func fp(p string) string { return filepath.FromSlash(p) }

func TestSanitize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Normal Title", "Normal Title"},
		{"Bad/Slashes\\Here", "Bad Slashes Here"},
		{`A "quoted" film: part 1`, "A quoted film part 1"},
		{"trailing dots... ", "trailing dots"},
		{"  multiple   spaces  ", "multiple spaces"},
		{"a*b?c<d>e|f", "a b c d e f"},
	}
	for _, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("Sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMoviePath(t *testing.T) {
	got := MoviePath("/m", "The Godfather", 1972)
	want := fp("/m/The Godfather (1972)/The Godfather (1972).mkv")
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestMoviePath_NoYear(t *testing.T) {
	got := MoviePath("/m", "Untitled", 0)
	want := fp("/m/Untitled/Untitled.mkv")
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestEpisodePath(t *testing.T) {
	got := EpisodePath("/s", "Breaking Bad", 1, 1)
	want := fp("/s/Breaking Bad/Season 01/Breaking Bad - S01E01.mkv")
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestEpisodePath_DoubleDigitSeason(t *testing.T) {
	got := EpisodePath("/s", "Show", 12, 7)
	want := fp("/s/Show/Season 12/Show - S12E07.mkv")
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestExtractYear(t *testing.T) {
	cases := []struct {
		in       string
		want     int
		wantTrim string
	}{
		{"The Godfather (1972)", 1972, "The Godfather"},
		{"Inception 2010", 2010, "Inception"},
		{"No Year Here", 0, "No Year Here"},
	}
	for _, c := range cases {
		gotYear, gotTitle := ExtractYear(c.in)
		if gotYear != c.want || gotTitle != c.wantTrim {
			t.Errorf("ExtractYear(%q) = (%d,%q), want (%d,%q)",
				c.in, gotYear, gotTitle, c.want, c.wantTrim)
		}
	}
}
