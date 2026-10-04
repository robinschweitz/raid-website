package db

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestForeignKeysOnEveryConnection(t *testing.T) {
	for i, query := range []string{
		"",
		"?_pragma=foreign_keys(0)",
		"?_pragma=foreign_keys%3DOFF&_pragma=foreign_keys(1)",
		"?_pragma=FOREIGN_KEYS(OFF)&_pragma=busy_timeout(4321)&_pragma=journal_mode(WAL)&mode=rwc",
		"?_pragma=main.foreign_keys(OFF)",
		"?_pragma=%22main%22%20.%20%22foreign_keys%22(OFF)",
	} {
		t.Run(fmt.Sprintf("dsn_%d", i), func(t *testing.T) {
			ctx := context.Background()
			bdb, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "fk.db")+query)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := bdb.Close(); err != nil {
					t.Error(err)
				}
			})
			for _, statement := range []string{
				"CREATE TABLE parent (id INTEGER PRIMARY KEY)",
				"CREATE TABLE child (parent_id INTEGER REFERENCES parent(id))",
			} {
				if _, err := bdb.DB.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			for connection := 0; connection < 2; connection++ {
				var enabled int
				if err := bdb.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
					t.Fatalf("connection %d: foreign_keys = %d, error %v", connection, enabled, err)
				}
				if _, err := bdb.DB.ExecContext(ctx, "INSERT INTO child VALUES (999)"); err == nil {
					t.Fatalf("connection %d accepted orphan", connection)
				}
				if connection == 0 {
					conn, err := bdb.DB.DB.Conn(ctx)
					if err != nil {
						t.Fatal(err)
					}
					// ErrBadConn discards the physical connection; the next query must open a new one.
					if err := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
						t.Fatalf("discard connection: %v", err)
					}
					_ = conn.Close() // Raw has already closed the discarded connection.
				}
			}
			if query == "?_pragma=FOREIGN_KEYS(OFF)&_pragma=busy_timeout(4321)&_pragma=journal_mode(WAL)&mode=rwc" {
				var timeout int
				var journal string
				if err := bdb.DB.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 4321 {
					t.Fatalf("busy_timeout = %d, error %v", timeout, err)
				}
				if err := bdb.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || journal != "wal" {
					t.Fatalf("journal_mode = %q, error %v", journal, err)
				}
			}
		})
	}
}

func TestOpenInitializationFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if bdb, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "cancelled.db")); err == nil || bdb != nil {
		t.Fatalf("Open = %v, %v; want nil database and error", bdb, err)
	}
	if bdb, err := Open(context.Background(), "file:test.db?_pragma=%ZZ"); err == nil || bdb != nil {
		t.Fatalf("invalid options: Open = %v, %v", bdb, err)
	}
}
