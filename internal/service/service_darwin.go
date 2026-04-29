//go:build darwin

// Package service installs and uninstalls the macOS launchd user agent
// that runs `m3u-dl worker`. On Linux and Windows the Install/Uninstall
// functions return an "unsupported" error — wire systemd or Task Scheduler
// yourself for those platforms.
package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// xmlEscape returns s safe for embedding inside a plist <string> element.
func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const label = "com.user.m3u-dl"

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

func plistFor(binPath, logPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyLists-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>worker</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
  </dict>
</dict>
</plist>
`, label, xmlEscape(binPath), xmlEscape(logPath), xmlEscape(logPath))
}

func Install() error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	bin, _ = filepath.Abs(bin)
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "Library", "Logs", "m3u-dl.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}

	plistFile, err := plistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistFile), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistFile, []byte(plistFor(bin, logPath)), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", plistFile).Run() // ignore errors — may not be loaded
	if out, err := exec.Command("launchctl", "load", plistFile).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, string(out))
	}
	fmt.Printf("Installed and started: %s\n", plistFile)
	return nil
}

func Uninstall() error {
	plistFile, err := plistPath()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", plistFile).Run()
	if err := os.Remove(plistFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("Uninstalled: %s\n", plistFile)
	return nil
}
