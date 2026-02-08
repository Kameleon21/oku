# Project Plan: `oku` — Hardcover CLI (with `oku`)

## 0) Goal

Build a fast, scriptable CLI for Hardcover that lets you:

- View your current reading list
- Search books
- Set an “active” book
- Update reading progress **by page**
- Change status, using **`oku`** for the “to read” pile
- Stay reliable via cache-first behavior and secure token storage

Non-goals (v1):

- Full-screen TUI
- Social features (reviews, feed, etc.)
- MCP server (planned later)

---

## 1) User Stories (v1)

1. I can authenticate with my Hardcover API token securely.
2. I can list my books by status: `reading`, `oku`, `finished`, `dnf`.
3. I can search for a book and choose it.
4. I can set an “active book” so commands default to it.
5. I can update progress via page number quickly.
6. I can change status quickly from the CLI.
7. The tool works well in a terminal workflow (pipeable `--json`, cache-first, fast).

---

## 2) CLI Command Spec

Binary name: `oku`

### Auth

- `oku auth set-token`
  - Stores token securely (macOS Keychain preferred).
  - Accept token via prompt or stdin.

- Token priority:
  1. `HARDCOVER_TOKEN` env var
  2. Keychain token
  3. (optional) config token if user explicitly enables plaintext storage

### Listing

- `oku list <reading|oku|finished|dnf> [--json] [--refresh]`
  - Reads from cache by default
  - `--refresh` forces API sync

Convenience shortcuts (optional but ergonomic):

- `oku reading`
- `oku oku`
- `oku finished`
- `oku dnf`

### Current / “Now”

- `oku now [--json] [--refresh]`
  - Equivalent to `oku list reading`

### Search

- `oku search <query> [--json] [--limit N]`
  - Returns book results (title/author/id/page count if available)

### Select active book

- `oku active [--json]`
  - Shows the active book (from local state)

- `oku set-active --book <id>`
  - Sets active book explicitly

- `oku open`
  - Interactive picker to choose a book and set active
  - Uses `fzf` if installed, otherwise a simple numbered prompt

### Update progress (pages)

- `oku update --page <N|+N|-N> [--book <id>] [--note <text>]`
  - If `--book` omitted, targets **active book**
  - `--page` parsing:
    - `123` → set page to 123
    - `+10` → add 10 pages
    - `-5` → subtract 5 pages

  - Validation:
    - clamp to >= 0
    - if total pages known, clamp <= total (or warn + allow if you prefer)

### Status changes

- `oku status <reading|oku|finished|dnf> [--book <id>]`
  - Default target: active book

Backwards-compatible aliases (recommended):

- `want`, `wtr`, `want-to-read` → `oku`

### Sync

- `oku sync`
  - Refresh all tracked lists into cache (reading/oku/finished/dnf)
  - Refresh active book details

### Config

- `oku config edit`
  - Opens config file in `$EDITOR` (fallback `vi`)

- `oku config show`
  - Prints current config values (no secrets)

Exit codes:

- `0` success
- `1` user/input error (missing token, missing active book, invalid args)
- `2` network/API error

---

## 3) Status Model (with `oku`)

Internal enum:

- `reading`
- `oku`
- `finished`
- `dnf`

Display:

- “Oku” in headings/output (or configurable)

---

## 4) Configuration & Storage

### Config file

Path: `~/.config/oku/config.toml` (or XDG)

Suggested keys:

- `editor = "nvim"`
- `notes_dir = "~/notes/books"` (optional, v1 can omit notes)
- `use_fzf = true`
- `default_list = "reading"`

### Token storage

- macOS Keychain entry (service `oku`, account `hardcover`)
- env var override `HARDCOVER_TOKEN`

### Cache DB

SQLite at `~/.local/share/oku/cache.db` (or XDG)

Suggested tables:

- `books`
  - `id TEXT PRIMARY KEY`
  - `title TEXT`
  - `authors TEXT` (JSON array or joined string)
  - `page_count INTEGER`
  - `updated_at TEXT`

- `user_books`
  - `book_id TEXT`
  - `status TEXT` (`reading|oku|finished|dnf`)
  - `current_page INTEGER`
  - `last_progress_at TEXT`
  - `last_sync_at TEXT`
  - `PRIMARY KEY (book_id)`

- `state`
  - `key TEXT PRIMARY KEY`
  - `value TEXT`
  - store `active_book_id`, timestamps, etc.

Cache strategy:

- Serve lists from cache immediately
- Refresh only when asked (`--refresh` or `oku sync`)
- Keep API calls minimal to avoid throttling

---

## 5) Hardcover API Integration (GraphQL)

Use a GraphQL client:

- Preferred: typed generation (e.g. `genqlient`)
- Alternative: lightweight client (e.g. `machinebox/graphql`)

Requirements:

- Set `authorization: <token>` header
- Rate limit to stay under Hardcover’s 60 req/min
- Retry/backoff on 429 + transient 5xx
- Normalize API objects into internal domain structs

Minimum operations to implement:

- `GetMe` (validate token)
- `ListUserBooks(status)` for `reading`, `oku`, `finished`, `dnf`
- `SearchBooks(query, limit)`
- `SetUserBookStatus(bookId, status)`
- `UpdateReadingProgress(bookId, page)`

Keep list queries minimal (id/title/authors/page_count/current_page/status/updated_at).

---

## 6) Go Architecture (CLI-first)

Suggested layout:

```
cmd/oku/
internal/
  app/        // business logic: list/search/update/status/active
  api/        // hardcover graphql client + operations
  store/      // sqlite cache + state + token provider
  cli/        // command definitions + output formatting
  picker/     // fzf integration + fallback prompt
  config/     // config load/validate
```

Rule:

- `internal/app` is the “engine” used by commands.
- `internal/cli` should not know about GraphQL response types.

---

## 7) Milestones & Deliverables

### Milestone A — Working CLI MVP

Deliver:

- `oku auth set-token`
- `oku now`
- `oku list <status>`
- `oku search`
- `oku open` (fzf + fallback)
- `oku set-active`, `oku active`
- `oku update --page`
- `oku status <reading|oku|finished|dnf>`
- SQLite cache + state

Acceptance:

- You can do a full loop:
  1. `oku search "…"`, `oku open`
  2. `oku status reading`
  3. `oku update --page 123`
  4. `oku status finished`

### Milestone B — Polish for daily use

Deliver:

- `--json` for list/search/active/now
- `oku sync`
- better errors + `--debug` logging (never prints token)
- Homebrew formula/tap (optional)

---

## 8) Testing Plan

Unit tests:

- page parsing (`123`, `+10`, `-5`, invalid)
- alias mapping (`want` → `oku`)
- state handling (active book set/get)
- cache upsert behavior

Integration tests (optional):

- mock GraphQL responses, ensure commands behave as expected

Manual QA:

- missing token
- invalid token
- missing active book
- offline behavior (cached list still works)
- rate limit (simulate 429)

---

## 9) Future (explicitly deferred)

- Add TUI (`oku` fullscreen)
- Add MCP server adapter on top of `internal/app`
- Notes integration (`oku note`, open in nvim)
- Audiobook/time progress
