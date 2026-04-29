package xtream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an Xtream Codes player_api.php client.
type Client struct {
	baseURL   string
	username  string
	password  string
	userAgent string
	http      *http.Client
}

func NewClient(baseURL, username, password, userAgent string) *Client {
	return &Client{
		baseURL:   baseURL,
		username:  username,
		password:  password,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) doJSON(ctx context.Context, action string, extras map[string]string, dst any) error {
	u := PlayerAPIURL(c.baseURL, c.username, c.password, action, extras)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("xtream %s: %w", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("xtream %s: %d: %s", action, resp.StatusCode, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", action, err)
	}
	return nil
}

func (c *Client) GetVODCategories(ctx context.Context) ([]Category, error) {
	var out []Category
	return out, c.doJSON(ctx, "get_vod_categories", nil, &out)
}

func (c *Client) GetSeriesCategories(ctx context.Context) ([]Category, error) {
	var out []Category
	return out, c.doJSON(ctx, "get_series_categories", nil, &out)
}

func (c *Client) GetVODStreams(ctx context.Context, categoryID string) ([]VOD, error) {
	var out []VOD
	var extras map[string]string
	if categoryID != "" {
		extras = map[string]string{"category_id": categoryID}
	}
	return out, c.doJSON(ctx, "get_vod_streams", extras, &out)
}

func (c *Client) GetSeries(ctx context.Context, categoryID string) ([]SeriesListing, error) {
	var out []SeriesListing
	var extras map[string]string
	if categoryID != "" {
		extras = map[string]string{"category_id": categoryID}
	}
	return out, c.doJSON(ctx, "get_series", extras, &out)
}

func (c *Client) GetSeriesInfo(ctx context.Context, seriesID int) (*SeriesInfo, error) {
	var out SeriesInfo
	extras := map[string]string{"series_id": fmt.Sprintf("%d", seriesID)}
	return &out, c.doJSON(ctx, "get_series_info", extras, &out)
}
