# lazydir

A terminal user interface (TUI) for browsing and managing records in [AGNTCY Directory](https://github.com/agntcy/dir) instances — inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker).

## Overview

`lazydir` lets you explore AGNTCY Directory nodes without memorizing `dirctl` commands. It presents the directory's contents in three navigable panels on the left side and a live preview panel on the right:

```
┌────────────────────────┬──────────────────────────────────────────────┐
│ [1] Connections        │ [Preview]                                    │
│  ● Directory: localh…  │                                              │
│  ● OASF: schema.oasf…  │  Shows either:                               │
├────────────────────────│  • OASF skill / domain / module description  │
│ [2] Filters            │  • Full record JSON (syntax highlighted)     │
│  ▶ Skills              │                                              │
│      natural_language… │                                              │
│  ▶ Domains             │                                              │
│  ▶ Modules             │                                              │
│  ▶ OASF version        │                                              │
│  ▶ Version             │                                              │
│  ▶ Author              │                                              │
│  ▶ Trusted             │                                              │
│  ▶ Verified            │                                              │
├────────────────────────│                                              │
│ [3] Records  /filter   │                                              │
│  > cisco.com/agent  v1 │                                              │
│    example.com/bot  v2 │                                              │
│    …                   │                                              │
└────────────────────────┴──────────────────────────────────────────────┘
  navigate: ↑↓  focus: tab  filter records: /  expand: enter
```

### Panel descriptions

| Panel | Purpose |
|-------|---------|
| **[1] Connections** | Shows both endpoints the TUI is currently talking to — the Directory server and the OASF schema server — along with the connection status of the former. Press `c` to switch to a different Directory server and `o` to point at a different OASF schema server. |
| **[2] Filters** | Lists every filter category as a collapsible tree (Skills, Domains, Modules, OASF version, Version, Author, Trusted, Verified). Each category has a `▶`/`▼` triangle — pressing `enter` on a category expands or collapses its options dropdown; pressing `enter` or `space` on an option toggles its selection. Skills, domains, and modules display their OASF numeric ID and caption (fetched from the OASF schema server) sorted by ID. Selected options are shown in a distinct color and remain visible under their category even when collapsed. Multiple values can be active per category. The `[3] Records` pane updates immediately as filters change. Press `/` to search across all non-boolean categories simultaneously — matching options appear grouped under their category headers as you type; the search matches against name, caption, and ID. Press `i` on a skill, domain, or module option to toggle its OASF class hierarchy tree and description inline. |
| **[3] Records** | Lists records that satisfy the active filters. Shows name and version. Use `/` to filter by name — results narrow live as you type. Press `enter` to load the full record JSON in the preview panel. Press `i` to toggle inline record info (CID, annotations, schema version, created-at) below the selected record. Press `y` to open a yank/copy menu where `c` copies the CID and `a` copies the full record JSON to the clipboard. |
| **Preview** | The right two-thirds of the screen. Displays syntax-highlighted JSON of the selected record. Scroll with `↑`/`↓` when the preview panel is focused. |

## Prerequisites

- **Go 1.26+**
- A running [AGNTCY Directory](https://github.com/agntcy/dir) server (local daemon or remote)

To start a local daemon for testing:

```bash
dirctl daemon start
```

## Installation

```bash
go install github.com/akijakya/lazydir@latest
```

Or build from source using `go build`:

```bash
git clone https://github.com/akijakya/lazydir
cd lazydir
go build -o lazydir .
```

Or using [Task](https://taskfile.dev):

```bash
git clone https://github.com/akijakya/lazydir
cd lazydir
task build        # downloads deps and builds into .bin/lazydir
```

### Development workflow

| Command | Description |
|---------|-------------|
| `task deps` | Download Go module dependencies |
| `task build` | Build the binary into `.bin/lazydir` (incremental) |
| `task run` | Build and immediately start `lazydir` |
| `task run CLI_ARGS="--server-addr localhost:8888"` | Build and start with custom flags |
| `task fmt` | Format Go source files (`gofmt -s`) |
| `task vet` | Run `go vet` |
| `task lint` | Run `golangci-lint` (must be [installed](https://golangci-lint.run/welcome/install/)) |
| `task check` | Run fmt + vet + lint + build in one step |

## Usage

```bash
lazydir [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server-addr`, `-s` | `localhost:8888` | Directory server address |
| `--oasf-addr`, `-o` | `https://schema.oasf.outshift.com` | OASF schema server URL (used via `oasf-sdk`) |
| `--auth-mode`, `-a` | _(auto-detect)_ | Auth mode: `insecure`, `tls`, `oidc`, `jwt`, `x509` |
| `--auth-token` | | Pre-issued Bearer token (for CI / non-interactive) |
| `--tls-ca-file` | | TLS CA certificate file path |
| `--tls-cert-file` | | TLS client certificate file path |
| `--tls-key-file` | | TLS client key file path |
| `--tls-skip-verify` | `false` | Skip TLS certificate verification |
| `--help`, `-h` | | Show usage |

### Environment variables

| Variable | Description |
|----------|-------------|
| `DIRECTORY_CLIENT_SERVER_ADDRESS` | Default Directory server address (overridden by `--server-addr`) |
| `OASF_SERVER_ADDRESS` | Default OASF schema server URL (overridden by `--oasf-addr`) |
| `DEBUG` | Set to any value to write a `lazydir_debug.log` file |

### Examples

```bash
# Connect to a local insecure server
lazydir --server-addr localhost:8888

# Connect with a pre-issued token
lazydir --server-addr my-dir.example.com:443 --auth-token "eyJ..."

# Connect using TLS certificates
lazydir -s my-dir.example.com:443 \
  --tls-ca-file /path/to/ca.pem \
  --tls-cert-file /path/to/client.crt \
  --tls-key-file /path/to/client.key
```

## Key Bindings

| Key | Action |
|-----|--------|
| `q` / `ctrl+c` | Quit |
| `tab` / `shift+tab` | Cycle panel focus |
| `1` | Focus the Connections panel |
| `2` | Focus the Filters panel |
| `3` | Focus the Records panel |
| `0` | Focus the Preview panel |
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `enter` (Filters, on category) | Expand / collapse the category dropdown |
| `enter` / `space` (Filters, on option) | Toggle filter selection |
| `/` (Filters) | Search options across all categories (name, caption, ID) |
| `i` (Filters, on option) | Toggle inline OASF class hierarchy and description |
| `esc` (Filters) | Clear search query |
| `enter` (Records) | Load the full record JSON in the preview panel |
| `i` (Records) | Toggle inline record info (CID, annotations, schema version, created-at) |
| `y` (Records) | Open yank/copy menu — `c` copies the CID, `a` copies the full record JSON |
| `/` (Records) | Live-filter by name |
| `esc` (Records) | Clear name filter |
| `c` (Connections panel) | Open Directory connect dialog |
| `o` (Connections panel) | Open OASF server connect dialog |
| `r` | Refresh records from server |
| `?` | Show the full keybinding popup for the focused panel |
| `wheel` | Scroll (list and preview panels) |

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
    color8: "brightYellow"  # trusted filter
    color9: "brightGreen"   # verified filter
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

Apache-2.0 — see [LICENSE](LICENSE).
