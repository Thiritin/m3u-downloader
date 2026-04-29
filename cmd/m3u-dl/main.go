package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/Thiritin/m3u-downloader/internal/catalog"
	"github.com/Thiritin/m3u-downloader/internal/config"
	"github.com/Thiritin/m3u-downloader/internal/downloader"
	"github.com/Thiritin/m3u-downloader/internal/service"
	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/tui"
	"github.com/Thiritin/m3u-downloader/internal/worker"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

const usage = `m3u-dl - personal IPTV catalog browser & downloader

Subcommands:
  tui                  Interactive browser & queue manager
  worker               Long-running download worker (launchd target)
  sync                 Force a full catalog refresh
  config               Print active config (with secrets redacted)
  install-service      Install launchd agent
  uninstall-service    Remove launchd agent
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	switch cmd {
	case "tui":
		exit(runTUI(args))
	case "worker":
		exit(runWorker(args))
	case "sync":
		exit(runSync(args))
	case "config":
		exit(runConfig(args))
	case "install-service":
		exit(runInstallService(args))
	case "uninstall-service":
		exit(runUninstallService(args))
	case "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n%s", cmd, usage)
		os.Exit(2)
	}
}

func exit(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// dataDirPath returns the platform-appropriate state directory.
//
//   - Windows:       %LOCALAPPDATA%/m3u-dl
//   - macOS / Linux: $HOME/.local/share/m3u-dl
func dataDirPath() (string, error) {
	if runtime.GOOS == "windows" {
		dir, err := os.UserCacheDir() // returns %LOCALAPPDATA% on Windows
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "m3u-dl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "m3u-dl"), nil
}

func loadAll() (*config.Config, *store.Store, *xtream.Client, error) {
	cfgPath, err := config.DefaultPath()
	if err != nil {
		return nil, nil, nil, err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, nil, err
	}
	dataDir, err := dataDirPath()
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := store.Open(filepath.Join(dataDir, "state.db"))
	if err != nil {
		return nil, nil, nil, err
	}
	xc := xtream.NewClient(cfg.Provider.BaseURL, cfg.Provider.Username, cfg.Provider.Password, cfg.Provider.UserAgent)
	return cfg, st, xc, nil
}

func runTUI(_ []string) error {
	cfg, st, xc, err := loadAll()
	if err != nil {
		return err
	}
	defer st.Close()
	return tui.New(st, xc, cfg.Output.MoviesDir, cfg.Output.SeriesDir).Run()
}

func runWorker(_ []string) error {
	cfg, st, _, err := loadAll()
	if err != nil {
		return err
	}
	defer st.Close()

	// When run interactively (a TTY on stderr) write logs to the terminal so
	// the user sees what the worker is doing. Under launchd there's no TTY,
	// so we fall back to the on-disk log file (also captured via plist).
	var logger *slog.Logger
	if isStderrTTY() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logFile, _ := openLogFile()
		logger = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	w := &worker.Worker{
		Store:          st,
		UserAgent:      cfg.Provider.UserAgent,
		MoviesDir:      cfg.Output.MoviesDir,
		SeriesDir:      cfg.Output.SeriesDir,
		Remux:          cfg.Downloader.Remux,
		MaxRetries:     cfg.Downloader.MaxRetries,
		BackoffSeconds: cfg.Downloader.BackoffSeconds,
		Logger:         logger,
		ResolveURL: func(j store.JobRow) (string, error) {
			ext := lookupExt(st, j)
			switch j.Kind {
			case "vod":
				return xtream.VODStreamURL(cfg.Provider.BaseURL, cfg.Provider.Username, cfg.Provider.Password, j.SourceID, ext), nil
			case "episode":
				return xtream.EpisodeStreamURL(cfg.Provider.BaseURL, cfg.Provider.Username, cfg.Provider.Password, j.SourceID, ext), nil
			}
			return "", fmt.Errorf("unknown job kind: %s", j.Kind)
		},
		Sidecars: makeSidecarFn(st, cfg.Provider.UserAgent),
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("worker starting", "movies", cfg.Output.MoviesDir, "series", cfg.Output.SeriesDir)
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func lookupExt(st *store.Store, j store.JobRow) string {
	ctx := context.Background()
	switch j.Kind {
	case "vod":
		row := st.DB().QueryRowContext(ctx,
			`SELECT COALESCE(container_extension,'mkv') FROM vods WHERE stream_id=?`, j.SourceID)
		var ext string
		_ = row.Scan(&ext)
		if ext == "" {
			ext = "mkv"
		}
		return ext
	case "episode":
		row := st.DB().QueryRowContext(ctx,
			`SELECT COALESCE(container_extension,'mkv') FROM episodes WHERE episode_id=?`, j.SourceID)
		var ext string
		_ = row.Scan(&ext)
		if ext == "" {
			ext = "mkv"
		}
		return ext
	}
	return "mkv"
}

func makeSidecarFn(st *store.Store, ua string) worker.SidecarFn {
	d := &downloader.Downloader{UserAgent: ua}
	return func(ctx context.Context, j store.JobRow) error {
		folder := filepath.Dir(j.DestPath)
		switch j.Kind {
		case "vod":
			row := st.DB().QueryRowContext(ctx, `SELECT COALESCE(stream_icon_url,'') FROM vods WHERE stream_id=?`, j.SourceID)
			var icon string
			_ = row.Scan(&icon)
			if icon != "" {
				_ = d.Download(ctx, icon, filepath.Join(folder, "poster.jpg"), nil)
			}
		case "episode":
			showFolder := filepath.Dir(folder) // one level up from Season NN
			row := st.DB().QueryRowContext(ctx, `
				SELECT COALESCE(s.cover_url,''), COALESCE(s.backdrop_url,'')
				FROM episodes e JOIN series s ON s.series_id=e.series_id
				WHERE e.episode_id=?`, j.SourceID)
			var cover, backdrop string
			_ = row.Scan(&cover, &backdrop)
			if cover != "" {
				_ = d.Download(ctx, cover, filepath.Join(showFolder, "poster.jpg"), nil)
			}
			if backdrop != "" {
				_ = d.Download(ctx, backdrop, filepath.Join(showFolder, "fanart.jpg"), nil)
			}
		}
		return nil
	}
}

func openLogFile() (*os.File, error) {
	logPath, err := logFilePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// logFilePath returns the platform-appropriate worker log location.
//   - macOS:   $HOME/Library/Logs/m3u-dl.log     (Console.app picks it up)
//   - Linux:   $HOME/.local/state/m3u-dl/m3u-dl.log
//   - Windows: %LOCALAPPDATA%/m3u-dl/m3u-dl.log
func logFilePath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", "m3u-dl.log"), nil
	case "windows":
		dir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "m3u-dl", "m3u-dl.log"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "state", "m3u-dl", "m3u-dl.log"), nil
	}
}

func isStderrTTY() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func runSync(_ []string) error {
	_, st, xc, err := loadAll()
	if err != nil {
		return err
	}
	defer st.Close()
	return catalog.FullSync(context.Background(), st, xc, func(stage string, count int) {
		if count > 0 {
			fmt.Printf("[sync] %s (%d)\n", stage, count)
		} else {
			fmt.Printf("[sync] %s\n", stage)
		}
	})
}

func runConfig(_ []string) error {
	cfg, _, _, err := loadAll()
	if err != nil {
		return err
	}
	fmt.Printf("provider.base_url   = %s\n", cfg.Provider.BaseURL)
	fmt.Printf("provider.username   = %s\n", cfg.Provider.Username)
	fmt.Printf("provider.password   = %s\n", strings.Repeat("*", len(cfg.Provider.Password)))
	fmt.Printf("provider.user_agent = %s\n", cfg.Provider.UserAgent)
	fmt.Printf("output.movies_dir   = %s\n", cfg.Output.MoviesDir)
	fmt.Printf("output.series_dir   = %s\n", cfg.Output.SeriesDir)
	return nil
}

func runInstallService(_ []string) error   { return service.Install() }
func runUninstallService(_ []string) error { return service.Uninstall() }
