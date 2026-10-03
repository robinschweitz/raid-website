package legacyimport

import (
	"context"

	"github.com/uptrace/bun"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

func (r *Repo) Insert(ctx context.Context, imp *Import) error {
	_, err := r.db.NewInsert().Model(imp).Exec(ctx)
	return err
}

func (r *Repo) Get(ctx context.Context, id int64) (*Import, error) {
	imp := new(Import)
	err := r.db.NewSelect().Model(imp).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imp, nil
}

func (r *Repo) FindByHash(ctx context.Context, source, hash string) (*Import, error) {
	imp := new(Import)
	err := r.db.NewSelect().Model(imp).
		Where("source = ?", source).
		Where("raw_hash = ?", hash).
		Where("status = ?", "MERGED").
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imp, nil
}

func (r *Repo) List(ctx context.Context) ([]Import, error) {
	var imports []Import
	err := r.db.NewSelect().Model(&imports).Order("id DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return imports, nil
}
