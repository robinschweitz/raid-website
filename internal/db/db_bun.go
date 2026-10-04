package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type BunDB struct {
	DB *bun.DB
}

func Open(ctx context.Context, dsn string) (*BunDB, error) {
	// modernc sorts pragmas, so remove conflicting values instead of relying on order.
	path, query, _ := strings.Cut(dsn, "?")
	options, err := url.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("parse database options: %w", err)
	}
	pragmas := options["_pragma"][:0]
	for _, pragma := range options["_pragma"] {
		name := strings.ToLower(strings.TrimSpace(pragma))
		end := strings.IndexAny(name, "(=;")
		if end >= 0 {
			name = name[:end]
		}
		name = strings.NewReplacer("\"", "", "'", "", "`", "", "[", "", "]", "", " ", "", "\t", "", "\r", "", "\n", "").Replace(name)
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[dot+1:]
		}
		if name != "foreign_keys" {
			pragmas = append(pragmas, pragma)
		}
	}
	options["_pragma"] = append(pragmas, "foreign_keys(1)")
	dsn = path + "?" + options.Encode()
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	if err != nil {
		return nil, err
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	if err := db.PingContext(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("initialize database: %w", err), db.Close())
	}
	return &BunDB{DB: db}, nil
}

func (b *BunDB) Close() error {
	if b == nil || b.DB == nil {
		return nil
	}
	return b.DB.Close()
}

func NowUTC() time.Time {
	return time.Now().UTC()
}
