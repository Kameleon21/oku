# Oku CLI — Implementation Plan (Milestone A)

## API Reference (from official docs)

- **Endpoint:** `https://api.hardcover.app/v1/graphql` (Hasura)
- **Auth header:** `authorization: <token>` (plain token, not Bearer)
- **Rate limit:** 60 req/min, 30s timeout, max depth 3
- **Disabled operators:** `_like`, `_ilike`, `_regex`, `_similar` (all pattern matching)
- **Schema source:** `/tmp/hardcover-docs/schema.graphql`

### Status IDs (official, from Books.mdx)

| ID  | Hardcover Label   | CLI alias  |
| --- | ----------------- | ---------- |
| 1   | Want to Read      | `oku`      |
| 2   | Currently Reading | `reading`  |
| 3   | Read              | `finished` |
| 4   | Paused            | (future)   |
| 5   | Did Not Finish    | `dnf`      |
| 6   | Ignored           | (future)   |

### Key Queries

```graphql
# Validate token / get user ID
query {
  me {
    id
    username
  }
}

# List books by status (via me, avoids needing user_id)
query Me {
  me {
    user_books(where: { status_id: { _eq: 2 } }) {
      id
      status_id
      user_book_reads {
        progress_pages
      }
      book {
        id
        title
        pages
        contributions {
          author {
            name
          }
        }
      }
    }
  }
}

# Search books (Typesense backend)
query {
  search(
    query: "project hail mary"
    query_type: "Book"
    per_page: 10
    page: 1
  ) {
    results
  }
}
```

### Key Mutations

```graphql
# Create user_book (add book to shelf with status)
mutation {
  insert_user_book(object: { book_id: 123, status_id: 2 }) {
    id
    user_book {
      id
    }
  }
}

# Update user_book status
mutation {
  update_user_book(id: 456, object: { status_id: 3 }) {
    id
  }
}

# Update reading progress (on existing user_book_read)
mutation {
  update_user_book_read(id: 789, object: { progress_pages: 142 }) {
    id
  }
}

# Upsert user_book_reads (create or update reads for a user_book)
mutation {
  upsert_user_book_reads(
    user_book_id: 456
    datesRead: [{ progress_pages: 142 }]
  ) {
    user_book_reads {
      id
    }
  }
}

# Insert new read entry
mutation {
  insert_user_book_read(
    user_book_id: 456
    user_book_read: { progress_pages: 50, started_at: "2025-01-01" }
  ) {
    id
  }
}
```

### Input Types

- **UserBookCreateInput:** `book_id: Int!`, `status_id: Int`, `edition_id: Int`, `rating: numeric`, ...
- **UserBookUpdateInput:** `status_id: Int`, `edition_id: Int`, `rating: numeric`, ...
- **DatesReadInput:** `id: Int`, `progress_pages: Int`, `progress_seconds: Int`, `started_at: date`, `finished_at: date`, `edition_id: Int`, `action: String`

---

## Go Dependencies

- **CLI:** `github.com/spf13/cobra` (standard, well-tested)
- **GraphQL:** `github.com/hasura/go-graphql-client` (native Hasura support) or `github.com/machinebox/graphql` (lightweight)
- **SQLite:** `github.com/mattn/go-sqlite3` (cgo) or `modernc.org/sqlite` (pure Go, no cgo)
- **Keychain:** `github.com/zalando/go-keyring` (cross-platform, macOS Keychain)
- **Config:** `github.com/BurntSushi/toml`
- **Color output:** `github.com/fatih/color` (optional, nice for terminal)

**Decision: Use `modernc.org/sqlite` (no cgo dependency) and `github.com/machinebox/graphql` (simple, no codegen needed).**

---

## Directory Structure

```
cmd/oku/main.go           # entrypoint
internal/
  api/                     # GraphQL client + operations
    client.go              # HTTP client setup, auth, rate limiting
    queries.go             # GetMe, ListUserBooks, SearchBooks
    mutations.go           # InsertUserBook, UpdateUserBook, UpdateProgress
    types.go               # API response types (not exported beyond api/)
  app/                     # business logic (the "engine")
    app.go                 # App struct, constructor
    list.go                # ListBooks(status)
    search.go              # SearchBooks(query, limit)
    active.go              # GetActive, SetActive
    update.go              # UpdateProgress(page)
    status.go              # ChangeStatus(status)
    sync.go                # SyncAll
  store/                   # SQLite cache + state
    db.go                  # Open, migrate, close
    books.go               # UpsertBook, GetBooks
    user_books.go          # UpsertUserBook, GetUserBooks(status)
    state.go               # Get/Set key-value (active_book_id, etc.)
  auth/                    # token retrieval
    token.go               # GetToken (env -> keychain -> config), SetToken
  config/                  # TOML config
    config.go              # Load, defaults, paths
  cli/                     # cobra commands
    root.go                # root command, global flags
    auth.go                # auth set-token
    list.go                # list <status>, reading, oku, finished, dnf, now
    search.go              # search <query>
    active.go              # active, set-active
    open.go                # open (fzf picker)
    update.go              # update --page
    status.go              # status <reading|oku|finished|dnf>
    sync.go                # sync
    config_cmd.go          # config edit, config show
    output.go              # shared formatting (table, json)
  picker/                  # fzf integration
    picker.go              # PickBook (fzf if available, else numbered prompt)
  model/                   # shared domain types
    book.go                # Book, UserBook, Status enum, page parsing
```

