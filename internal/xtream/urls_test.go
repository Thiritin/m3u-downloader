package xtream

import "testing"

func TestVODStreamURL(t *testing.T) {
	got := VODStreamURL("http://line.example.com", "u", "p", 12345, "mkv")
	want := "http://line.example.com/movie/u/p/12345.mkv"
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestEpisodeStreamURL(t *testing.T) {
	got := EpisodeStreamURL("http://x.com", "u", "p", 999, "ts")
	want := "http://x.com/series/u/p/999.ts"
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestPlayerAPIURL(t *testing.T) {
	got := PlayerAPIURL("http://x.com", "u", "p", "get_vod_categories", nil)
	want := "http://x.com/player_api.php?username=u&password=p&action=get_vod_categories"
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestPlayerAPIURL_NoAction(t *testing.T) {
	got := PlayerAPIURL("http://x.com", "u", "p", "", nil)
	want := "http://x.com/player_api.php?username=u&password=p"
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestPlayerAPIURL_TrailingSlashStripped(t *testing.T) {
	got := PlayerAPIURL("http://x.com/", "u", "p", "", nil)
	want := "http://x.com/player_api.php?username=u&password=p"
	if got != want { t.Errorf("got %q want %q", got, want) }
}

func TestPlayerAPIURL_WithExtras(t *testing.T) {
	got := PlayerAPIURL("http://x.com", "u", "p", "get_vod_streams",
		map[string]string{"category_id": "12"})
	want := "http://x.com/player_api.php?username=u&password=p&action=get_vod_streams&category_id=12"
	if got != want { t.Errorf("got %q want %q", got, want) }
}
