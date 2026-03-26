# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build -o cliamp .

# Test all packages
go test ./...

# Test a specific package
go test ./ui

# Run a single test
go test ./config -run TestLoadSeekLargeStepSec

# Vet
go vet ./...
```

## Architecture

**cliamp** is a terminal music player (Winamp-inspired TUI) written in Go. It uses [Bubbletea](https://github.com/charmbracelet/bubbletea) for the TUI and [Beep](https://github.com/gopxl/beep) for the audio pipeline.

### Startup flow (`main.go`)

1. Load config (`config/`) and parse CLI flags
2. Initialize external providers (Radio, Navidrome, Spotify, YouTube Music, Plex)
3. Resolve CLI arguments to tracks via `resolve/`
4. Create `Player` and `Playlist`, wire them into the Bubbletea `Model`
5. Run the Bubbletea event loop; spawn MPRIS service on Linux

### Key packages

| Package | Role |
|---|---|
| `ui/model.go` | Central Bubbletea `Model` — all keyboard input, focus management, rendering |
| `ui/view.go` | Rendering logic (Lip Gloss styles defined in `ui/styles.go`) |
| `player/` | Audio pipeline: decode → 10-band EQ → volume → spectrum tap → speaker |
| `playlist/` | Track queue, shuffle/repeat modes, `Provider` interface |
| `resolve/` | Converts file paths, URLs, `ytsearch:` prefixes, M3U/PLS files → tracks |
| `external/` | Pluggable media providers: `radio/`, `spotify/`, `ytmusic/`, `navidrome/`, `plex/`, `local/` |
| `internal/browser/` | Reusable list-browser UI component used by providers |
| `internal/resume/` | Persists and restores playback position across sessions |

### Audio pipeline (`player/`)

```
Decoder (decode.go / ffmpeg.go / ytdl.go)
    → Gapless streamer (gapless.go)
    → 10× Biquad EQ bands (eq.go)
    → Volume (volume.go)
    → Spectrum tap (tap.go)  ← feeds visualizer
    → Beep speaker output
```

- External tools: `ffmpeg` (AAC/ALAC/WMA/Opus), `yt-dlp` (YouTube/SoundCloud/Bandcamp)
- Platform device detection: `device_darwin.go` / `device_other.go`

### Adding a new provider

Implement the `playlist.Provider` interface (`playlist/provider.go`), add initialization in `main.go`, and integrate into `ui/model.go` for display and navigation (follow the pattern of existing providers in `external/`).

### Configuration

Runtime config: `~/.config/cliamp/config.toml` (see `config.toml.example` and `docs/configuration.md`).
