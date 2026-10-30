package v1

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"raid-website/internal/raidseries"
)

func TestReqArch013_AC02(t *testing.T) {
	for _, test := range []struct {
		err    error
		code   string
		status int
	}{
		{fmt.Errorf("private SQL: %w", raidseries.ErrNotFound), "series_not_found", 404},
		{fmt.Errorf("private SQL: %w", raidseries.ErrLocalTimeAmbiguous), "local_time_ambiguous", 409},
		{fmt.Errorf("private SQL: %w", raidseries.ErrInvalid), "invalid_series", 422},
	} {
		w := httptest.NewRecorder()
		if !writeSeriesError(w, test.err) || w.Code != test.status || !strings.Contains(w.Body.String(), test.code) || strings.Contains(w.Body.String(), "private SQL") {
			t.Fatalf("mapping = %d %s", w.Code, w.Body.String())
		}
	}
}

func TestReqArch013_AC03(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "err.Error()") {
			t.Errorf("%s formats an error chain; use a fixed message", path)
		}
	}
}
