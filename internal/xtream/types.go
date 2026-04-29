package xtream

// Category is shared across live/vod/series — only `type` differs.
type Category struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	ParentID     int    `json:"parent_id"`
}

// VOD represents a movie entry from get_vod_streams.
// Rating stays a string because the provider returns "" for missing values,
// which json.Number rejects.
type VOD struct {
	StreamID           int    `json:"stream_id"`
	Name               string `json:"name"`
	StreamIcon         string `json:"stream_icon"`
	Rating             string `json:"rating"`
	Year               string `json:"year"`
	Added              string `json:"added"`
	CategoryID         string `json:"category_id"`
	ContainerExtension string `json:"container_extension"`
	Plot               string `json:"plot"`
}

// SeriesListing is what get_series returns: the show, not its seasons.
type SeriesListing struct {
	SeriesID    int      `json:"series_id"`
	Name        string   `json:"name"`
	Cover       string   `json:"cover"`
	Plot        string   `json:"plot"`
	ReleaseDate string   `json:"releaseDate"`
	CategoryID  string   `json:"category_id"`
	Backdrop    []string `json:"backdrop_path"`
}

// SeriesInfo is the get_series_info response: seasons + episodes.
type SeriesInfo struct {
	Info struct {
		Name     string   `json:"name"`
		Cover    string   `json:"cover"`
		Plot     string   `json:"plot"`
		Backdrop []string `json:"backdrop_path"`
	} `json:"info"`
	Seasons []struct {
		SeasonNumber int    `json:"season_number"`
		Name         string `json:"name"`
		Overview     string `json:"overview"`
		Cover        string `json:"cover"`
	} `json:"seasons"`
	// Episodes is keyed by season number as a string.
	Episodes map[string][]Episode `json:"episodes"`
}

// Episode is one entry in SeriesInfo.Episodes[season].
type Episode struct {
	ID                 string `json:"id"`
	EpisodeNum         int    `json:"episode_num"`
	Title              string `json:"title"`
	ContainerExtension string `json:"container_extension"`
	Info               struct {
		Plot     string `json:"plot"`
		Duration string `json:"duration"`
	} `json:"info"`
	Season int `json:"season"`
}
