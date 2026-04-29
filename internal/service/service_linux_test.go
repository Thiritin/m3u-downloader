//go:build linux

package service

import (
	"strings"
	"testing"
)

func TestUnitContents(t *testing.T) {
	got := unitFor("/usr/local/bin/m3u-dl")
	for _, want := range []string{
		"[Unit]",
		"Description=m3u-dl download worker",
		"After=network-online.target",
		"[Service]",
		"ExecStart=/usr/local/bin/m3u-dl worker",
		"Restart=on-failure",
		"[Install]",
		"WantedBy=default.target",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit missing %q\n%s", want, got)
		}
	}
}

func TestUnitPathRespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdgconfig")
	got, err := userUnitPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/xdgconfig/systemd/user/m3u-dl.service"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestUnitPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err := userUnitPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/fakehome/.config/systemd/user/m3u-dl.service"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
