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

// lootFormatV2 is the loot export of ForgeLootLedger: ";"-separated (a WoW
// EditBox treats "|" as an escape character) and with roll details.
const lootFormatV2 = "RS2"

// maxEnhancedPoints is the most EP an enhanced roll can add (ADR 023).
const maxEnhancedPoints = 50

// ErrInvalidPayload marks pasted text that is not the expected RaidSync export.
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
	Version   string
	RaidID    int64
	Recorder  string
	StartedAt time.Time
	Lines     []LootLine
}

// LootState says how the addon learned a drop's recipient.
type LootState string

// Loot states written by the addon.
const (
	// LootGiven is a master-loot assignment, a loot message or a completed trade.
	LootGiven LootState = "given"
	// LootHeld is an item the recorder looted to himself and has not traded on yet.
	LootHeld LootState = "held"
	// LootManual is a recipient the recorder set by hand in the addon.
	LootManual LootState = "manual"
	// LootParked (RS2) is an item another raider keeps until it is rolled,
	// e.g. a unique item the recorder cannot carry twice.
	LootParked LootState = "parked"
	// LootDeleted (RS2) is an award the recorder removed in the addon.
	LootDeleted LootState = "deleted"
	// LootDisenchanted (RS2) is an item nobody wanted, given to the recipient
	// to disenchant; it is nobody's award.
	LootDisenchanted LootState = "disenchanted"
	// LootBank (RS2) is an item nobody wanted, kept by the recipient for the
	// guild bank; it is nobody's award.
	LootBank LootState = "bank"
)

// Roll specs written by RS2 exports.
const (
	SpecMain = "ms"
	SpecOff  = "os"
)

// LootLine is one recorded drop. Error is set when the line could not be read;
// such lines are reported, never silently dropped.
type LootLine struct {
	LineNumber    int
	Raw           string
	EntryID       string
	LootedAt      time.Time
	NPCID         string
	ItemID        string
	RecipientName string
	State         LootState
	// RS2 only: the winning roll, its spec and range and the roll winner. For
	// a held line the winner is who the recorder still has to trade it to.
	Roll       int
	Spec       string
	RollLow    int
	RollHigh   int
	WinnerName string
	Error      string
}

// EnhancedPoints is X of an enhanced main-spec roll 1+X..100+X (X 1-50,
// ADR 023), or 0 for an ordinary roll.
func (l LootLine) EnhancedPoints() int {
	x := l.RollLow - 1
	if l.RollHigh != l.RollLow+99 || x < 1 || x > maxEnhancedPoints {
		return 0
	}
	return x
}

// ParseLootExport reads the RaidSync format
//
//	RS1|LOOT|<raidID>|<recorder>|<startUnix>
//	<entryID>|<unixTime>|<npcID or ->|<itemID>|<recipient or ->|<state or ->
//
// and the ForgeLootLedger format
//
//	RS2;LOOT;<raidID>;<recorder>;<startUnix>;<exportUnix>
//	<entryID>;<unixTime>;<npcID or ->;<itemID>;<recipient>;<state>;<roll or ->;<ms|os|->;<low-high or ->;<winner or ->
//
// A broken header rejects the whole text; a broken drop line is kept with Error set.
func ParseLootExport(text string) (*LootExport, error) {
	header, body, err := parseHeader(text, "LOOT")
	if err != nil {
		return nil, err
	}
	export := &LootExport{Version: header.version, RaidID: header.raidID, Recorder: header.recorder, StartedAt: header.at}
	parse := parseLootLine
	if header.version == lootFormatV2 {
		parse = parseLootLineV2
	}
	entryIDs := map[string]bool{}
	for _, row := range body {
		line := parse(row.number, row.text)
		if line.Error == "" {
			if entryIDs[line.EntryID] {
				line.Error = "Eintrags-ID kommt doppelt vor"
			}
			entryIDs[line.EntryID] = true
		}
		export.Lines = append(export.Lines, line)
	}
	return export, nil
}

