package addons

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestParseToc(t *testing.T) {
	toc, err := ParseToc([]byte("\xef\xbb\xbf## Interface: 30300\r\n## Title: RaidLedger\r\n## Notes: Loot\r\n## Version: 2.3.0\r\n# comment\r\n\r\nLocales.lua\r\nsub\\Raid.lua\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Toc{Title: "RaidLedger", Version: "2.3.0", Interface: "30300", Notes: "Loot", Files: []string{"Locales.lua", "sub/Raid.lua"}}
	if !reflect.DeepEqual(toc, want) {
		t.Fatalf("toc %+v, want %+v", toc, want)
	}
	for _, bad := range []string{"## Interface: 30300\nA.lua", "## Version: 1\nA.lua", "## Version: 1\n## Interface: 30300\n", "## Version: 1\n## Interface: 30300\n../evil.lua"} {
		if _, err := ParseToc([]byte(bad)); !errors.Is(err, ErrInvalidBundle) {
			t.Fatalf("%q: err %v, want ErrInvalidBundle", bad, err)
		}
	}
}

// The real bundle must load: every .toc complete, no stray files, and each
// zip holds exactly one folder named after the addon.
func TestBundledAddonsLoadAsInstallableZips(t *testing.T) {
	catalog, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	list := catalog.List()
	if len(list) != len(Names) {
		t.Fatalf("addons %d, want %d", len(list), len(Names))
	}
	for i, addon := range list {
		if addon.Name != Names[i] || addon.Version == "" || addon.Interface != "30300" {
			t.Fatalf("addon %+v", addon)
		}
		if addon.FileName != addon.Name+"-"+addon.Version+".zip" || addon.DownloadURL != "/api/v1/addons/"+addon.Name+"/download" || addon.SizeBytes != len(addon.Zip()) {
			t.Fatalf("download fields %+v", addon)
		}
		reader, err := zip.NewReader(bytes.NewReader(addon.Zip()), int64(len(addon.Zip())))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, file := range reader.File {
			names = append(names, file.Name)
		}
		var want []string
		for _, file := range addon.Files {
			want = append(want, addon.Name+"/"+file)
		}
		if !reflect.DeepEqual(names, want) || names[0] != addon.Name+"/"+addon.Name+".toc" {
			t.Fatalf("%s zip entries %v, want %v", addon.Name, names, want)
		}
	}
	if _, err := catalog.Get("RaidSync"); !errors.Is(err, ErrUnknownAddon) {
		t.Fatalf("unknown addon: %v", err)
	}
}

func TestLoadRejectsIncompleteOrStaleBundles(t *testing.T) {
	toc := func(name string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte("## Interface: 30300\n## Version: 1.0\n" + name + ".lua\n")}
	}
	complete := fstest.MapFS{
		"RaidLedger/RaidLedger.toc":     toc("RaidLedger"),
		"RaidLedger/RaidLedger.lua":     {Data: []byte("-- ledger")},
		"RaidWishlist/RaidWishlist.toc": toc("RaidWishlist"),
		"RaidWishlist/RaidWishlist.lua": {Data: []byte("-- wishlist")},
	}
	if _, err := LoadFS(complete); err != nil {
		t.Fatalf("complete bundle: %v", err)
	}
	missing := fstest.MapFS{}
	stale := fstest.MapFS{"RaidLedger/Old.lua": {Data: []byte("-- old")}}
	for key, file := range complete {
		stale[key] = file
		if key != "RaidWishlist/RaidWishlist.lua" {
			missing[key] = file
		}
	}
	for label, bundle := range map[string]fstest.MapFS{"missing file": missing, "stale file": stale} {
		if _, err := LoadFS(bundle); !errors.Is(err, ErrInvalidBundle) {
			t.Fatalf("%s: err %v, want ErrInvalidBundle", label, err)
		}
	}
}

func TestSyncCopiesTocFilesAndRemovesStaleOnes(t *testing.T) {
	source, bundle := t.TempDir(), t.TempDir()
	write := func(root, file, content string) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(source, "RaidLedger/RaidLedger.toc", "## Interface: 30300\n## Version: 2.3.0\nRaidLedger.lua\nMinimap.lua\n")
	write(source, "RaidLedger/RaidLedger.lua", "-- new")
	write(source, "RaidLedger/Minimap.lua", "-- minimap")
	write(source, "RaidLedger/README.md", "docs stay in wow-addons")
	write(source, "RaidLedger/tests/a.spec.lua", "-- spec")
	write(source, "RaidWishlist/RaidWishlist.toc", "## Interface: 30300\n## Version: 1.4.0\nRaidWishlist.lua\n")
	write(source, "RaidWishlist/RaidWishlist.lua", "-- wish")
	write(bundle, "RaidLedger/RaidLedger.toc", "## Interface: 30300\n## Version: 2.1.0\nRaidLedger.lua\nOld.lua\n")
	write(bundle, "RaidLedger/Old.lua", "-- removed in 2.3.0")

	results, err := Sync(source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].OldVersion != "2.1.0" || results[0].NewVersion != "2.3.0" || results[1].OldVersion != "" || results[1].NewVersion != "1.4.0" {
		t.Fatalf("results %+v", results)
	}
	catalog, err := LoadFS(os.DirFS(bundle))
	if err != nil {
		t.Fatalf("synced bundle does not load: %v", err)
	}
	ledger, _ := catalog.Get("RaidLedger")
	if !reflect.DeepEqual(ledger.Files, []string{"RaidLedger.toc", "RaidLedger.lua", "Minimap.lua"}) {
		t.Fatalf("ledger files %v", ledger.Files)
	}
	reader, _ := zip.NewReader(bytes.NewReader(ledger.Zip()), int64(len(ledger.Zip())))
	content, err := reader.File[1].Open()
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := io.ReadAll(content); string(data) != "-- new" {
		t.Fatalf("zipped RaidLedger.lua %q", data)
	}

	// A source with a missing file is rejected before anything is written.
	if err := os.Remove(filepath.Join(source, "RaidWishlist", "RaidWishlist.lua")); err != nil {
		t.Fatal(err)
	}
	write(source, "RaidLedger/RaidLedger.toc", "## Interface: 30300\n## Version: 9.9.9\nRaidLedger.lua\n")
	if _, err := Sync(source, bundle); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("broken source: %v", err)
	}
	if catalog, err := LoadFS(os.DirFS(bundle)); err != nil {
		t.Fatal(err)
	} else if ledger, _ := catalog.Get("RaidLedger"); ledger.Version != "2.3.0" {
		t.Fatalf("broken sync changed the bundle to %s", ledger.Version)
	}
}

