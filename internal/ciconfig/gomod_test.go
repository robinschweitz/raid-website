package ciconfig

import (
	"strings"
	"testing"
)

// TestReqArch012_AC02: CI runs the tidy check and fails on a diff. The tidy
// check itself is the CI step; the test only guards that the step stays in
// the workflow (DR-11, SC-036).
func TestReqArch012_AC02(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/ci.yml")
	for _, want := range []string{"go mod tidy", "git diff --exit-code -- go.mod go.sum"} {
		if !strings.Contains(workflow, want) {
			t.Errorf("ci.yml does not contain %q", want)
		}
	}
}
