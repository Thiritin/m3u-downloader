// Package plex builds Plex-canonical file paths and sanitizes titles
// to be macOS filesystem-safe. Pure functions, no I/O.
package plex

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	illegal     = regexp.MustCompile(`[/\\:*?"<>|]`)
	whitespaces = regexp.MustCompile(`\s+`)
	yearRE      = regexp.MustCompile(`\s*\(?(\d{4})\)?\s*$`)
)

// Sanitize strips filesystem-illegal characters, collapses whitespace,
// and trims trailing dots and spaces.
func Sanitize(s string) string {
	s = illegal.ReplaceAllString(s, " ")
	s = whitespaces.ReplaceAllString(s, " ")
	s = strings.Trim(s, ". ")
	return s
}

// ExtractYear pulls a trailing 4-digit year from a title. Returns
// (year, titleWithoutYear). If no year is found, returns (0, original).
func ExtractYear(title string) (int, string) {
	m := yearRE.FindStringSubmatch(title)
	if m == nil {
		return 0, title
	}
	y := 0
	fmt.Sscanf(m[1], "%d", &y)
	if y < 1900 || y > 2100 {
		return 0, title
	}
	return y, strings.TrimSpace(yearRE.ReplaceAllString(title, ""))
}

// MoviePath returns "{root}/{Title} (Year)/{Title} (Year).mkv".
// If year == 0, the " (Year)" suffix is omitted.
func MoviePath(root, title string, year int) string {
	folder := movieFolder(title, year)
	return filepath.Join(root, folder, folder+".mkv")
}

func movieFolder(title string, year int) string {
	t := Sanitize(title)
	if year > 0 {
		return fmt.Sprintf("%s (%d)", t, year)
	}
	return t
}

// EpisodePath returns "{root}/{Show}/Season NN/{Show} - SNNENN.mkv".
func EpisodePath(root, show string, season, episode int) string {
	s := Sanitize(show)
	return filepath.Join(
		root,
		s,
		fmt.Sprintf("Season %02d", season),
		fmt.Sprintf("%s - S%02dE%02d.mkv", s, season, episode),
	)
}

