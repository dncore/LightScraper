# LightScraper

LightScraper is a local-first desktop game library manager for retro game
collectors. This repository currently contains the **LS-T001 desktop
foundation** (Wails + React + TypeScript, minimal Go bridge, background event
demo, initialized SQLite), the **LS-T002 standalone ROM scanner** (read-only,
in-memory Batocera directory scanning), and the **LS-T003-A gamelist.xml core**
(strict in-memory parsing, source-preserving views and local edits; no disk
writes yet).

`gamelist.xml` persistence, metadata providers, media download, and the full
product UI are not implemented yet. See
[`docs/rom-scanner.md`](docs/rom-scanner.md) for the scanner boundary and
[`docs/gamelist-compatibility.md`](docs/gamelist-compatibility.md) for the
gamelist boundary.

## Stack

| Component | Choice |
| --- | --- |
| Desktop shell | Wails v2.17.1 |
| Backend | Go 1.26+ (tested with Go 1.27.0) |
| Frontend | React 19 + TypeScript 5.9 + Vite 7 |
| Database | SQLite via `modernc.org/sqlite` v1.61.1 (pure Go, no CGO) |
| Logging | Go standard library `log/slog` |

### Why Wails v2 instead of v3

Wails v3.1.0 is a stable release, but v3.0.0 was published on 2026-10-26 and v3
follows a weekly minor-release cadence. Wails v2.17.1 is the current maintained
v2 release, has a mature application/event/binding API, and works with the
GTK3 + WebKit2GTK 4.1 stack available on the development machine. The
repository pins the CLI and `go.mod` version instead of using `@latest`.

The project can revisit Wails v3 later if its API and release cadence are a
good fit. That would be a separate technical decision, not an implicit upgrade.

### Why `modernc.org/sqlite`

`modernc.org/sqlite` is a pure-Go SQLite implementation. It avoids adding a
second CGO dependency beyond Wails itself and keeps Go tests and cross-platform
builds independent of a C SQLite toolchain. The alternative,
`github.com/mattn/go-sqlite3`, requires CGO and a platform C compiler and makes
cross-compilation and static packaging less predictable.

The migration runner is intentionally small: each embedded SQL file is
applied in a transaction and recorded in `schema_migrations`. LS-T001 creates
only that metadata table; business schemas will be added by later tasks.

## Repository layout

```text
.
├── main.go                         Wails entry point and dependency wiring
├── assets_production.go            Embeds frontend/dist for production/debug builds
├── assets_dev.go                   Empty asset FS for dev/test builds
├── internal/app/                   Bound service, bridge calls, heartbeat event loop
├── internal/config/                Environment/default configuration
├── internal/database/              SQLite connection pool and migration runner
│   └── migrations/                 Embedded, ordered SQL migrations
├── internal/logging/               slog setup
├── internal/version/               Build-time name/version/commit metadata
├── internal/library/scanner/       Read-only Batocera ROM directory scanner
├── internal/library/gamelist/      Source-preserving gamelist.xml parser/editor (T003-A)
├── docs/rom-scanner.md             Scanner semantics, evidence, and limitations
├── docs/gamelist-compatibility.md  gamelist XML/namespace/path preservation boundary
├── frontend/                       React + TypeScript source and Wails bindings
├── build/                          Wails platform metadata and app icons
├── .github/workflows/ci.yml        Tests, formatting, and platform build matrix
└── wails.json                      Wails v2 project configuration
```

The Go application is a modular monolith. There is no HTTP server, provider
layer, task queue, or business repository yet.

## Prerequisites

Install the pinned Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
```

Make sure `$(go env GOPATH)/bin` is on `PATH`.

### Linux

Wails v2 needs GTK3 and WebKitGTK development headers. This project uses the
WebKit2GTK 4.1 build tag (`webkit2_41`, configured in `wails.json`).

Arch Linux:

```bash
sudo pacman -S --needed base-devel gtk3 webkit2gtk-4.1
```

Ubuntu/Debian:

```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

If a distribution only provides WebKit2GTK 4.0, remove `build:tags` from
`wails.json` and use Wails' default 4.0 backend. This configuration has not
been tested with 4.0.

### macOS

Install Xcode Command Line Tools. Wails uses the system WebKit view; no
additional webview package is required.

### Windows

Install WebView2 (included with current Windows 10/11) and the Go toolchain.
NSIS is only needed if you create an installer.

### Frontend

Node.js 20.19+ or 22+ and npm. The checked-in `package-lock.json` is the
source of truth.

## Development

From a clean checkout:

```bash
npm ci --prefix frontend
WAILS_DEV_ALLOWED_HOSTS=wails.localhost wails dev
```

`wails dev` starts the Vite dev server and the desktop window. The first build
can take a few minutes while Go modules compile.

