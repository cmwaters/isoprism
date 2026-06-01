package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// TestRequiredMigrationVersionMatchesLatestLocalMigration verifies required migration version matches latest local migration.
func TestRequiredMigrationVersionMatchesLatestLocalMigration(t *testing.T) {
	files, err := filepath.Glob("../../../supabase/migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	re := regexp.MustCompile(`^([0-9]+)_`)
	versions := make([]string, 0, len(files))
	for _, file := range files {
		name := filepath.Base(file)
		match := re.FindStringSubmatch(name)
		if match == nil {
			t.Fatalf("migration %s does not start with a numeric version", name)
		}
		versions = append(versions, match[1])
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("stat migration %s: %v", name, err)
		}
	}
	sort.Strings(versions)
	latest := versions[len(versions)-1]
	if RequiredMigrationVersion != latest {
		t.Fatalf("RequiredMigrationVersion = %s, want latest local migration %s", RequiredMigrationVersion, latest)
	}
}
