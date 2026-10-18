// Command sync-addons copies RaidLedger and RaidWishlist from a checkout of
// robinschweitz/wow-addons into internal/addons/bundle, which the website
// embeds and offers as downloads (ADR 085). Run it from the repository root:
//
//	go run ./cmd/sync-addons ../wow-addons
//
// then rebuild, check the addons page and commit the bundle.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"raid-website/internal/addons"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/sync-addons <wow-addons checkout>")
		os.Exit(2)
	}
	results, err := addons.Sync(os.Args[1], filepath.Join("internal", "addons", "bundle"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync-addons:", err)
		os.Exit(1)
	}
	for _, result := range results {
		old := result.OldVersion
		if old == "" {
			old = "(none)"
		}
		fmt.Printf("%s: %s -> %s (%d files)\n", result.Name, old, result.NewVersion, len(result.CopiedFiles))
	}
}
