package archrules

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// queryCall matches Bun query builders and raw SQL execution (DR-4). RunInTx is
// transaction orchestration, not a query, and is not matched.
var queryCall = regexp.MustCompile(`\.(NewSelect|NewInsert|NewUpdate|NewDelete|NewRaw|NewCreateTable|NewDropTable|ExecContext|QueryContext|QueryRowContext)\(`)

// TestReqArch010_AC01 checks that no Bun query or SQL call appears outside a
// repo.go file in the domain packages that DR-4 already covers.
func TestReqArch010_AC01(t *testing.T) {
	packages := []string{"character", "lootcatalog", "recruitment", "unclaimedloot"}
	for _, name := range packages {
		dir := filepath.Join("..", name)
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			base := entry.Name()
			if !strings.HasSuffix(base, ".go") || strings.HasSuffix(base, "_test.go") || base == "repo.go" {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if loc := queryCall.FindIndex(source); loc != nil {
				t.Errorf("%s: query call %q outside repo.go (DR-4)", path, source[loc[0]:loc[1]])
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
