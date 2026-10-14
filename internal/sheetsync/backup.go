package sheetsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// sheetTab identifies a stable tab from a Google Sheets file copy.
type sheetTab struct {
	Title string `json:"title"`
	ID    int64  `json:"sheetId"`
}

// matchingBackupTabs checks the original imported tables, including their IDs.
// The original may later gain "Website ..." export tabs absent from the backup.
func matchingBackupTabs(original, backup []sheetTab) error {
	// The source workbook has 17 tabs. Website snapshots are added later and
	// must never be required in the immutable pre-sync backup.
	required := []string{
		"Fun Stats", "Saurfang", "Fauldarm", "Raid-Overview",
		"Erklärung zum Sheet", "Raidsetup Orientierungshilfe", "ICC25 Loot",
		"ICC25 Raidsetup", "Raid Archive", "Loot Archive", "Stammies",
		"Punkte", "Charaktere", "Helpertabelle", "ICC10 Raidsetup",
		"09-02-2026-ICC-Raid", "ICC25",
	}
	if len(backup) < len(required) {
		return errors.New("backup does not contain all 17 source tabs")
	}
	toMap := func(tabs []sheetTab) map[string]int64 {
		out := make(map[string]int64, len(tabs))
		for _, t := range tabs {
			out[t.Title] = t.ID
		}
		return out
	}
	orig := toMap(original)
	saved := toMap(backup)
	for _, title := range required {
		a, hasOriginal := orig[title]
		b, hasBackup := saved[title]
		if !hasOriginal || !hasBackup || a != b {
			return fmt.Errorf("backup differs on required tab %q", title)
		}
	}
	return nil
}

// VerifyWorkingCopy allows only the designated copy as the sync target.
func (g *GoogleWriter) VerifyWorkingCopy(ctx context.Context, sheetID string) error {
	const copyID = "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8"
	if strings.TrimSpace(sheetID) != copyID {
		return errors.New("sync must target the authorized working copy, never the original")
	}
	token, err := g.token(ctx)
	if err != nil {
		return err
	}
	var metadata struct {
		Sheets []struct {
			Properties sheetTab `json:"properties"`
		} `json:"sheets"`
	}
	path := url.PathEscape(sheetID) + "?fields=sheets(properties(title,sheetId))"
	if err := g.request(ctx, token, http.MethodGet, path, nil, &metadata); err != nil {
		return err
	}
	var found []sheetTab
	for _, tab := range metadata.Sheets {
		found = append(found, tab.Properties)
	}
	return matchingBackupTabs(found, found)
}
