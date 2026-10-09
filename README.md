# LinFBBPages

Web application for managing a [LinFBB](https://sourceforge.net/projects/linfbb/) instance (packet-radio BBS) running on the same host, or in a container with a shared FBB filesystem.

## Features

### Stage 1 (current)

- **Login**: validate callsign and password against FBB's `inf.sys` (only users with a configured password).
- **Messages**: searchable paginated list (title, sender, recipient, and route) built from the `dirmes.sys` index with sender, recipient, route, and size; compact two-line entries on mobile; reply (SR) and personal copy (SC) actions; message bodies read from `mail/`; and composition/sending through `mail/mail.in` (FBB imports them automatically in approximately one minute).
- **Message visibility**: non-Sysop users see bulletins and private messages sent or received by them; Sysops can see all valid message types and statuses.
- **7+ files**: gallery with previews of decoded files in `7pfbb/ok/`, file information, download links, image viewer, search, sorting, cards, and a directory-style list without image previews.

### Stage 2 (future)

- Application information
    - References to the LinFBB project
    - Version
    - License
    - Application repository
- BBS information
- Message list
    - Sort by columns
    - Infinite scrolling
- Messaging
    - Reply, private reply, forward, etc.
    - File sending (7+ conversion)
    - CP437 Font (?)
- Read-message history and filtering by "unread"
- Login without password (read-only)
    - User request form
- Terminal
    - Web application <-> backend <-> Telnet <-> LinFBB (?)
- API

### Stage 3 (another future)

- FBB instance management
    - Sysop
        - Configuration
        - User administration
        - Logs
        - ...

## Architecture

- **Backend**: Go using only the standard library. Produces a single static binary designed for modest hardware (1 GHz CPU / 512 MB RAM).
- **Frontend**: vanilla HTML/CSS/JS, with no build step, embedded in the binary with `go:embed`. Multilingual interface (ES/EN).
- **FBB data access**: direct, read-only access to FBB files. The only write operation performed by the application in the FBB directory is creating/appending to `mail/mail.in` to send messages.

The gallery detects each file's actual MIME type. This allows it to preview both `.jpg` images and JPEG payloads that some installations store with a `.7mf` extension; `.7ix` and `.err` files are shown as auxiliary files.

```
┌─────────────┐   HTTP/JSON   ┌──────────────────┐   read   ┌─────────────────────┐
│  Frontend   │ ◄───────────► │  Backend (Go)    │ ───────► │ inf.sys, dirmes.sys │
│  (static,   │               │  net/http stdlib │          │ mail/*, 7pfbb/ok/*  │
│   embedded) │               │                  │ ───────► │ mail/mail.in        │
└─────────────┘               └──────────────────┘  append  └─────────────────────┘
```

## Requirements

- Go 1.22+ (only needed for compilation; `CGO_ENABLED=0` produces a dependency-free binary).
- Read access to the FBB data directory (`/usr/local/var/ax25/fbb` by default) and write access to `mail/` for sending messages.

## Build and run

```sh
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o linfbbpages ./cmd/linfbbpages
./linfbbpages --fbb-dir /usr/local/var/ax25/fbb --listen :8080
```

## Configuration

All options can be provided as a flag or an environment variable.

| Flag            | Environment variable        | Default                     | Description |
|-----------------|-----------------------------|-----------------------------|-------------|
| `--fbb-dir`     | `LINFBBPAGES_FBB_DIR`       | `/usr/local/var/ax25/fbb`   | FBB data directory. |
| `--listen`      | `LINFBBPAGES_LISTEN`         | `:8080`                     | HTTP listen address and port. |
| `--fbb-arch`    | `LINFBBPAGES_FBB_ARCH`       | `auto`                      | Binary format of `inf.sys`/`dirmes.sys`: `auto`, `32`, or `64` (see below). |
| `--session-ttl` | `LINFBBPAGES_SESSION_TTL`    | `8h`                        | Login session duration. |

### FBB binary format (`--fbb-arch`)

FBB binary files (`inf.sys`, `dirmes.sys`) contain C structs with `long` fields. The size of those fields depends on how FBB was compiled:

- **32-bit** (`long` = 4 bytes): 360-byte records (`inf.sys`) and 194-byte records (`dirmes.sys`).
- **64-bit** (`long` = 8 bytes, aligned to 8): 384-byte records and 224-byte records.

With `--fbb-arch=auto` (the default), the application detects the format at startup based on file-size divisibility. To diagnose an installation manually:

```sh
# inf.sys: size % 360 == 0 → 32-bit ; size % 384 == 0 → 64-bit
# dirmes.sys: size % 194 == 0 → 32-bit ; size % 224 == 0 → 64-bit
stat -c '%s %n' /usr/local/var/ax25/fbb/inf.sys /usr/local/var/ax25/fbb/dirmes.sys
```

If both formats are compatible with the file sizes (very large files that are multiples of both record sizes), `auto` assumes 64-bit. Little-endian encoding is assumed (valid for common x86 and ARM systems).

## Development

The [`design/`](design/) directory contains the original FBB format documentation (`design/docs/`) and **real installation fixtures** (`design/usr/local/var/ax25/fbb/`). To run in development mode against the fixtures:

```sh
# The fixtures use the 32-bit format:
CGO_ENABLED=0 go run ./cmd/linfbbpages --fbb-dir design/usr/local/var/ax25/fbb --fbb-arch 32
```

```sh
go test ./...
```

## Security notes

- Passwords in `inf.sys` are stored as plaintext by FBB; the application only reads them to validate login.
- Sessions are kept in memory (they are lost when the backend restarts) and identified by an httpOnly cookie.
- The application is intended for trusted networks (LAN/VPN). If exposed to the internet, place it behind a reverse proxy with TLS.

## Repository structure

```
├── cmd/linfbbpages/   # binary entry point
├── internal/
│   ├── fbb/           # parsers for inf.sys, dirmes.sys, mail/, 7pfbb/
│   ├── api/           # HTTP handlers, authentication, and sessions
│   └── config/        # configuration (flags + environment variables)
├── web/static/        # embedded frontend (index.html, app.js, style.css, i18n/)
└── design/            # FBB format documentation + fixtures for tests/development
```

## License

To be defined.
