package accountmerge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

// ErrExportForbidden is returned when a non-Admin requests personal data.
var ErrExportForbidden = errors.New("personal-data export requires admin")

// PlayerExport is the machine-readable data disclosure of FR-30.7.
type PlayerExport struct {
	ExportedAt time.Time                   `json:"exported_at"`
	Account    *player.Player              `json:"account"`
	Sections   map[string][]map[string]any `json:"sections"`
}

// Export returns a consistent snapshot without authentication secrets and audits access.
func (s *Service) Export(ctx context.Context, actorID, playerID int64) (*PlayerExport, error) {
	var result *PlayerExport
	err := s.repo.InTransaction(ctx, func(repo *Repo) error {
		actor, err := repo.GetPlayer(ctx, actorID)
		if err != nil {
			return err
		}
		if actor.Role != validation.RoleAdmin || !actor.Active {
			return ErrExportForbidden
		}
		account, err := repo.GetPlayer(ctx, playerID)
		if err != nil {
			return err
		}
		sections, err := repo.ExportSections(ctx, playerID)
		if err != nil {
			return err
		}
		for _, row := range sections["audit"] {
			for _, field := range []string{"before_json", "after_json"} {
				if value, ok := row[field].(string); ok {
					row[field] = redactExportSnapshot(value)
				}
			}
		}
		if err := repo.AuditExport(ctx, actorID, playerID); err != nil {
			return err
		}
		result = &PlayerExport{ExportedAt: time.Now().UTC(), Account: account, Sections: sections}
		return nil
	})
	return result, err
}

func redactExportSnapshot(raw string) string {
	if raw == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return ""
	}
	var redact func(any)
	redact = func(v any) {
		switch data := v.(type) {
		case map[string]any:
			for key, child := range data {
				switch strings.ToLower(key) {
				case "password", "password_hash", "totp_secret", "secret", "code", "plain_code", "code_hash", "recovery_codes", "token", "session_token", "token_hash", "challenge_hash", "session_hash", "credential_json", "session_json", "raw_profile_json", "external_profile_json":
					delete(data, key)
				default:
					redact(child)
				}
			}
		case []any:
			for _, child := range data {
				redact(child)
			}
		}
	}
	redact(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
