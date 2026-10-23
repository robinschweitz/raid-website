package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/uptrace/bun"
)

type statsFixture struct {
	t *testing.T
	b *bun.DB
}

func (f statsFixture) exec(query string, args ...any) int64 {
	f.t.Helper()
	res, err := f.b.ExecContext(context.Background(), query, args...)
	if err != nil {
		f.t.Fatalf("seed %q: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f statsFixture) player(name string, active bool) int64 {
	return f.exec(`INSERT INTO players (name, role, active, created_at, updated_at) VALUES (?, 'MEMBER', ?, datetime('now'), datetime('now'))`, name, active)
}

func (f statsFixture) character(playerID int64, name, class string) int64 {
	return f.exec(`INSERT INTO characters (player_id, name, class, main_spec, second_spec, created_at, updated_at) VALUES (?, ?, ?, 'x', '', datetime('now'), datetime('now'))`, playerID, name, class)
}

func (f statsFixture) raid(leader int64, status string, archived bool) int64 {
	archivedAt := "NULL"
	if archived {
		archivedAt = "datetime('now')"
	}
	return f.exec(fmt.Sprintf(`INSERT INTO raids (name, raid_leader_id, status, archived_at, created_at, updated_at) VALUES ('Raid', ?, ?, %s, datetime('now'), datetime('now'))`, archivedAt), leader, status)
}

func (f statsFixture) attend(raidID, playerID, charID int64, role string) {
	f.exec(`INSERT INTO raid_attendance (raid_id, player_id, character_id, role, notes, created_at, updated_at) VALUES (?, ?, ?, ?, 'GEHEIMNOTIZ', datetime('now'), datetime('now'))`, raidID, playerID, charID, role)
}

func (f statsFixture) loot(raidID, playerID, charID int64, withdrawn bool) {
	withdrawnAt := "NULL"
	if withdrawn {
		withdrawnAt = "datetime('now')"
	}
	f.exec(fmt.Sprintf(`INSERT INTO loot_awards (raid_id, boss, item_id, item_name, awarded_to_player_id, awarded_to_character_id, awarded_by_player_id, source, notes, awarded_at, withdrawn_at, created_at, updated_at)
		VALUES (?, 'Boss', '49623', 'GEHEIMITEM', ?, ?, ?, 'manual', 'GEHEIMNOTIZ', datetime('now'), %s, datetime('now'), datetime('now'))`, withdrawnAt), raidID, playerID, charID, playerID)
}

func getStatistics(t *testing.T, srv string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(srv + "/api/v1/statistics")
	if err != nil {
		t.Fatalf("GET statistics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read statistics: %v", err)
	}
	return resp.StatusCode, body
}

func decodeStatistics(t *testing.T, body []byte) map[string][]map[string]any {
	t.Helper()
	var out struct {
		Data map[string][]map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v body %s", err, body)
	}
	return out.Data
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestReqStats010_AC01 checks the public contract: no session, exactly the
// documented fields, every status listed, empty lists as [].
func TestReqStats010_AC01(t *testing.T) {
	srv, _ := setupServer(t)
	status, body := getStatistics(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("want 200 got %d body %s", status, body)
	}
	var raw struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	var fields []string
	for k := range raw.Data {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	want := []string{"class_distribution", "loot_by_class", "raids_by_status", "role_history", "top_attendance", "top_loot"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
	for _, k := range []string{"class_distribution", "loot_by_class", "role_history", "top_attendance", "top_loot"} {
		if string(raw.Data[k]) != "[]" {
			t.Fatalf("%s = %s, want []", k, raw.Data[k])
		}
	}
	var order []string
	for _, row := range decodeStatistics(t, body)["raids_by_status"] {
		if row["count"].(float64) != 0 {
			t.Fatalf("empty database has count %v", row)
		}
		order = append(order, row["status"].(string))
	}
	if !reflect.DeepEqual(order, []string{"PLANNED", "OPEN", "COMPLETED", "CANCELLED"}) {
		t.Fatalf("status order = %v", order)
	}
}

// TestReqStats010_AC02 checks FR-27.4 ordering: count descending, then name in
// UTF-8 byte order, then ID, and the cut of the top lists at 10.
func TestReqStats010_AC02(t *testing.T) {
	srv, bdb := setupServer(t)
	f := statsFixture{t, bdb}
	leader := f.player("Leiter", true)
	raidID := f.raid(leader, "COMPLETED", true)
	// "Ärger" sorts after "Zed" in byte order (0xC3 > 0x5A), and the two
	// "Gleich" players tie on name, so the ID decides.
	names := []string{"Zed", "Ärger", "Anna", "Gleich", "Gleich"}
	var ids []int64
	for _, name := range names {
		pid := f.player(name, true)
		ids = append(ids, pid)
		cid := f.character(pid, name+"-c", "MAGE")
		f.attend(raidID, pid, cid, "DD")
	}
	// A bigger count comes first regardless of name.
	top := f.player("Zzz", true)
	topChar := f.character(top, "Zzz-c", "MAGE")
	raidTwo := f.raid(leader, "COMPLETED", true)
	f.attend(raidTwo, top, topChar, "DD")
	f.attend(raidID, top, topChar, "DD")

	status, body := getStatistics(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("status %d body %s", status, body)
	}
	var got []string
	for _, row := range decodeStatistics(t, body)["top_attendance"] {
		got = append(got, fmt.Sprintf("%s#%d", row["player_name"], int64(row["player_id"].(float64))))
	}
	want := []string{
		fmt.Sprintf("Zzz#%d", top),
		fmt.Sprintf("Anna#%d", ids[2]),
		fmt.Sprintf("Gleich#%d", ids[3]),
		fmt.Sprintf("Gleich#%d", ids[4]),
		fmt.Sprintf("Zed#%d", ids[0]),
		fmt.Sprintf("Ärger#%d", ids[1]),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	for i := 0; i < 12; i++ {
		pid := f.player(fmt.Sprintf("Extra%02d", i), true)
		cid := f.character(pid, fmt.Sprintf("Extra%02d-c", i), "MAGE")
		f.attend(raidID, pid, cid, "DD")
	}
	_, body = getStatistics(t, srv.URL)
	if n := len(decodeStatistics(t, body)["top_attendance"]); n != 10 {
		t.Fatalf("top_attendance has %d entries, want 10", n)
	}
}

// TestReqStats010_AC03 checks that no item, EP, note, or login data appears and
// that only counted data is returned.
func TestReqStats010_AC03(t *testing.T) {
	srv, bdb := setupServer(t)
	f := statsFixture{t, bdb}
	leader := f.player("Leiter", true)
	raidID := f.raid(leader, "COMPLETED", true)
	f.raid(leader, "PLANNED", false)
	f.raid(leader, "COMPLETED", false) // completed, not archived: still history
	pid := f.player("Anna", true)
	cid := f.character(pid, "Annchar", "MAGE")
	f.attend(raidID, pid, cid, "HEAL")
	f.loot(raidID, pid, cid, false)
	f.loot(raidID, pid, cid, true) // withdrawn: not counted

	status, body := getStatistics(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	text := string(body)
	for _, forbidden := range []string{"GEHEIMITEM", "GEHEIMNOTIZ", "49623", "item", "ep_", "balance", "note", "username", "login", "password", "MEMBER"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("response contains %q: %s", forbidden, text)
		}
	}
	data := decodeStatistics(t, body)
	allowed := map[string][]string{
		"raids_by_status":    {"count", "status"},
		"class_distribution": {"class", "count"},
		"role_history":       {"count", "role"},
		"loot_by_class":      {"class", "count"},
		"top_attendance":     {"count", "player_id", "player_name"},
		"top_loot":           {"count", "player_id", "player_name"},
	}
	for list, keys := range allowed {
		for _, row := range data[list] {
			if !reflect.DeepEqual(keysOf(row), keys) {
				t.Fatalf("%s row keys = %v, want %v", list, keysOf(row), keys)
			}
		}
	}
	counts := map[string]float64{}
	for _, row := range data["raids_by_status"] {
		counts[row["status"].(string)] = row["count"].(float64)
	}
	if counts["COMPLETED"] != 2 || counts["PLANNED"] != 1 || counts["OPEN"] != 0 || counts["CANCELLED"] != 0 {
		t.Fatalf("raid counts = %v", counts)
	}
	if got := data["loot_by_class"]; len(got) != 1 || got[0]["class"] != "MAGE" || got[0]["count"].(float64) != 1 {
		t.Fatalf("loot_by_class = %v", got)
	}
	if got := data["role_history"]; len(got) != 1 || got[0]["role"] != "HEAL" {
		t.Fatalf("role_history = %v", got)
	}
}

// TestReqStats010_AC04 checks that inactive and anonymised players appear
// under their replacement name with the same ID.
func TestReqStats010_AC04(t *testing.T) {
	srv, bdb := setupServer(t)
	f := statsFixture{t, bdb}
	leader := f.player("Leiter", true)
	raidID := f.raid(leader, "COMPLETED", true)
	anon := f.player("Ehemaliges Mitglied #7", false)
	cid := f.character(anon, "Altchar", "ROGUE")
	f.attend(raidID, anon, cid, "DD")
	f.loot(raidID, anon, cid, false)

	status, body := getStatistics(t, srv.URL)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	for _, list := range []string{"top_attendance", "top_loot"} {
		rows := decodeStatistics(t, body)[list]
		if len(rows) != 1 || rows[0]["player_name"] != "Ehemaliges Mitglied #7" || int64(rows[0]["player_id"].(float64)) != anon {
			t.Fatalf("%s = %v", list, rows)
		}
	}
}
