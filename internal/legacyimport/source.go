package legacyimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrSourceTooLarge indicates a source tab exceeds the 8 MiB CSV limit.
var ErrSourceTooLarge = errors.New("import source tab exceeds 8 MiB limit")

// SheetData holds the raw CSV bytes for each imported tab plus a stable hash.
type SheetData struct {
	Raid      []byte
	Loot      []byte
	Character []byte
	Points    []byte
	Setup     []byte
	Hash      string
}

// Fetcher retrieves the source tabs for a spreadsheet id.
type Fetcher interface {
	Fetch(ctx context.Context, sheetID string) (SheetData, error)
}

// SheetNames configures the tab names to import.
type SheetNames struct {
	Raid      string
	Loot      string
	Character string
	Points    string
	Setup     string
}

func DefaultSheetNames() SheetNames {
	return SheetNames{
		Raid:      "Raid Archive",
		Loot:      "Loot Archive",
		Character: "Charaktere",
		Points:    "Punkte",
		Setup:     "ICC25 Raidsetup",
	}
}

// GoogleFetcher downloads tabs as CSV via the public gviz endpoint.
type GoogleFetcher struct {
	Names   SheetNames
	BaseURL string
	Client  *http.Client
}

func (g GoogleFetcher) Fetch(ctx context.Context, sheetID string) (SheetData, error) {
	names := g.Names
	if names.Raid == "" {
		names = DefaultSheetNames()
	}
	base := g.BaseURL
	if base == "" {
		base = "https://docs.google.com"
	}
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	fetch := func(name, columns string) ([]byte, error) {
		u := fmt.Sprintf("%s/spreadsheets/d/%s/gviz/tq?tqx=out:csv&sheet=%s",
			base, url.PathEscape(sheetID), url.QueryEscape(name))
		if columns != "" {
			u += "&range=" + url.QueryEscape(columns) + "&headers=1"
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %q: %w", name, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %q: unexpected status %d", name, resp.StatusCode)
		}
		const limit = 8 << 20
		data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			return nil, fmt.Errorf("fetch %q: %w", name, err)
		}
		if len(data) > limit {
			return nil, fmt.Errorf("fetch %q: %w", name, ErrSourceTooLarge)
		}
		return data, nil
	}

	raid, err := fetch(names.Raid, "")
	if err != nil {
		return SheetData{}, err
	}
	loot, err := fetch(names.Loot, "")
	if err != nil {
		return SheetData{}, err
	}
	chars, err := fetch(names.Character, "")
	if err != nil {
		return SheetData{}, err
	}
	points, err := fetch(names.Points, "")
	if err != nil {
		return SheetData{}, err
	}

	var setup []byte
	if names.Setup != "" {
		setup, err = fetch(names.Setup, "A:F")
		if err != nil {
			return SheetData{}, err
		}
	}
	h := sha256.New()
	for _, part := range [][]byte{raid, loot, chars, points, setup} {
		_, _ = h.Write(part)
		_, _ = h.Write([]byte{0})
	}
	return SheetData{
		Raid:      raid,
		Loot:      loot,
		Character: chars,
		Points:    points,
		Setup:     setup,
		Hash:      hex.EncodeToString(h.Sum(nil)),
	}, nil
}
