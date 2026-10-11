// Package addonsync converts raid data to and from the line-based text format
// that the RaidSync WoW addon exchanges by copy and paste. WoW addons have no
// network access, so this text is the only channel between game and website.
package addonsync

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/validation"
)

// formatVersion prefixes every export so a future format can be told apart.
const formatVersion = "RS1"

// ErrInvalidPayload marks pasted text that is not a RaidSync loot export.
var ErrInvalidPayload = errors.New("invalid addon export")

// InviteMember is one roster entry the addon should invite.
type InviteMember struct {
	Name    string
	Role    validation.AttendanceRole
	Class   string
	IsBench bool
}

// BuildInviteExport renders the invite list:
//
//	RS1|INV|<raidID>|<raidName>|<raidSize>
//	<name>|<role>|<class>|<bench 0/1>
func BuildInviteExport(raidID int64, raidName string, raidSize *int, members []InviteMember) string {
	size := ""
	if raidSize != nil {
		size = strconv.Itoa(*raidSize)
	}
	lines := []string{strings.Join([]string{formatVersion, "INV", strconv.FormatInt(raidID, 10), field(raidName), size}, "|")}
	for _, member := range members {
		bench := "0"
		if member.IsBench {
			bench = "1"
		}
		lines = append(lines, strings.Join([]string{field(member.Name), string(member.Role), field(member.Class), bench}, "|"))
	}
	return strings.Join(lines, "\n")
}

// field keeps free text from breaking the line and column separators.
func field(value string) string {
	value = strings.ReplaceAll(value, "|", "/")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

// LootExport is a parsed loot export from the addon.
type LootExport struct {
	RaidID    int64
	Recorder  string
	StartedAt time.Time
	Lines     []LootLine
}

// LootLine is one recorded drop. Error is set when the line could not be read;
// such lines are reported, never silently dropped.
type LootLine struct {
	LineNumber    int
	Raw           string
	LootedAt      time.Time
	NPCID         string
	ItemID        string
	RecipientName string
	Error         string
}

// ParseLootExport reads:
//
//	RS1|LOOT|<raidID>|<recorder>|<startUnix>
//	<unixTime>|<npcID or ->|<itemID>|<recipient or ->
//
// A broken header rejects the whole text; a broken drop line is kept with Error set.
func ParseLootExport(text string) (*LootExport, error) {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	headerIndex := -1
	for i, line := range lines {
		if line != "" {
			headerIndex = i
			break
		}
	}
	if headerIndex < 0 {
		return nil, fmt.Errorf("empty text: %w", ErrInvalidPayload)
	}
	header := strings.Split(lines[headerIndex], "|")
	if len(header) != 5 || header[0] != formatVersion || header[1] != "LOOT" {
		return nil, fmt.Errorf("header %q: %w", lines[headerIndex], ErrInvalidPayload)
	}
	raidID, err := strconv.ParseInt(header[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("raid id %q: %w", header[2], ErrInvalidPayload)
	}
	startUnix, err := strconv.ParseInt(header[4], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("start time %q: %w", header[4], ErrInvalidPayload)
	}
	export := &LootExport{RaidID: raidID, Recorder: header[3], StartedAt: time.Unix(startUnix, 0).UTC()}
	for i := headerIndex + 1; i < len(lines); i++ {
		if lines[i] == "" {
			continue
		}
		export.Lines = append(export.Lines, parseLootLine(i+1, lines[i]))
	}
	return export, nil
}

func parseLootLine(lineNumber int, raw string) LootLine {
	line := LootLine{LineNumber: lineNumber, Raw: raw}
	parts := strings.Split(raw, "|")
	if len(parts) != 4 {
		line.Error = "Zeile hat nicht vier Felder"
		return line
	}
	lootedUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		line.Error = "Zeitpunkt ist keine Zahl"
		return line
	}
	line.LootedAt = time.Unix(lootedUnix, 0).UTC()
	if parts[1] != "-" {
		if _, err := strconv.Atoi(parts[1]); err != nil {
			line.Error = "NPC-ID ist keine Zahl"
			return line
		}
		line.NPCID = parts[1]
	}
	if _, err := strconv.Atoi(parts[2]); err != nil || parts[2] == "" {
		line.Error = "Item-ID ist keine Zahl"
		return line
	}
	line.ItemID = parts[2]
	if parts[3] != "-" {
		line.RecipientName = strings.TrimSpace(parts[3])
	}
	return line
}

// lineKeys gives every line a stable identity, used as the loot request ID so
// importing the same export again never creates a second award. Identical
// lines are numbered by occurrence so two equal drops stay two awards.
func lineKeys(raidID int64, export *LootExport) []string {
	seen := map[string]int{}
	keys := make([]string, len(export.Lines))
	for i, line := range export.Lines {
		identity := fmt.Sprintf("%d|%s|%s", raidID, export.Recorder, line.Raw)
		seen[identity]++
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", identity, seen[identity])))
		keys[i] = fmt.Sprintf("raidsync-%x", sum[:12])
	}
	return keys
}
