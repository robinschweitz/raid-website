package ciconfig

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The repository root is two levels above this package. The tests read the
// committed CI files, so a change that unpins an action or drops a Dependabot
// ecosystem fails the Go gates (SC-036, REQ-PLATFORM-014).
const repoRoot = "../.."

var (
	usesLine       = regexp.MustCompile(`^\s*(?:-\s+)?uses:\s*(\S+)(.*)$`)
	pinnedAction   = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	versionComment = regexp.MustCompile(`^\s+#\s*v\d+`)
	dependabotPair = regexp.MustCompile(`(?m)package-ecosystem:\s*(\S+)\s*\n\s*directory:\s*(\S+)`)
)

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestReqPlatform014_AC01: every uses: line is pinned to a full commit SHA with
// the version tag in a trailing comment.
func TestReqPlatform014_AC01(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no workflow files found")
	}

	uses := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for number, line := range strings.Split(string(data), "\n") {
			match := usesLine.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			uses++
			reference, comment := match[1], match[2]
			if !pinnedAction.MatchString(reference) {
				t.Errorf("%s:%d: %s is not pinned by a full commit SHA", file, number+1, reference)
			}
			if !versionComment.MatchString(comment) {
				t.Errorf("%s:%d: %s has no version tag comment", file, number+1, reference)
			}
		}
	}
	if uses == 0 {
		t.Fatal("no uses: lines found in the workflows")
	}
}

// TestReqPlatform014_AC02: Dependabot covers GitHub Actions, Go modules, and
// the npm project in /web.
func TestReqPlatform014_AC02(t *testing.T) {
	entries := map[string]string{}
	for _, match := range dependabotPair.FindAllStringSubmatch(readRepoFile(t, ".github/dependabot.yml"), -1) {
		entries[match[1]] = match[2]
	}
	for ecosystem, directory := range map[string]string{
		"github-actions": "/",
		"gomod":          "/",
		"npm":            "/web",
	} {
		got, ok := entries[ecosystem]
		if !ok {
			t.Errorf("dependabot.yml has no %s entry", ecosystem)
			continue
		}
		if got != directory {
			t.Errorf("dependabot.yml %s directory = %q, want %q", ecosystem, got, directory)
		}
	}
}
