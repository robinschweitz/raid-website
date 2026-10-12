package registration

import (
	"time"

	"github.com/uptrace/bun"
)

const (
	StatusPending  = "PENDING"
	StatusApproved = "APPROVED"
	StatusRejected = "REJECTED"
	StatusConsumed = "CONSUMED"

	PurposeNewAccount = "NEW_ACCOUNT"
	PurposeClaim      = "CLAIM_CHARACTER"

	ExternalSourceRisingGods = "rising-gods"
)

type Request struct {
	bun.BaseModel `bun:"table:registration_requests"`

	ID                 int64     `bun:"id,pk,autoincrement" json:"id"`
	RequesterPlayerID  *int64    `bun:"requester_player_id,nullzero" json:"requester_player_id"`
	Status             string    `bun:"status,notnull" json:"status"`
	Purpose            string    `bun:"purpose,notnull" json:"purpose"`
	ExternalSource     string    `bun:"external_source,notnull" json:"external_source"`
	ExternalProfileID  string    `bun:"external_profile_id,notnull" json:"external_profile_id"`
	ExternalProfileURL string    `bun:"external_profile_url,notnull" json:"external_profile_url"`
	CharacterName      string    `bun:"character_name,notnull" json:"character_name"`
	Class              string    `bun:"class,notnull" json:"class"`
	MainSpec           string    `bun:"main_spec,notnull" json:"main_spec"`
	SecondSpec         string    `bun:"second_spec,notnull" json:"second_spec"`
	GearScore          *int      `bun:"gear_score,nullzero" json:"gear_score"`
	Level              int       `bun:"level,notnull" json:"level"`
	Race               string    `bun:"race,notnull" json:"race"`
	Faction            string    `bun:"faction,notnull" json:"faction"`
	RawProfileJSON     string    `bun:"raw_profile_json,notnull" json:"raw_profile_json"`
	CreatedAt          time.Time `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt          time.Time `bun:"updated_at,notnull" json:"updated_at"`
}

type Code struct {
	bun.BaseModel `bun:"table:registration_codes"`

	ID                int64      `bun:"id,pk,autoincrement" json:"id"`
	RequestID         int64      `bun:"request_id,notnull" json:"request_id"`
	CodeHash          string     `bun:"code_hash,notnull" json:"-"`
	CreatedByPlayerID int64      `bun:"created_by_player_id,notnull" json:"created_by_player_id"`
	CreatedAt         time.Time  `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt         time.Time  `bun:"expires_at,notnull" json:"expires_at"`
	SentAt            *time.Time `bun:"sent_at,nullzero" json:"sent_at"`
	UsedAt            *time.Time `bun:"used_at,nullzero" json:"used_at"`
	UsedByPlayerID    *int64     `bun:"used_by_player_id,nullzero" json:"used_by_player_id"`
	UsedByUsername    *string    `bun:"used_by_username,scanonly" json:"used_by_username"`
	RevokedAt         *time.Time `bun:"revoked_at,nullzero" json:"revoked_at"`
}

type CodeWithRequest struct {
	bun.BaseModel `bun:"table:registration_codes"`

	Code    `bun:",extend"`
	Request *Request `bun:"rel:belongs-to,join:request_id=id" json:"request"`
}

type ExternalCharacter struct {
	ProfileID  string `json:"profile_id"`
	Name       string `json:"name"`
	Class      string `json:"class"`
	MainSpec   string `json:"main_spec"`
	SecondSpec string `json:"second_spec"`
	GearScore  *int   `json:"gear_score"`
	Level      int    `json:"level"`
	Race       string `json:"race"`
	Faction    string `json:"faction"`
	ProfileURL string `json:"profile_url"`
}