func parseLootLine(lineNumber int, raw string) LootLine {
	line := LootLine{LineNumber: lineNumber, Raw: raw}
	parts := strings.Split(raw, "|")
	if len(parts) != 6 {
		line.Error = "Zeile hat nicht sechs Felder (Addon aktualisieren?)"
		return line
	}
	if id, err := strconv.ParseInt(parts[0], 10, 64); err != nil || id <= 0 {
		line.Error = "Eintrags-ID ist keine positive Zahl"
		return line
	}
	line.EntryID = parts[0]
	lootedUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		line.Error = "Zeitpunkt ist keine Zahl"
		return line
	}
	line.LootedAt = time.Unix(lootedUnix, 0).UTC()
	if parts[2] != "-" {
		if _, err := strconv.Atoi(parts[2]); err != nil {
			line.Error = "NPC-ID ist keine Zahl"
			return line
		}
		line.NPCID = parts[2]
	}
	if _, err := strconv.Atoi(parts[3]); err != nil || parts[3] == "" {
		line.Error = "Item-ID ist keine Zahl"
		return line
	}
	line.ItemID = parts[3]
	if parts[4] != "-" {
		line.RecipientName = strings.TrimSpace(parts[4])
	}
	switch state := LootState(parts[5]); {
	case parts[5] == "-" && line.RecipientName == "":
	case line.RecipientName != "" && (state == LootGiven || state == LootHeld || state == LootManual):
		line.State = state
	default:
		line.Error = "Status passt nicht zum Empfänger"
	}
	return line
}

func parseLootLineV2(lineNumber int, raw string) LootLine {
	line := LootLine{LineNumber: lineNumber, Raw: raw}
	parts := strings.Split(raw, ";")
	if len(parts) != 10 {
		line.Error = "Zeile hat nicht zehn Felder (Addon aktualisieren?)"
		return line
	}
	common := parseLootLine(lineNumber, strings.Join([]string{parts[0], parts[1], parts[2], parts[3], "-", "-"}, "|"))
	if common.Error != "" {
		line.Error = common.Error
		return line
	}
	line.EntryID, line.LootedAt, line.NPCID, line.ItemID = common.EntryID, common.LootedAt, common.NPCID, common.ItemID
	if parts[4] != "-" {
		line.RecipientName = strings.TrimSpace(parts[4])
	}
	switch state := LootState(parts[5]); state {
	case LootGiven, LootHeld, LootManual, LootParked, LootDeleted, LootDisenchanted, LootBank:
		line.State = state
	default:
		line.Error = "Unbekannter Status"
		return line
	}
	if line.RecipientName == "" && line.State != LootDeleted {
		line.Error = "Empfänger fehlt"
		return line
	}
	if parts[6] != "-" {
		roll, err := strconv.Atoi(parts[6])
		if err != nil || roll < 1 || roll > 100+maxEnhancedPoints {
			line.Error = "Wurf ist keine Zahl von 1 bis 150"
			return line
		}
		line.Roll = roll
	}
	switch parts[7] {
	case "-":
	case SpecMain, SpecOff:
		line.Spec = parts[7]
	default:
		line.Error = "Spezialisierung muss ms, os oder - sein"
		return line
	}
	if parts[8] != "-" {
		low, high, ok := parseRange(parts[8])
		if !ok {
			line.Error = "Wurfbereich muss <von>-<bis> sein"
			return line
		}
		line.RollLow, line.RollHigh = low, high
	}
	if parts[9] != "-" {
		line.WinnerName = strings.TrimSpace(parts[9])
	}
	return line
}

func parseRange(value string) (int, int, bool) {
	lowText, highText, found := strings.Cut(value, "-")
	low, lowErr := strconv.Atoi(lowText)
	high, highErr := strconv.Atoi(highText)
	if !found || lowErr != nil || highErr != nil || low < 1 || high <= low || high > 100+maxEnhancedPoints {
		return 0, 0, false
	}
	return low, high, true
}

// lineKeys gives every line a stable identity, used as the loot request ID so
// importing the same export again never creates a second award. The identity
// is the addon's entry ID within one recording, not the line text, so a drop
// whose recipient changed after a trade is still recognised. Unreadable lines
// fall back to their text, numbered by occurrence.
func lineKeys(raidID int64, export *LootExport) []string {
	seen := map[string]int{}
	keys := make([]string, len(export.Lines))
	for i, line := range export.Lines {
		identity := fmt.Sprintf("%d|%s|%d|entry:%s", raidID, export.Recorder, export.StartedAt.Unix(), line.EntryID)
		if line.Error != "" {
			identity = fmt.Sprintf("%d|%s|raw:%s", raidID, export.Recorder, line.Raw)
			seen[identity]++
			identity = fmt.Sprintf("%s|%d", identity, seen[identity])
		}
		sum := sha256.Sum256([]byte(identity))
		keys[i] = fmt.Sprintf("raidsync-%x", sum[:12])
	}
	return keys
}

