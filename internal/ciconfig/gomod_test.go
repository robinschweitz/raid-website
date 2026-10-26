package ciconfig

import (
	"os/exec"
	"strings"
	"testing"
)

// TestReqArch012_AC01: go mod tidy would change nothing (DR-11). `go mod tidy
// -diff` prints the patch it would apply and exits 0 in both cases, so the test
// fails on any printed output.
func TestReqArch012_AC01(t *testing.T) {
	cmd := exec.Command("go", "mod", "tidy", "-diff")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy -diff: %v\n%s", err, out)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf("go.mod or go.sum is not tidy; run go mod tidy:\n%s", out)
	}
}

// TestReqArch012_AC02: CI runs the tidy check and fails on a diff.
func TestReqArch012_AC02(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/ci.yml")
	for _, want := range []string{"go mod tidy", "git diff --exit-code -- go.mod go.sum"} {
		if !strings.Contains(workflow, want) {
			t.Errorf("ci.yml does not contain %q", want)
		}
	}
}
