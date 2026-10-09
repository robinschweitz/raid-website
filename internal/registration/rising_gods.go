package registration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/validation"
)

const risingGodsBaseURL = "https://db.rising-gods.de"

var (
	ErrExternalNotFound = errors.New("rising-gods character not found")
	ErrExternalInvalid  = errors.New("rising-gods profile invalid")

	listviewProfileID = regexp.MustCompile(`"id"\s*:\s*(\d+)`)
	listviewName      = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	listviewClass     = regexp.MustCompile(`"classs"\s*:\s*(\d+)`)
	listviewLevel     = regexp.MustCompile(`"level"\s*:\s*(\d+)`)
	listviewTree1     = regexp.MustCompile(`"talenttree1"\s*:\s*(\d+)`)
	listviewTree2     = regexp.MustCompile(`"talenttree2"\s*:\s*(\d+)`)
	listviewTree3     = regexp.MustCompile(`"talenttree3"\s*:\s*(\d+)`)
	profileIDField    = regexp.MustCompile(`"id"\s*:\s*(\d+)`)
	profileLevel      = regexp.MustCompile(`"level"\s*:\s*(\d+)`)
	profileClass      = regexp.MustCompile(`"classs"\s*:\s*(\d+)`)
	profileRace       = regexp.MustCompile(`"race"\s*:\s*(\d+)`)
	profileFaction    = regexp.MustCompile(`"faction"\s*:\s*(\d+)`)
	profileName       = regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`)
	profileTalent     = regexp.MustCompile(`"talents"\s*:\s*"([0-9]+)"`)
)

type ExternalClient interface {
	SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error)
	LoadCharacter(ctx context.Context, profileID string) (ExternalCharacter, string, error)
}

type RisingGodsClient struct {
	HTTPClient *http.Client
}

func (c RisingGodsClient) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c RisingGodsClient) SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error) {
	q := strings.TrimSpace(query)
	if len([]rune(q)) < 3 {
		return nil, ErrExternalInvalid
	}
	u := risingGodsBaseURL + "/?profiles=eu.rising-gods&filter=" + url.QueryEscape("na="+q)
	body, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	dataStart := strings.Index(body, `"data":[`)
	if dataStart < 0 {
		return []ExternalCharacter{}, nil
	}
	dataEnd := strings.Index(body[dataStart:], `,"parent"`)
	if dataEnd < 0 {
		return []ExternalCharacter{}, nil
	}
	data := body[dataStart : dataStart+dataEnd]
	objects := splitJSObjects(data)
	results := make([]ExternalCharacter, 0, len(objects))
	for _, raw := range objects {
		id := firstSubmatch(listviewProfileID, raw)
		name := firstSubmatch(listviewName, raw)
		classID, _ := strconv.Atoi(firstSubmatch(listviewClass, raw))
		level, _ := strconv.Atoi(firstSubmatch(listviewLevel, raw))
		tree1, _ := strconv.Atoi(firstSubmatch(listviewTree1, raw))
		tree2, _ := strconv.Atoi(firstSubmatch(listviewTree2, raw))
		tree3, _ := strconv.Atoi(firstSubmatch(listviewTree3, raw))
		class := className(classID)
		if id == "" || name == "" || class == "" {
			continue
		}
		results = append(results, ExternalCharacter{ProfileID: id, Name: name, Class: class, MainSpec: specFromTreeTotals(class, [3]int{tree1, tree2, tree3}), Level: level, ProfileURL: profileURL(id)})
	}
	return results, nil
}

func (c RisingGodsClient) LoadCharacter(ctx context.Context, profileID string) (ExternalCharacter, string, error) {
	id := strings.TrimSpace(profileID)
	if id == "" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	body, err := c.fetch(ctx, risingGodsBaseURL+"/?profile=load&id="+url.QueryEscape(id))
	if err != nil {
		return ExternalCharacter{}, "", err
	}
	raw := extractRegisterProfileObject(body)
	if raw == "" {
		return ExternalCharacter{}, "", ErrExternalNotFound
	}
	loadedID := firstSubmatch(profileIDField, raw)
	level, _ := strconv.Atoi(firstSubmatch(profileLevel, raw))
	classID, _ := strconv.Atoi(firstSubmatch(profileClass, raw))
	raceID, _ := strconv.Atoi(firstSubmatch(profileRace, raw))
	factionID, _ := strconv.Atoi(firstSubmatch(profileFaction, raw))
	name := firstSubmatch(profileName, raw)
	class := className(classID)
	if loadedID != id || name == "" || class == "" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	talentMatches := profileTalent.FindAllStringSubmatch(raw, 2)
	mainSpec := ""
	secondSpec := ""
	if len(talentMatches) > 0 {
		mainSpec = deriveSpec(class, talentMatches[0][1])
	}
	if len(talentMatches) > 1 {
		secondSpec = deriveSpec(class, talentMatches[1][1])
	}
	if mainSpec == "" {
		rows, err := c.SearchCharacters(ctx, name)
		if err == nil {
			for _, row := range rows {
				if row.ProfileID == id && row.MainSpec != "" {
					mainSpec = row.MainSpec
					break
				}
			}
		}
	}
	if mainSpec == "" {
		return ExternalCharacter{}, "", ErrExternalInvalid
	}
	profile := ExternalCharacter{ProfileID: id, Name: name, Class: class, MainSpec: mainSpec, SecondSpec: secondSpec, Level: level, Race: raceName(raceID), Faction: factionName(factionID), ProfileURL: profileURL(id)}
	return profile, raw, nil
}

func (c RisingGodsClient) fetch(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrExternalNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("rising-gods status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func splitJSObjects(data string) []string {
	start := strings.IndexByte(data, '[')
	if start < 0 {
		return nil
	}
	var out []string
	depth := 0
	objectStart := -1
	inString := false
	escaped := false
	for i := start + 1; i < len(data); i++ {
		ch := data[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			if depth == 0 {
				objectStart = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && objectStart >= 0 {
				out = append(out, data[objectStart:i+1])
				objectStart = -1
			}
		}
	}
	return out
}

func extractRegisterProfileObject(body string) string {
	marker := "$WowheadProfiler.registerProfile("
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	if start >= len(body) || body[start] != '{' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(body); i++ {
		ch := body[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return body[start : i+1]
			}
		}
	}
	return ""
}

func firstSubmatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func className(id int) string {
	switch id {
	case 1:
		return "Krieger"
	case 2:
		return "Paladin"
	case 3:
		return "Jäger"
	case 4:
		return "Schurke"
	case 5:
		return "Priester"
	case 6:
		return "Todesritter"
	case 7:
		return "Schamane"
	case 8:
		return "Magier"
	case 9:
		return "Hexer"
	case 11:
		return "Druide"
	default:
		return ""
	}
}

func raceName(id int) string {
	switch id {
	case 1:
		return "Mensch"
	case 2:
		return "Orc"
	case 3:
		return "Zwerg"
	case 4:
		return "Nachtelf"
	case 5:
		return "Untoter"
	case 6:
		return "Taure"
	case 7:
		return "Gnom"
	case 8:
		return "Troll"
	case 10:
		return "Blutelf"
	case 11:
		return "Draenei"
	default:
		return ""
	}
}

func factionName(id int) string {
	if id == 1 {
		return "Alliance"
	}
	if id == 2 {
		return "Horde"
	}
	return ""
}

func profileURL(id string) string {
	return risingGodsBaseURL + "/?profile=" + url.QueryEscape(id)
}

func deriveSpec(class, talents string) string {
	if talents == "" || len(talents)%3 != 0 {
		return ""
	}
	section := len(talents) / 3
	sums := [3]int{}
	for tree := 0; tree < 3; tree++ {
		for _, ch := range talents[tree*section : (tree+1)*section] {
			if ch >= '0' && ch <= '9' {
				sums[tree] += int(ch - '0')
			}
		}
	}
	best := 0
	for i := 1; i < 3; i++ {
		if sums[i] > sums[best] {
			best = i
		}
	}
	if sums[best] == 0 {
		return ""
	}
	specs := validation.ClassSpecs[class]
	if len(specs) < 3 {
		return ""
	}
	if class == "Todesritter" && best == 0 {
		return "Blut (DD)"
	}
	if class == "Druide" && best == 1 {
		return "Wilder Kampf (Cat)"
	}
	return specs[best]
}

func specFromTreeTotals(class string, sums [3]int) string {
	best := 0
	for i := 1; i < 3; i++ {
		if sums[i] > sums[best] {
			best = i
		}
	}
	if sums[best] == 0 {
		return ""
	}
	specs := validation.ClassSpecs[class]
	if len(specs) < 3 {
		return ""
	}
	if class == "Todesritter" && best == 0 {
		return "Blut (DD)"
	}
	if class == "Druide" && best == 1 {
		return "Wilder Kampf (Cat)"
	}
	return specs[best]
}