// ADR 108: an addon's LICENSE is the one file outside its .toc that ships,
// in the bundle and in the download zip; any other unlisted file still fails.
func TestLicenseShipsInBundleAndZip(t *testing.T) {
	source, bundle := t.TempDir(), t.TempDir()
	write := func(root, file, content string) {
		t.Helper()
		target := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(source, "RaidLedger/RaidLedger.toc", "## Interface: 30300\n## Version: 2.5.1\nRaidLedger.lua\n")
	write(source, "RaidLedger/RaidLedger.lua", "-- ledger")
	write(source, "RaidWishlist/RaidWishlist.toc", "## Interface: 30300\n## Version: 2.3.0\nRaidWishlist.lua\n")
	write(source, "RaidWishlist/RaidWishlist.lua", "-- wish")
	write(source, "RaidWishlist/LICENSE", "GNU GENERAL PUBLIC LICENSE Version 2")
	write(bundle, "RaidLedger/LICENSE", "a license the source no longer has")

	if _, err := Sync(source, bundle); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "RaidLedger", "LICENSE")); !os.IsNotExist(err) {
		t.Fatalf("a LICENSE the source dropped must be removed: %v", err)
	}
	catalog, err := LoadFS(os.DirFS(bundle))
	if err != nil {
		t.Fatalf("bundle with LICENSE does not load: %v", err)
	}
	wish, _ := catalog.Get("RaidWishlist")
	reader, err := zip.NewReader(bytes.NewReader(wish.Zip()), int64(len(wish.Zip())))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range reader.File {
		if file.Name == "RaidWishlist/LICENSE" {
			content, _ := file.Open()
			data, _ := io.ReadAll(content)
			found = string(data) == "GNU GENERAL PUBLIC LICENSE Version 2"
		}
	}
	if !found {
		t.Fatal("the download zip has no RaidWishlist/LICENSE")
	}
	ledger, _ := catalog.Get("RaidLedger")
	reader, _ = zip.NewReader(bytes.NewReader(ledger.Zip()), int64(len(ledger.Zip())))
	for _, file := range reader.File {
		if file.Name == "RaidLedger/LICENSE" {
			t.Fatal("an addon without LICENSE must not get one")
		}
	}

	write(bundle, "RaidWishlist/README.md", "still not shipped")
	if _, err := LoadFS(os.DirFS(bundle)); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("an unlisted README must still fail: %v", err)
	}
}
