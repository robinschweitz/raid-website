package addonsync

import (
    "strings"
    "testing"
    "time"
)

func TestBuildCalendarICS(t *testing.T) {
    start := time.Date(2026, 11, 4, 19, 0, 0, 0, time.UTC)
    end := start.Add(3 * time.Hour)
    text := BuildCalendarICS(42, "ICC, 25", "Bring flasks; food\nReady", start, end, start)
    for _, part := range []string{"BEGIN:VCALENDAR\r\n", "UID:raid-42@raid-website\r\n", "DTSTART:20261104T190000Z", "DTEND:20261104T220000Z", "SUMMARY:ICC\\, 25", "DESCRIPTION:Bring flasks\\; food\\nReady", "END:VCALENDAR\r\n"} {
        if !strings.Contains(text, part) { t.Fatalf("missing %q in %s", part, text) }
    }
}
func TestScheduledInviteExport(t *testing.T) {
    start := time.Unix(1793818800, 0)
    end := start.Add(3 * time.Hour)
    got := BuildScheduledInviteExport(42, "ICC", nil, &start, &end, nil)
    if !strings.HasPrefix(got, "RS2|INV|42|ICC||1793818800|1793829600") { t.Fatal(got) }
    old := BuildInviteExport(42, "ICC", nil, nil)
    if old != "RS1|INV|42|ICC|" { t.Fatal(old) }
}
