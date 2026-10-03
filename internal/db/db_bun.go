package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type BunDB struct {
	DB *bun.DB
}

func Open(ctx context.Context, dsn string) (*BunDB, error) {
	sqldb, err := sql.Open(sqliteshim.ShimName, dsn)
	if err != nil {
		return nil, err
	}
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	if err := db.PingContext(ctx); err != nil {
		return nil, err
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
