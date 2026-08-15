package migrations

import (
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The embedded filesystem is the schema. A migration that is present on disk
// but missing from the binary, or a pair whose down step was never written,
// fails at deploy time rather than here — which is far too late. These tests
// are cheap and turn both into compile-and-test failures.

var migrationName = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.(up|down)\.sql$`)

func migrationFiles(t *testing.T) []string {
	t.Helper()

	entries, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	sort.Strings(entries)
	return entries
}

// The go:embed pattern matches nothing if the files are missing, which is a
// build error — but an embed that silently picks up zero files after a
// refactor would leave a binary that migrates nothing at all.
func TestFS_IsNotEmpty(t *testing.T) {
	files := migrationFiles(t)
	require.NotEmpty(t, files, "no migrations are embedded; the binary carries no schema")
}

func TestFS_EveryMigrationHasBothDirections(t *testing.T) {
	directions := map[string]map[string]bool{}

	for _, name := range migrationFiles(t) {
		matches := migrationName.FindStringSubmatch(path.Base(name))
		require.NotNil(t, matches,
			"%q does not match golang-migrate's <version>_<name>.<direction>.sql convention; "+
				"it will be ignored or misordered", name)

		version, subject, direction := matches[1], matches[2], matches[3]
		key := version + "_" + subject
		if directions[key] == nil {
			directions[key] = map[string]bool{}
		}
		directions[key][direction] = true
	}

	for migration, seen := range directions {
		assert.True(t, seen["up"], "%s has no .up.sql", migration)
		// A missing down step means the migration cannot be rolled back, which
		// is only discovered when a rollback is already needed.
		assert.True(t, seen["down"], "%s has no .down.sql, so it cannot be rolled back", migration)
	}
}

// Versions must be unique and contiguous from 1. golang-migrate orders by
// version, so a duplicate makes the order ambiguous and a gap usually means a
// migration was renamed or lost.
func TestFS_VersionsAreUniqueAndContiguous(t *testing.T) {
	seen := map[int]string{}

	for _, name := range migrationFiles(t) {
		matches := migrationName.FindStringSubmatch(path.Base(name))
		require.NotNil(t, matches, name)
		if matches[3] != "up" {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		require.NoError(t, err)

		if previous, clash := seen[version]; clash {
			t.Errorf("version %d is used by both %q and %q", version, previous, name)
		}
		seen[version] = name
	}

	for i := 1; i <= len(seen); i++ {
		assert.Contains(t, seen, i, "migration versions must run 1..n with no gaps; %d is missing", i)
	}
}

// An empty .sql file applies cleanly and does nothing, so it would leave the
// schema silently incomplete rather than failing.
func TestFS_NoMigrationIsEmpty(t *testing.T) {
	for _, name := range migrationFiles(t) {
		contents, err := fs.ReadFile(FS, name)
		require.NoError(t, err)

		// Comments alone are not a migration.
		var statements []string
		for _, line := range strings.Split(string(contents), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				statements = append(statements, trimmed)
			}
		}
		assert.NotEmpty(t, statements, "%q contains no statements", name)
	}
}

// The first migration must create the two tables everything else depends on.
// This is a smoke test against the embedded content, not the database: it
// catches an embed that picked up the wrong directory entirely.
func TestFS_FirstMigrationCreatesTheCoreTables(t *testing.T) {
	contents, err := fs.ReadFile(FS, "000001_create_hospitals_and_patients.up.sql")
	require.NoError(t, err, "the first migration is not embedded under the expected name")

	sql := strings.ToLower(string(contents))
	assert.Contains(t, sql, "create table hospitals")
	assert.Contains(t, sql, "create table patients")
}
