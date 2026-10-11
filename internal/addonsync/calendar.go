package addonsync

import (
    "fmt"
    "strings"
    "time"
)

// BuildCalendarICS exports a raid event in RFC 5545-compatible UTC form.
// Calendar clients identify revisions by stable UID.
func BuildCalendarICS(id int64, name, notes string, start, end time.Time, updated time.Time) string {
    escape := func(s string) string {
        s = strings.ReplaceAll(s, "\\", "\\\\")
        s = strings.ReplaceAll(s, "\n", "\\n")
        s = strings.ReplaceAll(s, "\r", "")
        s = strings.ReplaceAll(s, ",", "\\,")
        s = strings.ReplaceAll(s, ";", "\\;")
        return s
    }
    stamp := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
    lines := []string{
        "BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Raid Website//Raid Calendar//EN",
        "CALSCALE:GREGORIAN", "METHOD:PUBLISH", "BEGIN:VEVENT",
        fmt.Sprintf("UID:raid-%d@raid-website", id),
        "DTSTAMP:" + stamp(updated), "LAST-MODIFIED:" + stamp(updated),
        "DTSTART:" + stamp(start), "DTEND:" + stamp(end),
        "SUMMARY:" + escape(name), "DESCRIPTION:" + escape(notes),
        "END:VEVENT", "END:VCALENDAR",
    }
    // Fold content lines by octets, never breaking UTF-8 rune boundaries.
    var out strings.Builder
    for _, line := range lines {
        count := 0
        for _, r := range line {
            n := len(string(r))
            if count+n > 75 {
                out.WriteString("\r\n ")
                count = 1
            }
            out.WriteRune(r)
            count += n
        }
        out.WriteString("\r\n")
    }
    return out.String()
}
