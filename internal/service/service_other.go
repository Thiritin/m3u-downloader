//go:build !darwin && !linux

package service

import "errors"

// Install is unsupported on this platform. Only macOS (launchd) and
// Linux (systemd-user) are supported.
func Install() error {
	return errors.New("install-service is only supported on macOS and Linux")
}

// Uninstall is unsupported on this platform.
func Uninstall() error {
	return errors.New("uninstall-service is only supported on macOS and Linux")
}
