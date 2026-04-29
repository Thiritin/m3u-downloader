package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[provider]
base_url = "http://example.com"
username = "u"
password = "p"
user_agent = "LimePlayer"

[output]
movies_dir = "/tmp/movies"
series_dir = "/tmp/tv"

[downloader]
remux = true
max_retries = 3
backoff_seconds = [5, 30, 120]
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Provider.BaseURL != "http://example.com" {
		t.Errorf("base_url = %q", c.Provider.BaseURL)
	}
	if c.Provider.UserAgent != "LimePlayer" {
		t.Errorf("user_agent = %q", c.Provider.UserAgent)
	}
	if !c.Downloader.Remux {
		t.Error("remux should be true")
	}
	if got := c.Downloader.BackoffSeconds; len(got) != 3 || got[2] != 120 {
		t.Errorf("backoff_seconds = %v", got)
	}
}

func TestValidate_MissingBaseURL(t *testing.T) {
	c := &Config{}
	c.Output.MoviesDir = "/m"
	c.Output.SeriesDir = "/s"
	c.Provider.Username = "u"
	c.Provider.Password = "p"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for missing base_url")
	}
}

func TestValidate_DefaultsApplied(t *testing.T) {
	c := &Config{}
	c.Provider.BaseURL = "http://x"
	c.Provider.Username = "u"
	c.Provider.Password = "p"
	c.Output.MoviesDir = "/m"
	c.Output.SeriesDir = "/s"
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.Provider.UserAgent != "LimePlayer" {
		t.Errorf("default user_agent: %q", c.Provider.UserAgent)
	}
	if c.Downloader.MaxRetries != 3 {
		t.Errorf("default max_retries: %d", c.Downloader.MaxRetries)
	}
	if len(c.Downloader.BackoffSeconds) != 3 {
		t.Errorf("default backoff: %v", c.Downloader.BackoffSeconds)
	}
}
