package sheetsync

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/csv"
    "encoding/hex"
    "fmt"
    "net/http"
    "net/url"
    "strings"

    "raid-website/internal/legacyimport"
)

// AuthenticatedFetcher reads the original five tabs through the Sheets API.
// Unlike public gviz CSV, it works on private spreadsheets shared with the
// configured service account. Google returns formatted values (German dates).
type AuthenticatedFetcher struct {
    Writer *GoogleWriter
    Names legacyimport.SheetNames
}

func (f AuthenticatedFetcher) Fetch(ctx context.Context, sheetID string) (legacyimport.SheetData, error) {
    if f.Writer == nil { return legacyimport.SheetData{}, fmt.Errorf("google writer required") }
    token, err := f.Writer.token(ctx)
    if err != nil { return legacyimport.SheetData{}, err }
    names := f.Names
    if names.Raid == "" { names = legacyimport.DefaultSheetNames() }
    fetch := func(title, columns string) ([]byte, error) {
        rangeName := "'"+strings.ReplaceAll(title, "'", "''")+"'"
        if columns != "" { rangeName += "!"+columns }
        var data struct { Values [][]string `json:"values"` }
        path := url.PathEscape(sheetID)+"/values/"+url.PathEscape(rangeName)+"?valueRenderOption=FORMATTED_VALUE"
        if err := f.Writer.request(ctx, token, http.MethodGet, path, nil, &data); err != nil { return nil, fmt.Errorf("read %s: %w", title, err) }
        var output bytes.Buffer
        csvWriter := csv.NewWriter(&output)
        for _, values := range data.Values {
            if err := csvWriter.Write(values); err != nil { return nil, err }
            if output.Len() > 8<<20 { return nil, fmt.Errorf("%s: %w", title, legacyimport.ErrSourceTooLarge) }
        }
        csvWriter.Flush()
        if err := csvWriter.Error(); err != nil { return nil, err }
        if output.Len() > 8<<20 { return nil, fmt.Errorf("%s: %w", title, legacyimport.ErrSourceTooLarge) }
        return output.Bytes(), nil
    }
    raid, err := fetch(names.Raid, "")
    if err != nil { return legacyimport.SheetData{}, err }
    loot, err := fetch(names.Loot, "")
    if err != nil { return legacyimport.SheetData{}, err }
    chars, err := fetch(names.Character, "")
    if err != nil { return legacyimport.SheetData{}, err }
    points, err := fetch(names.Points, "")
    if err != nil { return legacyimport.SheetData{}, err }
    setup, err := fetch(names.Setup, "A:F")
    if err != nil { return legacyimport.SheetData{}, err }
    h := sha256.New()
    for _, raw := range [][]byte{raid, loot, chars, points, setup} {
        _, _ = h.Write(raw)
        _, _ = h.Write([]byte{0})
    }
    return legacyimport.SheetData{Raid:raid, Loot:loot, Character:chars, Points:points, Setup:setup, Hash:hex.EncodeToString(h.Sum(nil))}, nil
}
