# lazydir

![GitHub Release](https://img.shields.io/github/v/release/agntcy/lazydir)
[![CI](https://github.com/agntcy/lazydir/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/agntcy/lazydir/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/agntcy/lazydir)](./LICENSE.md)

A terminal user interface (TUI) for browsing and managing records in [AGNTCY Directory](https://github.com/agntcy/dir) instances — inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker).

## Overview

`lazydir` lets you explore AGNTCY Directory nodes without memorizing `dirctl` commands. It presents the directory's contents in three navigable panels on the left and a live preview panel on the right:

```
┌────────────────────────┬──────────────────────────────────────────────┐
│ [1] Connections        │ [0] Preview                                  │
│  ● Directory: localh…  │                                              │
│  ● OASF: schema.oasf…  │  Syntax-highlighted JSON of the              │
├────────────────────────│  selected record                             │
│ [2] Filters            │                                              │
│  ▶ Skills              │                                              │
│      natural_language… │                                              │
│  ▶ Domains             │                                              │
│  ▶ Modules             │                                              │
│  ▶ OASF version        │                                              │
│  ▶ Version             │                                              │
│  ▶ Author              │                                              │
│  ▶ Trusted / Verified  │                                              │
├────────────────────────│                                              │
│ [3] Records  /filter   │                                              │
│  > cisco.com/agent  v1 │                                              │
│    example.com/bot  v2 │                                              │
│    …                   │                                              │
└────────────────────────┴──────────────────────────────────────────────┘
  navigate: ↑↓  focus: tab  filter records: /  expand: enter  ?: help
```

### Features by panel

**[1] Connections** — live status for Directory and OASF endpoints; switch servers with `c`/`o`; view connection details with `i`.

**[2] Filters** — collapsible categories (Skills, Domains, Modules, OASF version, Version, Author, Trusted / Verified); toggle options with `enter`/`space`; `/` to search across all categories by name, caption, or ID; `i` to open a popup with the OASF class hierarchy and description.

**[3] Records** — filtered list showing name and version; most filters (skills, domains, modules, version, author, OASF version) are applied instantly client-side from a local cache — only Trusted/Verified requires a server round-trip; multi-version records auto-grouped under collapsible headers; `/` for live name filtering; `i` for record info popup (CID, annotations, schema version, created-at); `y` to yank/copy CID or full JSON.

**[0] Preview** — syntax-highlighted JSON of the selected record; scrollable when focused.

## Prerequisites

- **Go 1.26+**
- A running [AGNTCY Directory](https://github.com/agntcy/dir) server (local daemon or remote)

To start a local daemon for testing:

```bash
dirctl daemon start
```

## Installation

### Homebrew

```bash
brew tap agntcy/lazydir https://github.com/agntcy/lazydir
brew install lazydir
```

### Go install

```bash
go install github.com/agntcy/lazydir@latest
```

### Build from source

Using `go build`:

```bash
git clone https://github.com/agntcy/lazydir
cd lazydir
go build -o lazydir .
```

Or using [Task](https://taskfile.dev):

```bash
git clone https://github.com/agntcy/lazydir
cd lazydir
task build        # downloads deps and builds into .bin/lazydir
```

### Development workflow

| Command | Description |
|---------|-------------|
| `task deps` | Download Go module dependencies |
| `task build` | Build the binary into `.bin/lazydir` (incremental) |
| `task run` | Build and immediately start `lazydir` |
| `task fmt` | Format Go source files (`gofmt -s`) |
| `task vet` | Run `go vet` |
| `task lint` | Run `golangci-lint` (must be [installed](https://golangci-lint.run/welcome/install/)) |
| `task check` | Run fmt + vet + lint + build in one step |

## Usage

```bash
lazydir
```

All configuration is read from the config file (see [Configuration](#configuration) below) and environment variables. Run `lazydir --help` for a quick reminder.

### Environment variables

| Variable | Description |
|----------|-------------|
| `DIRECTORY_CLIENT_SERVER_ADDRESS` | Override the Directory server address from config |
| `OASF_SERVER_ADDRESS` | Override the OASF schema server URL from config |
| `DEBUG` | Set to any value to write a `lazydir_debug.log` file |

## Configuration

`lazydir` reads an optional config file from `~/.config/lazydir/config.yml` (or `config.yaml`). The `XDG_CONFIG_HOME` environment variable is respected. See [`config.example.yml`](config.example.yml) for a complete annotated template.

### Theme colors

The TUI uses 10 abstract color slots that default to base16 terminal colors. Each slot can be overridden with a color name, a 256-color index, or a hex true-color value:

```yaml
gui:
  theme:
    color1: "yellow"        # skills, annotations
    color2: "cyan"          # domains, class tree, accents
    color3: "magenta"       # modules, timestamps
    color4: "green"         # connected indicator, OASF version, loading
    color5: "blue"          # version filter, options bar, section headers
    color6: "red"           # disconnected indicator
    color7: "brightRed"     # author filter
    color8: "brightYellow"  # trusted / verified filter
    color9: "brightGreen"   # (available)
    color10: "brightBlack"  # dim/muted text (IDs)
    activeBorderColor: "green"    # focused panel border + cursor
    selectedRowBgColor: "8"       # highlighted row background (256-color)
```

Accepted value formats:

| Format | Example | Applies to |
|--------|---------|------------|
| Color name | `red`, `brightCyan`, `yellow` | all color fields |
| 256-color index | `42`, `208` | all color fields |
| Hex true-color | `#ff8800` | `color1`–`color10` only |

Available color names: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `brightBlack`, `brightRed`, `brightGreen`, `brightYellow`, `brightBlue`, `brightMagenta`, `brightCyan`, `brightWhite`.

### GUI options

```yaml
gui:
  scrollStep: 3           # lines per scroll keypress (default: 3)
  splitRatio: 0.33        # left panel width as fraction of terminal (default: 0.33)
  inputDebounceDelay: 150 # ms before live filter fires (default: 150)
  dimLevel: 0.6           # preview dimming when a popup overlays it (0 = off, 1 = invisible)
```

### Server defaults

Config-file defaults for server addresses and timeouts. CLI flags and environment variables still take precedence. Multiple predefined servers can be listed; the first entry is used as the default, and all entries appear in the in-app server selection popup.

```yaml
server:
  directoryServers:
    - address: "localhost:8888"
    - address: "dir.example.com:443"
      oidcIssuer: "https://auth.example.com"
      oidcClientID: "lazydir"
  oasfServers:
    - "https://schema.oasf.outshift.com"
  oasfTimeout: 10  # seconds for OASF HTTP requests (default: 10)
```

Servers with `oidcIssuer` and `oidcClientID` trigger an OIDC device-flow login when no cached token is available. The TUI displays the authorization URL and code inline.

### Stream tuning

Controls how records are batched when streaming from the directory.

```yaml
stream:
  firstPageSize: 100  # records in the initial batch (default: 100)
  batchSize: 50       # records per subsequent batch (default: 50)
```

## Architecture

```
lazydir/
├── main.go                        # Entry point; flag parsing; config loading
├── go.mod / go.sum
├── internal/
│   ├── config/
│   │   └── config.go              # Config file loading; color name resolution
│   ├── gui/
│   │   ├── gui.go                 # Top-level Gui struct; gocui init; async helpers
│   │   ├── theme.go               # Color palette (Theme); defaults; config integration
│   │   ├── layout.go              # Panel layout; frame drawing; status bar
│   │   ├── views.go               # Render functions for filters, records, and preview
│   │   ├── keybindings.go         # Key handlers; focus cycling; panel actions
│   │   ├── filters.go             # Filter state; category aggregation; query building
│   │   └── hints.go               # Options-bar and help-popup text generation
│   ├── dirclient/
│   │   └── wrapper.go             # Thin wrapper around github.com/agntcy/dir/client
│   └── oasf/
│       └── fetch.go               # HTTP fetch of OASF class descriptions; in-memory cache
```

### Data sources

| Data | Source |
|------|--------|
| Record list | `SearchRecords` gRPC call via `github.com/agntcy/dir/client` |
| Record JSON | `Pull` gRPC call (by CID) |
| Record info | `Pull` gRPC call (by CID), decoded to extract metadata |
| Skill / Domain / Module descriptions | OASF SDK schema client via `oasf-sdk/pkg/schema` |

### Technology

- **[gocui](https://github.com/jesseduffield/gocui)** — Terminal UI library (jesseduffield fork, as used by lazygit)
- **[Chroma v2](https://github.com/alecthomas/chroma)** — JSON syntax highlighting
- **[agntcy/dir client](https://github.com/agntcy/dir)** — gRPC client for Directory API

## Contributing

Pull requests and issues are welcome. Please open an issue first to discuss significant changes.

## License

Apache-2.0 — see [LICENSE](LICENSE.md).
