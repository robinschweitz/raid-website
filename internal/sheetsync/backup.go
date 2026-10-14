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

// VerifyBackup ensures that a separate, accessible workbook copy exists before
// any automatic database merge or Sheet export. It never writes to the backup.
// The contents of the backup are not expected to track later source edits.
func (g *GoogleWriter) VerifyBackup(ctx context.Context, originalID, backupID string) error {
	if strings.TrimSpace(originalID) == "" || strings.TrimSpace(backupID) == "" || originalID == backupID {
		return errors.New("a separate backup spreadsheet ID is required")
	}
	token, err := g.token(ctx)
	if err != nil {
		return err
	}
	read := func(id string) ([]sheetTab, error) {
		var response struct {
			Sheets []struct {
				Properties sheetTab `json:"properties"`
			} `json:"sheets"`
		}
		path := url.PathEscape(id) + "?fields=sheets(properties(title,sheetId))"
		if err := g.request(ctx, token, http.MethodGet, path, nil, &response); err != nil {
			return nil, err
		}
		tabs := make([]sheetTab, len(response.Sheets))
		for i, s := range response.Sheets {
			tabs[i] = s.Properties
		}
		return tabs, nil
	}
	original, err := read(originalID)
	if err != nil {
		return fmt.Errorf("read original sheet metadata: %w", err)
	}
	backup, err := read(backupID)
	if err != nil {
		return fmt.Errorf("read backup sheet metadata: %w", err)
	}
	if err := matchingBackupTabs(original, backup); err != nil {
		return fmt.Errorf("verify sheet backup: %w", err)
	}
	return nil
}
