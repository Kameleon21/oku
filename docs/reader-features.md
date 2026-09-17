# Reader tools

These commands use your configured Hardcover account. IDs are Hardcover book or goal IDs.

## Notes and quotes

```sh
oku note --book 123 'An idea to return to'
oku quote --book 123 'A passage from the book'
oku journal --book 123
oku journal --book 123 --json
```

Omit `--book` when there is exactly one active book. Entries use your account's default privacy. If that setting cannot be read, saving stops instead of falling back to public.

In a library tab, press **n** to open the journal editor, **Tab** to choose note or quote, **Ctrl+S** to save, or **Esc** to cancel. Failed saves retain your draft. Enter opens book detail and loads your existing notes and quotes below the community information; press Enter again to refresh the journal after saving.

## Pause and resume

```sh
oku status paused --book 123
oku paused
oku status reading --book 123
```

In the dashboard, **p** pauses the selected book. In Reading, **P** toggles the paused shelf; **g** resumes the selected book. Pausing preserves reading progress and removes the book from the active reading list. `x` continues to mean Ignore.

## Goals

```sh
oku goals
oku goals set --target 24 --year 2026 --description 'This year’s reading'
oku goals set --id 42 --target 30
oku goals set --target 6000 --metric page --start 2026-01-01 --end 2026-12-31
```

New goals use your account privacy. Updating an existing goal preserves unspecified settings, including conditions and privacy. Metrics are `book` and `page`. Run `oku sync` after editing to refresh the Stats view. Add `--json` for machine-readable results.

## Book details and discovery

```sh
oku book 123
oku book 123 --refresh --json
oku trending --week
oku trending --period month --limit 30 --json
```

Book detail includes description, headline, community ratings distribution and editions count. Details are cached; `--refresh` reloads them. Dashboard **Enter** loads this information into the scrollable detail pane. Use j/k to scroll and Esc to return.

In Search, leave the input with Esc (or navigate to Search with h/l), then press **D** to browse this week's trending books. Existing detail and add-to-shelf keys work on discovery results. CLI periods are `week`, `month`, `three_month`, `one_year`, and `all`.

## Want-to-read priority

On the Oku tab, **K** raises a book by one place and **J** lowers it. **R** refreshes the order from Hardcover. Selection stays on the moved book.

The first reorder creates a **private ranked Hardcover list named “Oku reading queue”** and adds missing want-to-read books in batches of five. Initial setup can take some time for large shelves because of the API rate limit. Existing entries are preserved. Queue order is cached locally; use R after changes on another device. Books no longer on your want-to-read shelf stay in the Hardcover list but are hidden from the Oku shelf. A remotely edited queue with duplicate ranks must be corrected on Hardcover before reordering in Oku.

## Export and import

```sh
oku export --format json --output library.json
oku export --format csv --output library.csv
oku import library.json
oku import library.json --apply
oku import goodreads_library_export.csv --format goodreads
oku import goodreads_library_export.csv --format goodreads --apply
```

Export reads the full library in pages of 100, including all reading-history records exposed by Oku (pages, start and finish dates), ratings, reviews and statuses. JSON exports have a versioned envelope. CSV preserves multiple reads in `reads_json`. Export covers library records, not notes, quotes, goals, ranked lists or timer sessions. Output files are created with private permissions and existing files are never overwritten; omit `--output` to write to stdout.

Import is a **preview unless `--apply` is supplied**. It validates the whole file and resolves matches before writing. Existing library books and duplicate input rows are skipped, preserving your current progress, ratings and reviews. Goodreads rows match by ISBN13/ISBN, never by Goodreads numeric ID or an approximate title. Missing or ambiguous ISBNs are reported as `unmatched` and skipped. Supported Goodreads shelves are `to-read`, `currently-reading`, and `read`; custom shelf tags are not imported. Ratings, review text and Date Read are included. Missing historical review dates use today's date when a review is imported.

An applied import stops at the first write error and reports completed, partial and not-attempted rows. If a book was added but saving its metadata failed, repair that entry before retrying: existing books are deliberately skipped. Requests that create records are not automatically retried after uncertain network failures. Check Hardcover before retrying an uncertain write. The command refreshes the local library after a successful import. Use `--json` for structured per-row results.

API reference: [Hardcover API](https://docs.hardcover.app/api/getting-started/).
