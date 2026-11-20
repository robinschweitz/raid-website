package supportview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"github.com/uptrace/bun"
	"raid-website/internal/audit"
)

type Repo struct{ DB *bun.DB }
type View struct {
	ID       string `json:"id"`
	Target   int64  `json:"target"`
	Writable bool   `json:"writable"`
}

func digest(cookie string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(cookie))) }
func (r Repo) Start(ctx context.Context, cookie string, actor, target int64) (View, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return View{}, err
	}
	v := View{ID: fmt.Sprintf("%x", key), Target: target}
	err := r.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO support_views(session_hash,view_id,target_id,writable) VALUES(?,?,?,FALSE) ON CONFLICT(session_hash) DO UPDATE SET view_id=excluded.view_id,target_id=excluded.target_id,writable=FALSE", digest(cookie), v.ID, target)
		if err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actor, "player", target, "support_view_started", nil, nil, "")
	})
	return v, err
}
func (r Repo) Load(ctx context.Context, cookie, id string) (View, error) {
	v := View{ID: id}
	err := r.DB.QueryRowContext(ctx, "SELECT target_id,writable FROM support_views WHERE session_hash=? AND view_id=?", digest(cookie), id).Scan(&v.Target, &v.Writable)
	return v, err
}
func (r Repo) Change(ctx context.Context, cookie string, v View, actor int64, end bool) error {
	return r.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		query := "UPDATE support_views SET writable=TRUE WHERE session_hash=? AND view_id=?"
		action := "support_write_enabled"
		if end {
			query = "DELETE FROM support_views WHERE session_hash=? AND view_id=?"
			action = "support_view_ended"
		}
		res, err := tx.ExecContext(ctx, query, digest(cookie), v.ID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		return audit.NewRepo(tx).Record(ctx, actor, "player", v.Target, action, nil, nil, "")
	})
}
