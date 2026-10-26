package db

import (
	"strconv"
	"strings"
	"testing"
)

// Two branches can each add the next migration number and merge without a
// git conflict. Migrate would then run only the first file of that number,
// and silently skip the other on every database that already ran it. CI
// runs on a pull request merged with main, so this catches the clash there.
func TestMigrationNumbersAreUnique(t *testing.T) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, e := range entries {
		version, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			t.Errorf("%s: a migration's name must start with its number, e.g. 009_name.sql", e.Name())
			continue
		}
		if other, ok := seen[version]; ok {
			t.Errorf("%s and %s share number %d: renumber the new one above the highest", other, e.Name(), version)
		}
		seen[version] = e.Name()
	}
}
