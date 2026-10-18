// Package addons serves the guild's WoW 3.3.5a addons (RaidLedger,
// RaidWishlist) as ready-to-unzip downloads (ADR 085).
//
// The addon files are bundled in this repository under bundle/<Name>/ and
// embedded into the binary; the robinschweitz/wow-addons repository is their
// source. `go run ./cmd/sync-addons <wow-addons checkout>` copies a new version
// in. Each download is a zip with exactly one top-level folder, the addon's
// name, holding the .toc and the files it lists, so it can be unzipped straight
// into Interface\AddOns.
package addons

import (
	"archive/zip"
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
)

//go:embed bundle
var bundleFS embed.FS

// Names are the bundled addons, in the order the website lists them.
var Names = []string{"RaidLedger", "RaidWishlist"}

var (
	// ErrUnknownAddon means a download for an addon that is not bundled.
	ErrUnknownAddon = errors.New("unknown addon")
	// ErrInvalidBundle means a bundled addon is incomplete or its .toc is unusable.
	ErrInvalidBundle = errors.New("invalid addon bundle")
)

// zipTime is the modification time of every zip entry, so a download only
// changes when the addon files change.
var zipTime = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// Addon is one bundled addon with its built zip.
type Addon struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Version     string   `json:"version"`
	Interface   string   `json:"interface"`
	Notes       string   `json:"notes"`
	Files       []string `json:"files"`
	FileName    string   `json:"file_name"`
	SizeBytes   int      `json:"size_bytes"`
	DownloadURL string   `json:"download_url"`
	zip         []byte
}

// Zip returns the addon's download.
func (a Addon) Zip() []byte { return a.zip }

// Catalog holds every bundled addon. It is built once at startup.
type Catalog struct {
	addons []Addon
}

// Load reads every bundled addon and builds its zip. A missing file, a .toc
// without version or a file the .toc does not list stops startup.
func Load() (*Catalog, error) {
	root, err := fs.Sub(bundleFS, "bundle")
	if err != nil {
		return nil, err
	}
	return LoadFS(root)
}

// LoadFS is Load for an arbitrary bundle directory with one folder per addon.
func LoadFS(root fs.FS) (*Catalog, error) {
	catalog := &Catalog{}
	for _, name := range Names {
		addon, err := loadAddon(root, name)
		if err != nil {
			return nil, err
		}
		catalog.addons = append(catalog.addons, addon)
	}
	return catalog, nil
}

// List returns the bundled addons in website order.
func (c *Catalog) List() []Addon {
	return append([]Addon(nil), c.addons...)
}

// Get returns one bundled addon by its folder name.
func (c *Catalog) Get(name string) (Addon, error) {
	for _, addon := range c.addons {
		if addon.Name == name {
			return addon, nil
		}
	}
	return Addon{}, ErrUnknownAddon
}

// Toc is the part of an addon's .toc file the website uses.
type Toc struct {
	Title     string
	Version   string
	Interface string
	Notes     string
	// Files are the addon's code files in load order.
	Files []string
}

// ParseToc reads a WoW .toc file: "## Key: Value" metadata, "#" comments and
// one file per remaining line.
func ParseToc(data []byte) (Toc, error) {
	var toc Toc
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\xef\xbb\xbf"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "##") {
			key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "##")), ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch strings.TrimSpace(key) {
			case "Title":
				toc.Title = value
			case "Version":
				toc.Version = value
			case "Interface":
				toc.Interface = value
			case "Notes":
				toc.Notes = value
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		file := strings.ReplaceAll(line, `\`, "/")
		if file != path.Clean(file) || strings.HasPrefix(file, "/") || strings.HasPrefix(file, "..") {
			return Toc{}, fmt.Errorf("%w: unsafe file path %q", ErrInvalidBundle, line)
		}
		toc.Files = append(toc.Files, file)
	}
	if err := scanner.Err(); err != nil {
		return Toc{}, err
	}
	if toc.Version == "" || toc.Interface == "" || len(toc.Files) == 0 {
		return Toc{}, fmt.Errorf("%w: .toc needs Version, Interface and at least one file", ErrInvalidBundle)
	}
	return toc, nil
}

func loadAddon(root fs.FS, name string) (Addon, error) {
	tocName := name + ".toc"
	data, err := fs.ReadFile(root, path.Join(name, tocName))
	if err != nil {
		return Addon{}, fmt.Errorf("%w: %s: %v", ErrInvalidBundle, name, err)
	}
	toc, err := ParseToc(data)
	if err != nil {
		return Addon{}, fmt.Errorf("%s: %w", name, err)
	}
	files := append([]string{tocName}, toc.Files...)
	if err := requireExactFiles(root, name, files); err != nil {
		return Addon{}, err
	}
	archive, err := buildZip(root, name, files)
	if err != nil {
		return Addon{}, err
	}
	title := toc.Title
	if title == "" {
		title = name
	}
	fileName := fmt.Sprintf("%s-%s.zip", name, toc.Version)
	return Addon{
		Name:        name,
		Title:       title,
		Version:     toc.Version,
		Interface:   toc.Interface,
		Notes:       toc.Notes,
		Files:       files,
		FileName:    fileName,
		SizeBytes:   len(archive),
		DownloadURL: "/api/v1/addons/" + name + "/download",
		zip:         archive,
	}, nil
}

// requireExactFiles makes sure the bundle folder holds the .toc's files and
// nothing else, so a stale file from an older version never ships.
func requireExactFiles(root fs.FS, name string, files []string) error {
	listed := map[string]bool{}
	for _, file := range files {
		listed[file] = true
		if _, err := fs.Stat(root, path.Join(name, file)); err != nil {
			return fmt.Errorf("%w: %s lists %s, which is missing", ErrInvalidBundle, name, file)
		}
	}
	return fs.WalkDir(root, name, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := strings.TrimPrefix(p, name+"/")
		if !listed[relative] {
			return fmt.Errorf("%w: %s holds %s, which its .toc does not list", ErrInvalidBundle, name, relative)
		}
		return nil
	})
}

func buildZip(root fs.FS, name string, files []string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		data, err := fs.ReadFile(root, path.Join(name, file))
		if err != nil {
			return nil, err
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name + "/" + file, Method: zip.Deflate, Modified: zipTime})
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
