//go:build !darwin

package service

import "errors"

// Install is unsupported on non-macOS platforms. Use systemd (Linux) or
// Task Scheduler (Windows) to keep `m3u-dl worker` running at startup.
func Install() error {
	return errors.New("install-service is currently macOS only; on Linux configure a systemd user unit, on Windows use Task Scheduler")
}

// Uninstall is unsupported on non-macOS platforms.
func Uninstall() error {
	return errors.New("uninstall-service is currently macOS only")
}
