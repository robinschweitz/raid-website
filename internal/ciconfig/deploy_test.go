package ciconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostScripts are the root-run scripts that deploy/README.md installs into
// /usr/local/sbin (SC-037, REQ-PLATFORM-013).
var hostScripts = []string{
	"raid-website-backup",
	"raid-website-configure-ufw",
	"raid-website-publish",
	"raid-website-update",
}

// TestReqPlatform013_AC01: the host scripts are committed under deploy/bin,
// .gitignore does not hide that folder, and deploy/README.md installs each one.
func TestReqPlatform013_AC01(t *testing.T) {
	readme := readRepoFile(t, "deploy/README.md")
	for _, name := range hostScripts {
		path := filepath.Join(repoRoot, "deploy", "bin", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		script := string(data)
		if !strings.HasPrefix(script, "#!/bin/sh\nset -eu\n") {
			t.Errorf("%s must start with #!/bin/sh and set -eu (LF line endings)", name)
		}
		install := "deploy/bin/" + name + " /usr/local/sbin/" + name
		if !strings.Contains(readme, install) {
			t.Errorf("deploy/README.md does not install %s", install)
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(readRepoFile(t, ".gitignore")))
	for scanner.Scan() {
		pattern := strings.TrimSpace(scanner.Text())
		switch pattern {
		case "bin", "bin/", "bin/*", "deploy/", "deploy/bin", "deploy/bin/", "deploy/bin/*", "**/bin", "**/bin/":
			t.Errorf(".gitignore pattern %q hides deploy/bin", pattern)
		}
	}
}
