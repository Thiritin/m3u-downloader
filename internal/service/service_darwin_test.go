//go:build darwin

package service

import (
	"strings"
	"testing"
)

func TestPlistContents(t *testing.T) {
	got := plistFor("/usr/local/bin/m3u-dl", "/Users/me/Library/Logs/m3u-dl.log")
	for _, want := range []string{
		"<key>Label</key>",
		"com.user.m3u-dl",
		"/usr/local/bin/m3u-dl",
		"<string>worker</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"/Users/me/Library/Logs/m3u-dl.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q\n%s", want, got)
		}
	}
}
