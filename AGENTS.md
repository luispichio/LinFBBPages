# AGENTS.md — LinFBBPages

Context for agents working in this repository. Read this file completely before modifying code.

## What this project is

Web application for managing a [LinFBB](https://sourceforge.net/projects/linfbb/) instance (packet-radio BBS) running on the same host (or in a container with a shared filesystem). Stage 1: login, message viewing/composition, and viewing decoded 7+ files. Stage 2 (future): instance management.

The backend **reads FBB data binary files directly**. Parser correctness is the most critical part of this project.

## Stack and constraints

- **Backend**: Go, **standard library only** (`net/http`, `encoding/binary`, `embed`, `crypto/rand`, ...). Do not add external dependencies without the user's explicit approval. Target hardware: modest systems (1 GHz / 512 MB RAM).
- **Frontend**: vanilla HTML/CSS/JS in `web/static/`, **no build step or npm**, embedded with `go:embed`. ES/EN i18n through JavaScript dictionaries (`web/static/i18n/es.js`, `en.js`); default language ES, detected from `navigator.language`.
- **Sessions**: in memory (token-to-user map with expiration), an httpOnly `SameSite=Lax` cookie, and a 32-byte `crypto/rand` token. Do not persist sessions to disk.

## Repository structure

```
├── cmd/linfbbpages/   # main.go: config, wiring, HTTP server
├── internal/
│   ├── fbb/           # parsers: inf.go, dirmes.go, mail.go, sevenplus.go (+ _test.go)
│   ├── api/           # HTTP handlers, authentication, sessions
│   └── config/        # flags + environment variables
├── web/static/        # embedded frontend
└── design/            # DO NOT TOUCH: FBB format documentation + test/development fixtures
```

## Commands

```sh
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o linfbbpages ./cmd/linfbbpages # build
go test ./...                                # tests
go vet ./...                                 # basic lint

# Development against fixtures (they use the 32-bit format!):
CGO_ENABLED=0 go run ./cmd/linfbbpages --fbb-dir design/usr/local/var/ax25/fbb --fbb-arch 32
```

## FBB file formats

Original documentation is in `design/docs/` (`fmtinf.html`, `fmtdirme.html`, `fmtmail.html`). Real fixtures are in `design/usr/local/var/ax25/fbb/`.

### General rules

- C strings: end at the first NUL byte (`00`); discard the rest of the field.
- Integers: **little-endian**. Dates: seconds since 1970-01-01 00:00 UTC (Unix epoch).
- C `long` = 4 bytes in 32-bit builds, 8 bytes in 64-bit builds (with 8-byte alignment and padding). The layout is selected with `--fbb-arch` (`auto`|`32`|`64`, default `auto`), and parsers must be **table-driven** from a `Layout` type: hardcoding offsets outside the layout definition is forbidden.
- Files can change while FBB is running: cache parsed data by **mtime** and reread when it changes. Never keep file handles open.
- Historical note: the documentation declares `flags`/`on_base` as `unsigned`, but the fixture confirms they occupy **2 bytes** (2880 = 8×360 and `pass` is found at offset 338).
- LinFBB's `F_SYS` user flag is `0x0008` (the `S`/Sysop flag from `include/fbb_serv.h`). The parser exposes it as `User.Sysop`; it is captured in the in-memory session at login time.

### `inf.sys` — users (login)

Fixed-size records, **without a header**. File size must be divisible by the record size. Login = case-insensitive callsign with a non-empty matching `pass`.

| Field | Size | Offset (32-bit, record=360) | Offset (64-bit, record=384) |
|---|---|---|---|
| `indic` (callsign[7]+ssid) | 8 | 0 | 0 |
| `relai[8]` (digipeater path) | 64 | 8 | 8 |
| `lastmes` | long | 72 | 72 |
| `nbcon` | long | 76 | 80 |
| `hcon` | long | 80 | 88 |
| `lastyap` | long | 84 | 96 |
| `flags` | 2 | 88 | 104 |
| `on_base` | 2 | 90 | 106 |
| `nbl` | 1 | 92 | 108 |
| `lang` | 1 | 93 | 109 |
| _(padding)_ | — | — | 110–111 |
| `newbanner` | long | 94 | 112 |
| `download` | 2 | 98 | 120 |
| `free[20]` | 20 | 100 | 122 |
| `thema` | 1 | 120 | 142 |
| `nom[18]` | 18 | 121 | 143 |
| `prenom[13]` | 13 | 139 | 161 |
| `adres[61]` | 61 | 152 | 174 |
| `ville[31]` | 31 | 213 | 235 |
| `teld[13]` | 13 | 244 | 266 |
| `telp[13]` | 13 | 257 | 279 |
| `home[41]` (home BBS) | 41 | 270 | 292 |
| `qra[7]` (QTH locator) | 7 | 311 | 333 |
| `priv[13]` | 13 | 318 | 340 |
| `filtre[7]` | 7 | 325 | 353 |
| **`pass[13]`** | 13 | **338** | **360** |
| `zip[9]` | 9 | 351 | 373 |
| **Total record** | | **360** | **384** (382 + 2 tail pad) |

### `dirmes.sys` — message index

Fixed-size records. **Record 0 is a header**: only its `numero` field is valid (last assigned message number). Messages are records 1..N; a NUL `type` (`00`) invalidates a record.

| Field | Size | Offset (32-bit, record=194) | Offset (64-bit, record=224) |
|---|---|---|---|
| `type` (A,B,P,T) | 1 | 0 | 0 |
| `status` ($,A,F,K,N,Y) | 1 | 1 | 1 |
| _(padding)_ | — | — | 2–7 |
| `numero` | long | 2 | 8 |
| `taille` (body size) | long | 6 | 16 |
| `date` | long | 10 | 24 |
| `bbsf[7]` (BBS that delivered it) | 7 | 14 | 32 |
| `bbsv[41]` (route) | 41 | 21 | 39 |
| `exped[7]` (origin/from) | 7 | 62 | 80 |
| `desti[7]` (destination/to) | 7 | 69 | 87 |
| `bid[13]` (BID/MID) | 13 | 76 | 94 |
| `titre[61]` (title) | 61 | 89 | 107 |
| `free[16]` | 16 | 150 | 168 |
| `datesd` (creation) | long | 166 | 184 |
| `datech` (last status change) | long | 170 | 192 |
| `fbbs[10]` (BBS mask to forward to) | 10 | 174 | 200 |
| `forw[10]` (already-forwarded mask) | 10 | 184 | 210 |
| **Total record** | | **194** | **224** (220 + 4 tail pad) |

Types: `B` bulletin, `P` private, `A`/`T` less common. Statuses: `N` new, `Y` read, `F` forwarded, `K` killed, `A` archived, `$` being processed. Full semantics are documented in `design/docs/fmtdirme.html`.

Visibility rules: Sysops see all valid types and statuses. Non-Sysop users see `B` bulletins except `K`/`$` records, and `P`/`A`/`T` private messages only when their callsign matches the sender or recipient (case-insensitive and ignoring SSID); other types and `K`/`$` records are hidden.

### Architecture detection (`--fbb-arch=auto`)

At startup, for `inf.sys` and `dirmes.sys`:

1. Candidates = {32, 64}; discard an architecture whose record size does **not** evenly divide the file size (intersection across both files).
2. One candidate remains → use it.
3. Both remain (file size is a multiple of both record sizes) → assume **64** and log the ambiguity.
4. Neither remains → fatal error with a clear message (corrupt file or unknown format).

### Message bodies — `mail/`

- One file per message: `mail/mail<N>/m_%06d.mes` where **N = number % 10** (validated: `m_000110` is in `mail0`).
- Text content: zero or more routing-header lines `R:AAMMDD/hhmmZ ...`, then a blank line, then the body. Parse `R:` lines as headers and the remainder as plain text.
- `mail/mail.in` may exist (FBB import queue): **never expose it as a message**.

### Sending messages — `mail/mail.in`

FBB checks `mail/mail.in` every minute, imports messages, and **deletes the file**. The application creates/appends to it (with a file lock) with one or more messages in this format:

```
SP <destination>[@route] < <source> $<optional-BID>
<title>
<body...>
/EX
```

`SP` = private, `SB` = bulletin. Real examples are in the fixture `design/usr/local/var/ax25/fbb/mail/mail.in` and the documentation in `design/docs/fmtmail.html`. The UI must communicate the approximately one-minute delay.

### Decoded 7+ files — `7pfbb/ok/`

- Useful files (e.g. `.jpg`) are stored with basename metadata: `.7ix` (index) and `.err` (7PLUS error report). In real installations, `.7mf` may contain the decoded payload directly (for example, a JPEG), so MIME detection identifies it and displays it as the main file.
- The view groups files by basename: show the main file with a preview when it is an image; show metadata as optional auxiliary data, never mixed into the gallery. The frontend supports search, name/date/type sorting, image cards with lightbox previews, downloads, modified date/time display, and a directory-style list without image previews.
- `7pfbb/7pl_log` and raw `*.pNN` parts in `7pfbb/` are **not exposed** in stage 1.

## HTTP API

JSON; a session cookie is required for every endpoint except `/api/login`. The auth middleware rejects unauthorized requests with 401.

| Method | Path | Description |
|---|---|---|
| POST | `/api/login` | `{callsign, password}` → validates against `inf.sys`, sets a cookie. |
| POST | `/api/logout` | Invalidates the session. |
| GET | `/api/me` | Logged-in user data (callsign, name, QTH, `sysop`). |
| GET | `/api/messages` | Paginated visibility-filtered list from `dirmes.sys`. Query: `type`, `q` (title), `page`, `page_size` (default 50). Order: descending number. |
| GET | `/api/messages/{num}` | Metadata + `R:` headers + body from `mail/` when visible to the user. Returns 404 if it does not exist or is not visible. |
| POST | `/api/messages` | Compose: `{to, route, type: P|B, title, body}` → append to `mail.in`. Immediate response; asynchronous import (~1 min). |
| GET | `/api/files` | Files from `7pfbb/ok/` grouped by basename (main + auxiliary). |
| GET | `/api/files/{name}` | Serves a file (inline for images). **Sanitize against path traversal.** |
| GET | `/*` | Embedded static frontend. |

## Security rules and invariants (non-negotiable)

1. **Never write to or modify FBB files** (`inf.sys`, `dirmes.sys`, `mail/*.mes`, `7pfbb/**`). The sole exception is creating/appending to `mail/mail.in`.
2. Fixtures under `design/` are **read-only**: do not edit or delete them; tests must not mutate them.
3. Any endpoint receiving a filename must validate it (no `..`, no path separators) before opening anything.
4. Login is allowed only for callsigns with a non-empty password; password comparison is case-sensitive, callsign comparison is case-insensitive.
5. Messages with an invalid status or NUL `type` must not be displayed.
6. Non-Sysop users must not receive private messages (`P`/`A`/`T`) belonging to other users, or `K`/`$` records. The API returns 404 for inaccessible message details; Sysops can see all valid records.

## Conventions

- Keep changes minimal and focused; follow the existing style.
- Every new parser must include tests: 32-bit layout against real fixtures in `design/`, 64-bit layout with synthetic records built from the tables above.
- Comments and UI are in Spanish unless they are code identifiers (English).
- If anything documented here changes (formats, endpoints, structure, commands), update this `AGENTS.md` and `README.md` in the same change.