---

## Implementation Steps

### Phase 1: Scaffolding [~10 min]

- [ ] `go mod init github.com/kamilrogozinski/oku`
- [ ] Create directory structure
- [ ] Install dependencies
- [ ] Minimal `cmd/oku/main.go` that prints version
- [ ] Verify `go build ./cmd/oku` works

### Phase 2: Domain Model + Config [~15 min]

- [ ] `internal/model/book.go` — Book, UserBook, Status constants, page parsing
- [ ] `internal/config/config.go` — TOML config load with XDG paths
- [ ] Unit tests for page parsing (`123`, `+10`, `-5`, clamping, invalid)
- [ ] Unit tests for status alias mapping (`want` -> `oku`, etc.)

### Phase 3: Token Storage [~15 min]

- [ ] `internal/auth/token.go` — GetToken (env -> keychain), SetToken (keychain)
- [ ] Priority: `HARDCOVER_TOKEN` env > keychain
- [ ] Prompt for token via stdin when `set-token` called

### Phase 4: SQLite Store [~25 min]

- [ ] `internal/store/db.go` — Open, auto-migrate schema, close
- [ ] `internal/store/books.go` — UpsertBook, GetBookByID
- [ ] `internal/store/user_books.go` — UpsertUserBook, ListUserBooks(status), GetUserBookByBookID
- [ ] `internal/store/state.go` — Get/Set string KV (active_book_id, last_sync, user_id)
- [ ] Tests for cache upsert + state round-trip

### Phase 5: GraphQL API Client [~30 min]

- [ ] `internal/api/client.go` — NewClient(token), rate limiter (token bucket), retry on 429/5xx
- [ ] `internal/api/types.go` — Response structs matching Hardcover schema
- [ ] `internal/api/queries.go` — GetMe, ListUserBooks(statusID), SearchBooks(query, perPage)
- [ ] `internal/api/mutations.go` — InsertUserBook, UpdateUserBook(status), UpdateProgress(pages), UpsertUserBookReads

### Phase 6: App Layer [~25 min]

- [ ] `internal/app/app.go` — App struct (holds api client, store, config)
- [ ] `internal/app/list.go` — ListBooks: cache-first, refresh if flag set
- [ ] `internal/app/search.go` — SearchBooks: always hits API (search is live)
- [ ] `internal/app/active.go` — GetActiveBook, SetActiveBook (from store state)
- [ ] `internal/app/update.go` — UpdateProgress: resolve active book, parse page, call API, update cache
- [ ] `internal/app/status.go` — ChangeStatus: resolve book, call API, update cache
- [ ] `internal/app/sync.go` — SyncAll: refresh all statuses into cache

### Phase 7: CLI Commands [~30 min]

- [ ] `internal/cli/root.go` — Root command, `--json` global flag, version
- [ ] `internal/cli/auth.go` — `oku auth set-token`
- [ ] `internal/cli/list.go` — `oku list <status>`, `oku now`, `oku reading`, `oku oku`, `oku finished`, `oku dnf`
- [ ] `internal/cli/search.go` — `oku search <query> [--limit N]`
- [ ] `internal/cli/active.go` — `oku active`, `oku set-active --book <id>`
- [ ] `internal/cli/open.go` — `oku open` (interactive picker)
- [ ] `internal/cli/update.go` — `oku update --page <N|+N|-N> [--book <id>]`
- [ ] `internal/cli/status.go` — `oku status <reading|oku|finished|dnf> [--book <id>]`
- [ ] `internal/cli/sync.go` — `oku sync`
- [ ] `internal/cli/config_cmd.go` — `oku config edit`, `oku config show`
- [ ] `internal/cli/output.go` — Table formatter, JSON output

### Phase 8: Picker [~10 min]

- [ ] `internal/picker/picker.go` — Try fzf first (pipe to stdin, read selection), fallback to numbered prompt
- [ ] Used by `oku open` to select from currently reading list

### Phase 9: Integration + Testing [~20 min]

- [ ] `go build ./cmd/oku` — verify compiles cleanly
- [ ] `go vet ./...` + `go test ./...`
- [ ] Manual smoke test with real token:
  - `oku auth set-token`
  - `oku now`
  - `oku search "Project Hail Mary"`
  - `oku list reading`
  - `oku open`
  - `oku active`
  - `oku update --page 50`
  - `oku status finished`
- [ ] Exit codes: 0 success, 1 user error, 2 network error

---

## Execution Strategy

**Phases 1-3** are sequential (scaffolding must come first, then model/config/auth).

**Phases 4-5** (store + API client) can be built in parallel via subagents since they're independent.

**Phase 6** (app layer) depends on 4+5.

**Phases 7-8** (CLI + picker) depend on 6.

**Phase 9** is final verification.

Plan: Build phases 1-3 in the main context, then launch parallel agents for phases 4+5, then wire together in 6-9.

---

## Exit Criteria (Milestone A)

Full loop works:

1. `oku auth set-token` — stores token
2. `oku search "..."` — returns results
3. `oku open` — picks a book, sets active
4. `oku status reading` — marks active book as reading
5. `oku update --page 123` — updates progress
6. `oku now` — shows currently reading with progress
7. `oku status finished` — marks as finished
8. All unit tests pass
9. `go vet ./...` clean
