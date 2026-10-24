package ciconfig

import (
	"strings"
	"testing"
)

// TestReqPlatform017_AC01: a daily timer runs the encrypted off-server copy and a
// monthly timer runs the restore test, and deploy/README.md installs both
// (NFR-2.3, NFR-2.5, SC-034).
func TestReqPlatform017_AC01(t *testing.T) {
	for _, unit := range []struct{ name, flag, calendar string }{
		{"raid-website-backup-offsite", "--offsite", "OnCalendar=*-*-* "},
		{"raid-website-backup-restore-test", "--restore-test", "OnCalendar=*-*-01 "},
	} {
		service := readRepoFile(t, "deploy/systemd/"+unit.name+".service")
		if !strings.Contains(service, "ExecStart=/usr/local/sbin/raid-website-backup "+unit.flag+"\n") {
			t.Errorf("%s.service does not run raid-website-backup %s", unit.name, unit.flag)
		}
		timer := readRepoFile(t, "deploy/systemd/"+unit.name+".timer")
		if !strings.Contains(timer, unit.calendar) || !strings.Contains(timer, "Persistent=true") {
			t.Errorf("%s.timer does not run on %q with Persistent=true", unit.name, unit.calendar)
		}
		readme := readRepoFile(t, "deploy/README.md")
		for _, suffix := range []string{".service", ".timer"} {
			install := "deploy/systemd/" + unit.name + suffix + " /etc/systemd/system/" + unit.name + suffix
			if !strings.Contains(readme, install) {
				t.Errorf("deploy/README.md does not install %s", install)
			}
		}
	}
}

// TestReqPlatform017_AC03: the copy is encrypted before upload, every sftp
// batch fails on the first error, an upload is renamed into place and compared
// against a download, and the committed settings hold no credentials.
func TestReqPlatform017_AC03(t *testing.T) {
	script := readRepoFile(t, "deploy/bin/raid-website-backup")
	for _, want := range []string{
		"--symmetric --cipher-algo AES256",
		"sftp -q -b - ",
		"-o StrictHostKeyChecking=yes",
		"rename .upload-$archive $archive",
		`cmp -s "$work/$archive" "$work/downloaded"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("raid-website-backup lacks %q", want)
		}
	}
	settings := readRepoFile(t, "deploy/raid-website-backup.env")
	for _, line := range strings.Split(settings, "\n") {
		key, _, isAssignment := strings.Cut(strings.TrimSpace(line), "=")
		if !isAssignment || strings.HasPrefix(key, "#") {
			continue
		}
		switch key {
		case "OFFSITE_HOST", "OFFSITE_USER", "OFFSITE_PORT":
		default:
			t.Errorf("deploy/raid-website-backup.env sets unexpected key %s", key)
		}
	}
}