`WAILS_DEV_ALLOWED_HOSTS` is needed with Wails v2.17.1: its development asset
proxy uses the `wails.localhost` hostname, and the release's DNS-rebinding
guard otherwise rejects that one request. The setting only trusts that
hostname for the local dev server; it does not affect production builds and
must not be widened to remote hosts. On Windows PowerShell, set it with
`$env:WAILS_DEV_ALLOWED_HOSTS = 'wails.localhost'` before running `wails dev`.

Press `Ctrl+C` to stop the process.

## Tests and checks

```bash
# Backend tests (the webkit2_41 tag is required for the Wails main package)
go test -tags webkit2_41 ./...
go vet -tags webkit2_41 ./...

# Frontend
npm --prefix frontend run typecheck
npm --prefix frontend run build
npm --prefix frontend run format:check

# Go formatting
test -z "$(gofmt -l main.go internal)"
```

The database tests use temporary directories and do not touch the user data
directory. Go tests and development builds use an empty asset filesystem, so
they do not require `frontend/dist`; only production/debug Wails builds embed
the Vite output.

## Build

```bash
wails build -clean
```

The output is written to `build/bin/`. The filename and platform packaging are
controlled by `wails.json` and Wails' platform metadata.

To stamp version metadata into the binary:

```bash
VERSION=0.1.0
COMMIT=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

wails build -clean \
  -ldflags "-X github.com/dncore/lightscraper/internal/version.Version=${VERSION} \
            -X github.com/dncore/lightscraper/internal/version.Commit=${COMMIT} \
            -X github.com/dncore/lightscraper/internal/version.BuildDate=${BUILD_DATE}"
```

## Configuration and local data

- Default data directory: the platform user config directory plus
  `LightScraper` (for example `~/.config/LightScraper` on Linux).
- `LIGHTSCRAPER_DATA_DIR` overrides the data directory. Use this for a portable
  or test installation.
- `LIGHTSCRAPER_LOG_LEVEL` accepts `debug`, `info`, `warn`, or `error`; the
  default is `info`.
- The SQLite database is `lightscraper.db` inside the data directory. Database
  files, WAL files, and local environment files are git-ignored.

The application does not intentionally log database paths, environment
contents, API keys, or user data. No real ROMs, credentials, or databases
belong in this repository.

## ROM library scanner

LS-T002 provides a standalone read-only scanner in
`internal/library/scanner`. It reads Batocera `_info.txt` extension metadata,
finds matching regular files recursively, and returns in-memory results with
structured issues. It is not wired into the UI or database.

See [`docs/rom-scanner.md`](docs/rom-scanner.md) for the exact input semantics,
official `_info.txt` evidence, path/symlink policy, error model, and known
limitations.

## gamelist.xml core

LS-T003-A adds `internal/library/gamelist`, a strict UTF-8 parser and
source-span editor for Batocera/EmulationStation `gamelist.xml`. It preserves
the original bytes, including BOM, namespaces, comments, CDATA, unknown nodes
and attributes, duplicates and formatting; field edits are applied by byte
splice in memory. It only reads `<systemDir>/gamelist.xml` and deliberately has
no Save/write path yet.

See [`docs/gamelist-compatibility.md`](docs/gamelist-compatibility.md) for the
upstream evidence, fixture provenance, API, path semantics and T003-A limits.

## Current verification status

| Platform | Build | Actual launch |
| --- | --- | --- |
| Linux amd64 | Verified locally | Verified locally |
| macOS arm64 | Built by CI; not verified locally | Not launched |
| Windows amd64 | Built by CI; not verified locally | Not launched |

"Verified locally" means the command was run in this checkout and the window
was started. The macOS and Windows entries are build results from the
GitHub-hosted CI matrix, not from a local machine. The CI matrix is a build
check, not a substitute for a manual launch on macOS or Windows.

## Known limitations

- The heartbeat is a deliberately small event demo, not a persistent task
  scheduler. It runs every two seconds while the window is open and stops via
  the Wails shutdown hook.
- SQLite uses one connection (`MaxOpenConns(1)`) for deterministic PRAGMAs and
  simple WAL behavior. This will need measurement before adding concurrent
  workers.
- The first migration creates only `schema_migrations`; there are no library
  or provider tables yet.
- macOS and Windows runtime behavior is not manually tested in this task.
  Code signing, notarization, installers, and auto-update are out of scope.
- Wails v2.17.1's development proxy requires `WAILS_DEV_ALLOWED_HOSTS=wails.localhost`
  in this environment. The desktop dev window still loaded without it, but the
  proxy logged a non-fatal 403 for its own `wails.localhost` request.
- The Wails-generated bindings under `frontend/wailsjs/` are committed because
  the frontend typecheck relies on them. They are regenerated by
  `wails build`/`wails dev`; do not hand-edit them.

## Task boundary

LS-T002 adds the standalone ROM scanner. LS-T003-A adds only the in-memory
gamelist parser/editor and its documentation; it does not add disk writes,
ScreenScraper or other providers, media downloads, database library tables,
worker pools, or a full library UI. Those require later tasks.
