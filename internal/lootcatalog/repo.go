package lootcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/uptrace/bun"
)

// Repo owns all catalog persistence and transaction boundaries.
type Repo struct{ db bun.IDB }

// NewRepo constructs a catalog repository.
func NewRepo(db bun.IDB) *Repo { return &Repo{db: db} }

// WithTx binds a repository to an existing transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo { return NewRepo(tx) }

// List reads all catalog records with deterministic ordering and nonnil arrays.
func (r *Repo) List(ctx context.Context) (Catalog, error) {
	if _, inTransaction := r.db.(bun.Tx); inTransaction {
		return r.list(ctx)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("begin catalog read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// All arrays must come from one snapshot when an import commits concurrently.
	catalog, err := r.WithTx(tx).list(ctx)
	if err != nil {
		return catalog, err
	}
	if err := tx.Commit(); err != nil {
		return catalog, fmt.Errorf("commit catalog read: %w", err)
	}
	return catalog, nil
}

func (r *Repo) list(ctx context.Context) (Catalog, error) {
	c := Catalog{Sources: []Provenance{}, Instances: []Instance{}, Bosses: []Boss{}, Items: []Item{}, Drops: []Drop{}}
	var metadata []struct{ Metadata string }
	if err := r.db.NewRaw("SELECT metadata FROM loot_catalog_sources ORDER BY source_id, revision").Scan(ctx, &metadata); err != nil {
		return c, fmt.Errorf("list catalog sources: %w", err)
	}
	for _, row := range metadata {
		var p Provenance
		if err := json.Unmarshal([]byte(row.Metadata), &p); err != nil {
			return c, fmt.Errorf("decode catalog provenance: %w", err)
		}
		if p.Notices == nil {
			p.Notices = []string{}
		}
		c.Sources = append(c.Sources, p)
	}
	if err := r.db.NewRaw("SELECT id, name FROM loot_catalog_instances ORDER BY id").Scan(ctx, &c.Instances); err != nil {
		return c, fmt.Errorf("list catalog instances: %w", err)
	}
	if err := r.db.NewRaw("SELECT id, instance_id, name FROM loot_catalog_bosses ORDER BY id").Scan(ctx, &c.Bosses); err != nil {
		return c, fmt.Errorf("list catalog bosses: %w", err)
	}
	if err := r.db.NewRaw("SELECT item_id, name, binding, quality FROM loot_catalog_items ORDER BY item_id").Scan(ctx, &c.Items); err != nil {
		return c, fmt.Errorf("list catalog items: %w", err)
	}
	if err := r.db.NewRaw("SELECT boss_id, item_id, raid_size, difficulty, loot_mode FROM loot_catalog_drops ORDER BY boss_id, item_id, raid_size, difficulty, loot_mode").Scan(ctx, &c.Drops); err != nil {
		return c, fmt.Errorf("list catalog drops: %w", err)
	}
	return c, nil
}

// Import compares every record before inserting anything; conflicts reject the whole merge.
func (r *Repo) Import(ctx context.Context, d Dataset, report Report, merge bool) (Report, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin catalog import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	repo := r.WithTx(tx)
	if merge {
		// A harmless preview metadata write acquires the SQLite writer lock before comparison.
		result, err := tx.ExecContext(ctx, "UPDATE loot_catalog_previews SET created_at = created_at WHERE source_id = ? AND revision = ? AND raw_hash = ?", d.Source.ID, d.Source.Revision, report.RawHash)
		if err != nil {
			return report, fmt.Errorf("check catalog preview: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return report, err
		}
		if count == 0 {
			return report, ErrPreviewRequired
		}
	}
	existing, err := repo.List(ctx)
	if err != nil {
		return report, err
	}
	for _, source := range existing.Sources {
		if source.Source.ID == d.Source.ID && source.Source.Revision == d.Source.Revision && source.RawHash != report.RawHash {
			report.Conflicts = append(report.Conflicts, "source: Revision bereits mit anderem Hash vorhanden")
		}
	}
	// Ownership is checked even when records are otherwise byte-for-byte identical.
	owners := map[string]string{}
	var ownership []struct {
		Kind     string
		ID       string
		SourceID string
	}
	if err := tx.NewRaw("SELECT 'instance' AS kind, id, source_id FROM loot_catalog_instances UNION ALL SELECT 'boss', id, source_id FROM loot_catalog_bosses UNION ALL SELECT 'item', item_id, source_id FROM loot_catalog_items").Scan(ctx, &ownership); err != nil {
		return report, fmt.Errorf("load catalog ownership: %w", err)
	}
	for _, row := range ownership {
		owners[row.Kind+":"+row.ID] = row.SourceID
	}
	instances := map[string]Instance{}
	for _, i := range existing.Instances {
		instances[i.ID] = i
	}
	bosses := map[string]Boss{}
	for _, b := range existing.Bosses {
		bosses[b.ID] = b
	}
	items := map[string]Item{}
	for _, i := range existing.Items {
		items[i.ItemID] = i
	}
	drops := map[string]bool{}
	for _, drop := range existing.Drops {
		drops[dropKey(drop)] = true
	}
	newInstances, newBosses, newItems, newDrops := []Instance{}, []Boss{}, []Item{}, []Drop{}
	compare := func(kind, id string, old, incoming any) {
		if owners[kind+":"+id] != d.Source.ID || !reflect.DeepEqual(old, incoming) {
			report.Conflicts = append(report.Conflicts, kind+" "+id+": Bestand oder Quelle weicht ab")
		} else {
			report.Skipped++
		}
	}
	for _, i := range d.Instances {
		if old, ok := instances[i.ID]; ok {
			compare("instance", i.ID, old, i)
		} else {
			newInstances = append(newInstances, i)
		}
	}
	for _, b := range d.Bosses {
		if old, ok := bosses[b.ID]; ok {
			compare("boss", b.ID, old, b)
		} else {
			newBosses = append(newBosses, b)
		}
	}
	for _, i := range d.Items {
		if old, ok := items[i.ItemID]; ok {
			compare("item", i.ItemID, old, i)
		} else {
			newItems = append(newItems, i)
		}
	}
	for _, drop := range d.Drops {
		if drops[dropKey(drop)] {
			report.Skipped++
		} else {
			newDrops = append(newDrops, drop)
		}
	}
	if len(report.Conflicts) != 0 {
		return report, ErrConflict
	}
	if !merge {
		if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_previews (source_id, revision, raw_hash, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING", d.Source.ID, d.Source.Revision, report.RawHash, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return report, fmt.Errorf("save catalog preview: %w", err)
		}
	} else {
		for _, i := range newInstances {
			if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_instances (id, name, source_id) VALUES (?, ?, ?)", i.ID, i.Name, d.Source.ID); err != nil {
				return report, fmt.Errorf("insert catalog instance: %w", err)
			}
		}
		for _, b := range newBosses {
			if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_bosses (id, instance_id, name, source_id) VALUES (?, ?, ?, ?)", b.ID, b.InstanceID, b.Name, d.Source.ID); err != nil {
				return report, fmt.Errorf("insert catalog boss: %w", err)
			}
		}
		for _, i := range newItems {
			if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_items (item_id, name, binding, quality, source_id) VALUES (?, ?, ?, ?, ?)", i.ItemID, i.Name, i.Binding, i.Quality, d.Source.ID); err != nil {
				return report, fmt.Errorf("insert catalog item: %w", err)
			}
		}
		for _, drop := range newDrops {
			if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_drops (boss_id, item_id, raid_size, difficulty, loot_mode, source_id) VALUES (?, ?, ?, ?, ?, ?)", drop.BossID, drop.ItemID, drop.RaidSize, drop.Difficulty, drop.LootMode, d.Source.ID); err != nil {
				return report, fmt.Errorf("insert catalog drop: %w", err)
			}
		}
		metadata, err := json.Marshal(Provenance{Source: d.Source, RawHash: report.RawHash, Notices: d.Notices})
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_sources (source_id, revision, raw_hash, metadata) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING", d.Source.ID, d.Source.Revision, report.RawHash, string(metadata)); err != nil {
			return report, fmt.Errorf("save catalog source: %w", err)
		}
		report.Merged = true
	}
	report.Inserted = len(newInstances) + len(newBosses) + len(newItems) + len(newDrops)
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("commit catalog import: %w", err)
	}
	return report, nil
}
