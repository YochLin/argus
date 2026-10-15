package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// New opens the main read-write connection used by the bot process itself.
// busy_timeout is set (see OpenReadOnly/OpenForWrites below) so that a
// second connection to the same file briefly holding a write lock — the MCP
// subprocess's own writable connection, see OpenForWrites — makes this
// connection wait rather than fail outright with "database is locked";
// both sides are low-frequency single-user writers, so contention should be
// rare, but there was no reason to leave the default (no wait at all) now
// that a second writer process exists at all.
func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		return nil, err
	}
	if err := d.seedSP500(); err != nil {
		return nil, err
	}
	if err := d.seedTW150(); err != nil {
		return nil, err
	}
	return d, nil
}

// OpenReadOnly opens a second connection to the SQLite database at path for
// a separate process that must never write to it — the MCP server
// subprocess's read-only query tools (see internal/mcptools and PLAN.md's
// Phase 3.5 "DB 唯讀查詢工具" item), which relaxes that phase's original
// "don't touch the DB at all" decision now that it's clear a
// same-process-family read-only connection carries none of the
// cross-process write-conflict risk that decision was guarding against.
//
// Unlike New, this skips migrate()/seedSP500() (both write) — it assumes
// the schema already exists, which holds in practice since this connection
// is only ever opened after the main bot process has already opened (and
// migrated) the database.
//
// modernc.org/sqlite's "mode=ro" DSN query parameter is silently ignored —
// confirmed against the driver's own test suite (all_test.go's
// TestInMemory: "// This parameter should be ignored"); this pure-Go
// driver always opens with SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE
// regardless of what the DSN says, so "mode=ro" would be a lie here. Real
// enforcement instead comes from `PRAGMA query_only = ON`, applied via the
// driver's `_pragma` DSN parameter: SQLite rejects any write statement on
// a query_only connection with "attempt to write a readonly database",
// independent of how the file was opened. The `file:` prefix is required
// for the driver to honor the query string at all — without it, the
// driver's DSN parser strips everything after `?` before opening.
func OpenReadOnly(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=query_only(1)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// OpenForWrites opens a second, genuinely writable connection to the SQLite
// database at path for a separate process — the MCP server subprocess's
// watchlist write-pilot tools (add_to_watchlist/remove_from_watchlist, see
// internal/mcptools and PLAN.md's Phase 3.5 "watchlist 寫入工具" item).
// Deliberately a distinct connection from OpenReadOnly (not just "the same
// thing minus query_only") so the four pre-existing read-only tools keep
// their hard DB-level guarantee that they can never write, even if a future
// bug tried to route a write through them — only the tools that are
// explicitly meant to mutate data get a handle capable of it.
//
// Like OpenReadOnly, this skips migrate()/seedSP500() (schema ownership
// stays with the main bot process) and sets busy_timeout so a write here
// waits rather than immediately erroring if it briefly races the main
// process's own connection (see New's doc comment).
func OpenForWrites(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

// Backup writes a consistent point-in-time copy of the database to destPath
// via SQLite's VACUUM INTO, which is safe to run against a live database
// (unlike copying the file directly, which can catch it mid-write or miss
// WAL-journaled pages). destPath is bound as a parameter rather than
// interpolated into the SQL string, even though it's caller-controlled
// (config, not user input) — no reason to hand-format a file path into SQL
// when the driver can bind it safely.
func (d *DB) Backup(destPath string) error {
	_, err := d.conn.Exec(`VACUUM INTO ?`, destPath)
	return err
}

func (d *DB) Close() error {
	return d.conn.Close()
}
