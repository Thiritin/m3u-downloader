# m3u-dl

TUI for browsing an Xtream Codes / IPTV catalog and downloading VOD and
series into a Plex-compatible library.

![demo](demo.gif)

- Browse the catalog by category, with live preview as you scroll.
- Search across the whole catalog (movies + shows) once it's indexed.
- Queue movies, full shows, single seasons, or single episodes.
- A worker pulls jobs one at a time, respects the provider's connection
  cap, resumes via HTTP `Range`, remuxes to MKV with `ffmpeg -c copy`,
  and writes a Plex-canonical layout on disk.
- Runs on macOS, Linux, and Windows. Single static binary, no CGO.

One binary, several subcommands: `m3u-dl tui`, `m3u-dl worker`,
`m3u-dl sync`, `m3u-dl config`, `m3u-dl install-service` (macOS).

## Requirements

- macOS (arm64/amd64), Linux (amd64/arm64), or Windows (amd64).
- `ffmpeg` on `PATH`.
  - macOS: `brew install ffmpeg`
  - Debian/Ubuntu: `sudo apt install ffmpeg`
  - Windows: `choco install ffmpeg`

## Install

Grab a binary from [Releases](https://github.com/Thiritin/m3u-downloader/releases),
or build:

```bash
git clone https://github.com/Thiritin/m3u-downloader
cd m3u-downloader
make build
```

## Configure

Copy `config.example.toml` to your config path:

| OS            | Config                            | State                              |
|---------------|-----------------------------------|------------------------------------|
| macOS / Linux | `~/.config/m3u-dl/config.toml`    | `~/.local/share/m3u-dl/state.db`   |
| Windows       | `%APPDATA%\m3u-dl\config.toml`    | `%LOCALAPPDATA%\m3u-dl\state.db`   |

```toml
[provider]
base_url   = "http://your-provider.example.com"
username   = "YOUR_USERNAME"
password   = "YOUR_PASSWORD"
user_agent = "LimePlayer"

[output]
movies_dir = "/path/to/Movies"
series_dir = "/path/to/TV Shows"

[downloader]
remux           = true
max_retries     = 3
backoff_seconds = [5, 30, 120]
```

`movies_dir` and `series_dir` must exist when the worker runs — that's the
unmounted-volume safety check. Verify with `./m3u-dl config`.

## Use

```bash
./m3u-dl tui               # browse + queue
./m3u-dl worker            # process the queue (logs to stderr)
./m3u-dl sync              # force a full catalog refresh
./m3u-dl install-service   # launchd agent (macOS only)
./m3u-dl uninstall-service # remove launchd agent (macOS only)
```

On Linux, run the worker via a systemd user unit. On Windows, register a
Task Scheduler task that runs `m3u-dl.exe worker` at logon.

### TUI keys

Global: `b` browse, `s` search, `q` queue, `ctrl+c` quit.

Browse: `↑↓` move, `enter` drill in, `esc/backspace` drill out, `space`
queue selected, `r` refresh category, `/` filter.

Queue: `r` retry failed, `d` (or `x`) cancel active or remove row.

Items already in the queue show a badge: `[Q]` queued · `[↓]` downloading
· `[✓]` completed · `[✗]` failed.

## On-disk layout

```
{movies_dir}/Movie Title (Year)/
├── Movie Title (Year).mkv
├── poster.jpg
└── fanart.jpg

{series_dir}/Show Title/
├── poster.jpg
├── fanart.jpg
└── Season 01/
    ├── Show Title - S01E01.mkv
    └── ...
```

Plex's default agents pick this up.

## Architecture

Single binary. TUI and worker share a SQLite database (WAL) — TUI writes
jobs, worker polls `pending` every two seconds. The catalog is mirrored
locally so search can fuzzy-filter ~100k titles instantly. Worker holds at
most one HTTP connection at a time because providers cap simultaneous
streams. Modules under `internal/` each do one thing — `plex`, `xtream`,
`store`, `downloader`, `remux`, `worker`, `tui`, `service`, `catalog`,
`config`.
