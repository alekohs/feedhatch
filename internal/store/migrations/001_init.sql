CREATE TABLE feeds (
    id            INTEGER PRIMARY KEY,
    url           TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL DEFAULT '',
    category      TEXT NOT NULL DEFAULT '',
    interval      INTEGER NOT NULL DEFAULT 15,
    etag          TEXT NOT NULL DEFAULT '',
    last_modified TEXT NOT NULL DEFAULT '',
    fetched_at    INTEGER
);

CREATE TABLE articles (
    id           INTEGER PRIMARY KEY,
    feed_id      INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid         TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    link         TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL DEFAULT '',
    published_at INTEGER,
    read         INTEGER NOT NULL DEFAULT 0,
    UNIQUE (feed_id, guid)
);

CREATE INDEX articles_feed_published ON articles (feed_id, published_at DESC);
