"""Seed a disposable recording library, never the user's normal database.

Reads start in the past so the forecasts have a pace to show, there is a
books goal for the year, and the finished books match the dashboard's
sample stats (12 books, the same months, 3,748 pages, a 3.9 average) so
every number on screen tells the same story.
"""
import os
import sqlite3
from datetime import datetime, timedelta, timezone
from pathlib import Path

root = Path(os.environ['XDG_DATA_HOME'])
if not root.parent.name.startswith('oku-recording.'):
    raise SystemExit('Run through docs/demo/record.sh with a temporary data directory')
now = datetime.now(timezone.utc)
stamp = now.strftime('%Y-%m-%dT%H:%M:%SZ')
year = now.year


def day(days_ago):
    """A bare date as Hardcover sends it: UTC midnight."""
    return f'{(now - timedelta(days=days_ago)).date().isoformat()}T00:00:00Z'


# id, title, author, pages, status, progress, started (days ago)
books = [
    (1, 'The Lantern Library', 'Aki Mori', 288, 2, 214, 9),
    (2, 'A Map of Quiet Places', 'Ren Sato', 320, 2, 300, 15),
    (3, 'Letters from the Inland Sea', 'Nao Hayashi', 224, 2, 61, 7),
    (4, 'The Paper Garden', 'Yuki Watanabe', 256, 1, 0, 0),
    (5, 'Midnight at the Bookshop', 'Haru Tanaka', 304, 1, 0, 0),
    (6, 'Small Things, Slowly', 'Mei Ito', 192, 1, 0, 0),
]
months = [1, 1, 1, 2, 2, 4, 4, 4, 4, 5, 6, 6]
pages = [288, 352, 240, 416, 304, 264, 336, 312, 288, 320, 256, 372]
ratings = [4, 4, 4, 4, 3.5, 3.5, 4, 4, 4, 0, 0, 0]

with sqlite3.connect(root / 'oku' / 'cache.db') as db:
    def add(book_id, title, author, book_pages, status, rating=0):
        db.execute('INSERT INTO books (id,title,authors,pages,updated_at) VALUES (?,?,?,?,?)',
                   (book_id, title, author, book_pages, stamp))
        db.execute('INSERT INTO user_books (id,book_id,status_id,rating,updated_at) VALUES (?,?,?,?,?)',
                   (book_id, book_id, status, rating, stamp))

    for book_id, title, author, book_pages, status, progress, started in books:
        add(book_id, title, author, book_pages, status)
        if progress:
            db.execute('INSERT INTO user_book_reads (id,user_book_id,progress_pages,started_at) VALUES (?,?,?,?)',
                       (book_id, book_id, progress, day(started)))
    for i, month in enumerate(months):
        book_id, done = 100 + i, f'{year}-{month:02d}-{10 + i:02d}T00:00:00Z'
        add(book_id, f'Finished Book {i + 1}', 'Various', pages[i], 3, ratings[i])
        db.execute('INSERT INTO user_book_reads (id,user_book_id,progress_pages,started_at,finished_at) VALUES (?,?,?,?,?)',
                   (book_id, book_id, pages[i], done, done))
    # Goal dates are bare dates in the store.
    db.execute('INSERT INTO goals (id,metric,target,progress,state,start_date,end_date) VALUES (?,?,?,?,?,?,?)',
               (1, 'books', 20, 12, 'active', f'{year}-01-01', f'{year}-12-31'))
    for status in (1, 2, 3):
        db.execute('INSERT OR REPLACE INTO state (key,value) VALUES (?,?)',
                   (f'last_sync_status_{status}', stamp))
