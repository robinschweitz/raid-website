package addons

import (
	"fmt"
	"os"
	"path/filepath"
)

// SyncResult says which version of an addon a sync replaced.
type SyncResult struct {
	Name        string
	OldVersion  string
	NewVersion  string
	CopiedFiles []string
}

// Sync copies every bundled addon from a wow-addons checkout (source/<Name>/)
// into the bundle directory: the .toc, the files it lists and, if present, its
// LICENSE (ADR 108), nothing else.
// Files of the old version that the new .toc no longer lists are removed, so
// the bundle always matches its .toc. The source is checked completely before
// anything is written.
func Sync(source, bundle string) ([]SyncResult, error) {
	type planned struct {
		name  string
		toc   Toc
		files []string
	}
	var plan []planned
	for _, name := range Names {
		tocName := name + ".toc"
		data, err := os.ReadFile(filepath.Join(source, name, tocName))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrInvalidBundle, name, err)
		}
		toc, err := ParseToc(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files := append([]string{tocName}, toc.Files...)
		if _, err := os.Stat(filepath.Join(source, name, LicenseFile)); err == nil {
			files = append(files, LicenseFile)
		}
		for _, file := range files {
			if _, err := os.Stat(filepath.Join(source, name, filepath.FromSlash(file))); err != nil {
				return nil, fmt.Errorf("%w: %s lists %s, which is missing in %s", ErrInvalidBundle, name, file, source)
			}
		}
		plan = append(plan, planned{name: name, toc: toc, files: files})
	}

	results := make([]SyncResult, 0, len(plan))
	for _, entry := range plan {
		target := filepath.Join(bundle, entry.name)
		result := SyncResult{Name: entry.name, NewVersion: entry.toc.Version, CopiedFiles: entry.files}
		if data, err := os.ReadFile(filepath.Join(target, entry.name+".toc")); err == nil {
			if old, err := ParseToc(data); err == nil {
				result.OldVersion = old.Version
			}
		}
		if err := os.RemoveAll(target); err != nil {
			return nil, fmt.Errorf("clear %s: %w", target, err)
		}
		for _, file := range entry.files {
			data, err := os.ReadFile(filepath.Join(source, entry.name, filepath.FromSlash(file)))
			if err != nil {
				return nil, err
			}
			destination := filepath.Join(target, filepath.FromSlash(file))
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(destination, data, 0o644); err != nil {
				return nil, err
			}
		}
		results = append(results, result)
	}
	return results, nil
}