// RosterExport is a parsed "who was in the raid" export from the addon.
type RosterExport struct {
	RaidID     int64
	Recorder   string
	ExportedAt time.Time
	Members    []RosterMember
}

// RosterMember is one character the addon saw in the raid.
type RosterMember struct {
	LineNumber int
	Raw        string
	Name       string
	ClassToken string
	FirstSeen  time.Time
	Error      string
}

// ParseRosterExport reads:
//
//	RS1|ROSTER|<raidID>|<recorder>|<exportUnix>
//	<name>|<classToken or ->|<firstSeenUnix>
//
// A broken header rejects the whole text; a broken member line is kept with Error set.
func ParseRosterExport(text string) (*RosterExport, error) {
	header, body, err := parseHeader(text, "ROSTER")
	if err != nil {
		return nil, err
	}
	export := &RosterExport{RaidID: header.raidID, Recorder: header.recorder, ExportedAt: header.at}
	for _, row := range body {
		member := RosterMember{LineNumber: row.number, Raw: row.text}
		parts := strings.Split(row.text, "|")
		switch {
		case len(parts) != 3:
			member.Error = "Zeile hat nicht drei Felder"
		case strings.TrimSpace(parts[0]) == "":
			member.Error = "Name fehlt"
		default:
			seen, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				member.Error = "Zeitpunkt ist keine Zahl"
				break
			}
			member.Name = strings.TrimSpace(parts[0])
			if parts[1] != "-" {
				member.ClassToken = parts[1]
			}
			member.FirstSeen = time.Unix(seen, 0).UTC()
		}
		export.Members = append(export.Members, member)
	}
	return export, nil
}

type exportHeader struct {
	version  string
	raidID   int64
	recorder string
	at       time.Time
}

type bodyLine struct {
	number int
	text   string
}

// parseHeader checks "RS1|<kind>|<raidID>|<recorder>|<unix>" (or, for loot,
// "RS2;LOOT;<raidID>;<recorder>;<startUnix>;<exportUnix>") on the first
// non-empty line and returns the remaining non-empty lines with their numbers.
func parseHeader(text, kind string) (exportHeader, []bodyLine, error) {
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
		return exportHeader{}, nil, fmt.Errorf("empty text: %w", ErrInvalidPayload)
	}
	header := strings.Split(lines[headerIndex], "|")
	version := formatVersion
	if kind == "LOOT" && strings.HasPrefix(lines[headerIndex], lootFormatV2+";") {
		header = strings.Split(lines[headerIndex], ";")
		version = lootFormatV2
		if len(header) == 6 {
			if _, err := strconv.ParseInt(header[5], 10, 64); err != nil {
				return exportHeader{}, nil, fmt.Errorf("export time %q: %w", header[5], ErrInvalidPayload)
			}
			header = header[:5]
		} else {
			header = nil
		}
	}
	if len(header) != 5 || header[0] != version || header[1] != kind {
		return exportHeader{}, nil, fmt.Errorf("header %q: %w", lines[headerIndex], ErrInvalidPayload)
	}
	raidID, err := strconv.ParseInt(header[2], 10, 64)
	if err != nil {
		return exportHeader{}, nil, fmt.Errorf("raid id %q: %w", header[2], ErrInvalidPayload)
	}
	unix, err := strconv.ParseInt(header[4], 10, 64)
	if err != nil {
		return exportHeader{}, nil, fmt.Errorf("time %q: %w", header[4], ErrInvalidPayload)
	}
	var body []bodyLine
	for i := headerIndex + 1; i < len(lines); i++ {
		if lines[i] != "" {
			body = append(body, bodyLine{number: i + 1, text: lines[i]})
		}
	}
	return exportHeader{version: version, raidID: raidID, recorder: header[3], at: time.Unix(unix, 0).UTC()}, body, nil
}
