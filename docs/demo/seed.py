"""Seed a disposable recording library, never the user's normal database."""
import os
import sqlite3
from datetime import datetime, timezone
from pathlib import Path

root = Path(os.environ['XDG_DATA_HOME'])
if not root.parent.name.startswith('oku-recording.'):
    raise SystemExit('Run through docs/demo/record.sh with a temporary data directory')
now = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
books = [
    (1, 'The Lantern Library', 'Aki Mori', 288, 2, 124),
    (2, 'Letters from the Inland Sea', 'Nao Hayashi', 224, 2, 68),
    (3, 'A Map of Quiet Places', 'Ren Sato', 320, 2, 205),
    (4, 'The Paper Garden', 'Yuki Watanabe', 256, 1, 0),
    (5, 'Midnight at the Bookshop', 'Haru Tanaka', 304, 1, 0),
    (6, 'Small Things, Slowly', 'Mei Ito', 192, 1, 0),
]
with sqlite3.connect(root / 'oku' / 'cache.db') as db:
    for book_id, title, author, pages, status, progress in books:
        db.execute('INSERT INTO books (id,title,authors,pages,updated_at) VALUES (?,?,?,?,?)',
                   (book_id, title, author, pages, now))
        db.execute('INSERT INTO user_books (id,book_id,status_id,updated_at) VALUES (?,?,?,?)',
                   (book_id, book_id, status, now))
        if progress:
            db.execute('INSERT INTO user_book_reads (id,user_book_id,progress_pages,started_at) VALUES (?,?,?,?)',
                       (book_id, book_id, progress, now))
    for status in (1, 2):
        db.execute('INSERT OR REPLACE INTO state (key,value) VALUES (?,?)',
                   (f'last_sync_status_{status}', now))
