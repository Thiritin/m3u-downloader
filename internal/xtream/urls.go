// Package xtream provides a typed client for the Xtream Codes player API.
// urls.go contains pure URL builders.
package xtream

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func base(baseURL string) string {
	return strings.TrimRight(baseURL, "/")
}

// VODStreamURL returns the direct download URL for a movie.
func VODStreamURL(baseURL, user, pass string, streamID int, ext string) string {
	return fmt.Sprintf("%s/movie/%s/%s/%d.%s", base(baseURL), user, pass, streamID, ext)
}

// EpisodeStreamURL returns the direct download URL for a series episode.
func EpisodeStreamURL(baseURL, user, pass string, episodeID int, ext string) string {
	return fmt.Sprintf("%s/series/%s/%s/%d.%s", base(baseURL), user, pass, episodeID, ext)
}

// PlayerAPIURL builds a player_api.php URL with optional action and extras.
// Extras (e.g. category_id, series_id) are appended in sorted-key order so
// the result is deterministic for tests. All values are URL-escaped.
func PlayerAPIURL(baseURL, user, pass, action string, extras map[string]string) string {
	parts := []string{
		"username=" + url.QueryEscape(user),
		"password=" + url.QueryEscape(pass),
	}
	if action != "" {
		parts = append(parts, "action="+url.QueryEscape(action))
	}
	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(extras[k]))
	}
	return base(baseURL) + "/player_api.php?" + strings.Join(parts, "&")
}
