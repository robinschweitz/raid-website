package lootcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/uptrace/bun"
)

// Repo owns the catalog foreign-key mirror. Reads never select the mirror, so a
// database row can never offer an alternative to the bundled source.
type Repo struct {
	db bun.IDB
}

// NewRepo constructs a catalog repository.
func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// WithTx binds a repository to an existing transaction.
func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return NewRepo(tx)
}

// List returns the compiled catalog. It never reads the mirror and never fails
// soft: an invalid bundled source is an error, not an empty catalog.
func (r *Repo) List(ctx context.Context) (Catalog, error) {
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	return bundledCatalog()
}

// EnsureBundled mirrors the bundled source so existing and future foreign keys
// resolve, and reports every changed fact. Nothing is deleted: historical rows
// stay as retained targets, and the persisted report is the explicit record.
func (r *Repo) EnsureBundled(ctx context.Context) (InitializationReport, error) {
	catalog, err := r.List(ctx)
	if err != nil {
		return InitializationReport{}, err
	}
	provenance := catalog.Sources[0]
	report := InitializationReport{
		Source:     provenance.Source,
		SourceHash: provenance.RawHash,
		Diffs:      []CatalogDiff{},
		Warnings:   []string{},
	}
	conditional := 0
	for _, drop := range catalog.Drops {
		if _, isConditional, err := sourceGameMode(drop); err != nil {
			return InitializationReport{}, err
		} else if isConditional {
			conditional++
		}
	}
	if conditional > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("%d bedingte Quellbeziehungen: ihre Rohpfade bleiben je Auswahl sichtbar.", conditional))
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return InitializationReport{}, fmt.Errorf("bundled catalog transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Write first so concurrent cold starts queue on the SQLite writer lock
	// instead of reading a mirror another process is still changing.
	if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_initializations (id, source_hash, report, created_at) VALUES (0, 'lock', 'lock', '1970-01-01T00:00:00Z') ON CONFLICT(id) DO UPDATE SET source_hash = excluded.source_hash"); err != nil {
		return InitializationReport{}, fmt.Errorf("bundled catalog writer lock: %w", err)
	}

	previous := map[string]mirrorFact{}
	if err := readMirror(ctx, tx, previous); err != nil {
		return InitializationReport{}, err
	}
	apply := func(kind, key string, value any, query string, args ...any) error {
		old, existed := previous[kind+"/"+key]
		delete(previous, kind+"/"+key)
		incoming := mirrorFact{Value: value, SourceID: provenance.Source.ID}
		if existed && reflect.DeepEqual(old, incoming) {
			report.Unchanged++
			return nil
		}
		diff := CatalogDiff{Kind: kind, Key: key, Action: "insert", Before: json.RawMessage("null")}
		encoded, err := json.Marshal(incoming)
		if err != nil {
			return err
		}
		diff.After = encoded
		if existed {
			report.Updated++
			diff.Action = "update"
			before, err := json.Marshal(old)
			if err != nil {
				return err
			}
			diff.Before = before
		} else {
			report.Inserted++
		}
		report.Diffs = append(report.Diffs, diff)
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("mirror %s %s: %w", kind, key, err)
		}
		return nil
	}

	for _, instance := range catalog.Instances {
		err := apply("instance", instance.ID, instance, "INSERT INTO loot_catalog_instances (id, name, source_id) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET name = excluded.name, source_id = excluded.source_id", instance.ID, instance.Name, provenance.Source.ID)
		if err != nil {
			return InitializationReport{}, err
		}
	}
	for _, boss := range catalog.Bosses {
		err := apply("boss", boss.ID, boss, "INSERT INTO loot_catalog_bosses (id, instance_id, name, source_id) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET instance_id = excluded.instance_id, name = excluded.name, source_id = excluded.source_id", boss.ID, boss.InstanceID, boss.Name, provenance.Source.ID)
		if err != nil {
			return InitializationReport{}, err
		}
	}
	for _, item := range catalog.Items {
		mirrorItem := Item{ItemID: item.ItemID, Name: item.Name, Binding: item.Binding, Quality: item.Quality}
		err := apply("item", item.ItemID, mirrorItem, "INSERT INTO loot_catalog_items (item_id, name, binding, quality, source_id) VALUES (?, ?, ?, ?, ?) ON CONFLICT(item_id) DO UPDATE SET name = excluded.name, binding = excluded.binding, quality = excluded.quality, source_id = excluded.source_id", item.ItemID, item.Name, item.Binding, item.Quality, provenance.Source.ID)
		if err != nil {
			return InitializationReport{}, err
		}
	}
	for _, drop := range catalog.Drops {
		err := apply("drop", dropKey(drop), drop, "INSERT INTO loot_catalog_drops (boss_id, item_id, raid_size, difficulty, loot_mode, source_id) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(boss_id, item_id, raid_size, difficulty, loot_mode) DO UPDATE SET source_id = excluded.source_id", drop.BossID, drop.ItemID, drop.RaidSize, drop.Difficulty, drop.LootMode, provenance.Source.ID)
		if err != nil {
			return InitializationReport{}, err
		}
	}

	keys := make([]string, 0, len(previous))
	for key := range previous {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fact := previous[key]
		encoded, err := json.Marshal(fact)
		if err != nil {
			return InitializationReport{}, err
		}
		report.Retained++
		report.Diffs = append(report.Diffs, CatalogDiff{Kind: "historical", Key: key, Action: "retain", Before: encoded, After: encoded})
	}

	encodedReport, err := json.Marshal(report)
	if err != nil {
		return InitializationReport{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO loot_catalog_initializations (source_hash, report, created_at) VALUES (?, ?, ?)", provenance.RawHash, string(encodedReport), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return InitializationReport{}, fmt.Errorf("store bundled catalog report: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM loot_catalog_initializations WHERE id = 0"); err != nil {
		return InitializationReport{}, fmt.Errorf("release bundled catalog writer lock: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return InitializationReport{}, fmt.Errorf("commit bundled catalog initialization: %w", err)
	}
	return report, nil
}

// mirrorFact is one stored row reduced to the columns the bundled source owns.
type mirrorFact struct {
	Value    any    `json:"value"`
	SourceID string `json:"source_id"`
}

func readMirror(ctx context.Context, tx bun.Tx, into map[string]mirrorFact) error {
	var instances []struct {
		ID       string `bun:"id"`
		Name     string `bun:"name"`
		SourceID string `bun:"source_id"`
	}
	if err := tx.NewRaw("SELECT id, name, source_id FROM loot_catalog_instances").Scan(ctx, &instances); err != nil {
		return err
	}
	var bosses []struct {
		ID         string `bun:"id"`
		InstanceID string `bun:"instance_id"`
		Name       string `bun:"name"`
		SourceID   string `bun:"source_id"`
	}
	if err := tx.NewRaw("SELECT id, instance_id, name, source_id FROM loot_catalog_bosses").Scan(ctx, &bosses); err != nil {
		return err
	}
	var items []struct {
		ItemID   string `bun:"item_id"`
		Name     string `bun:"name"`
		Binding  *int   `bun:"binding"`
		Quality  *int   `bun:"quality"`
		SourceID string `bun:"source_id"`
	}
	if err := tx.NewRaw("SELECT item_id, name, binding, quality, source_id FROM loot_catalog_items").Scan(ctx, &items); err != nil {
		return err
	}
	var drops []DropRow
	if err := tx.NewRaw("SELECT boss_id, item_id, raid_size, difficulty, loot_mode, source_id FROM loot_catalog_drops").Scan(ctx, &drops); err != nil {
		return err
	}
	for _, row := range instances {
		into["instance/"+row.ID] = mirrorFact{Value: Instance{ID: row.ID, Name: row.Name}, SourceID: row.SourceID}
	}
	for _, row := range bosses {
		into["boss/"+row.ID] = mirrorFact{Value: Boss{ID: row.ID, InstanceID: row.InstanceID, Name: row.Name}, SourceID: row.SourceID}
	}
	for _, row := range items {
		into["item/"+row.ItemID] = mirrorFact{Value: Item{ItemID: row.ItemID, Name: row.Name, Binding: row.Binding, Quality: row.Quality}, SourceID: row.SourceID}
	}
	for _, row := range drops {
		drop := Drop{BossID: row.BossID, ItemID: row.ItemID, RaidSize: row.RaidSize, Difficulty: row.Difficulty, LootMode: row.LootMode}
		into["drop/"+dropKey(drop)] = mirrorFact{Value: drop, SourceID: row.SourceID}
	}
	return nil
}

// DropRow is a stored drop relationship with its provenance.
type DropRow struct {
	BossID     string `bun:"boss_id"`
	ItemID     string `bun:"item_id"`
	RaidSize   int    `bun:"raid_size"`
	Difficulty string `bun:"difficulty"`
	LootMode   int    `bun:"loot_mode"`
	SourceID   string `bun:"source_id"`
}
