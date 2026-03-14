// Package database provides SQLite initialisation and schema management.
package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'rover',
    enabled       INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS mountpoints (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    name             TEXT    NOT NULL UNIQUE,
    description      TEXT    NOT NULL DEFAULT '',
    enabled          INTEGER NOT NULL DEFAULT 1,
    format           TEXT    NOT NULL DEFAULT 'RTCM3',
    source_auth_mode TEXT    NOT NULL DEFAULT 'user_binding',
    source_secret_hash TEXT,
    write_queue      INTEGER,
    write_timeout_ms INTEGER
);

CREATE TABLE IF NOT EXISTS user_mountpoint_bindings (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mountpoint_id INTEGER NOT NULL REFERENCES mountpoints(id) ON DELETE CASCADE,
    permission    TEXT    NOT NULL DEFAULT 'subscribe',
    UNIQUE(user_id, mountpoint_id, permission)
);
`

// Open opens (or creates) a SQLite database at path and ensures the schema
// is up to date.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return db, nil
}
