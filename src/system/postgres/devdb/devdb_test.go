package devdb

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
)

// billTables is every table that the bill migrations make on a new database (schema.sql).
var billTables = []string{"bill_inquiries", "bill_inquiry_requests", "bills", "errors", "fees", "orders", "plates",
	"providers", "services", "tolls"}

// seedIDs returns the id of each VALUES row of an INSERT file (each row starts with "(id,").
func seedIDs(sql string) []string {
	var ids []string
	for _, m := range regexp.MustCompile(`(?m)^\((\d+),`).FindAllStringSubmatch(sql, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

// unseedIDs returns the ids of the "id IN (...)" list of a DELETE file.
func unseedIDs(sql string) []string {
	m := regexp.MustCompile(`id IN \(([^)]*)\)`).FindStringSubmatch(sql)
	if len(m) != 2 {
		return nil
	}
	return strings.Split(m[1], ", ")
}

func TestUnseedDeletesTheSeedIDs(t *testing.T) {
	for _, tc := range []struct {
		name         string
		seed, unseed string
		rows         int
	}{
		{"services", seedServicesSQL, unseedServicesSQL, 5},
		{"providers", seedProvidersSQL, unseedProvidersSQL, 45},
	} {
		ids := seedIDs(tc.seed)
		assert.Len(t, ids, tc.rows, tc.name)
		assert.Equal(t, ids, unseedIDs(tc.unseed), tc.name)
		assert.Contains(t, tc.seed, "INSERT INTO "+tc.name+" (", tc.name)
		assert.Contains(t, tc.seed, "ON CONFLICT (id) DO NOTHING", tc.name)
		assert.Contains(t, tc.unseed, "DELETE FROM "+tc.name+" WHERE id IN (", tc.name)
	}
}

// TestSchemaChangesNothingThatExists: the schema makes every bill table, and every CREATE has
// IF NOT EXISTS, so a second run and an existing table are safe.
func TestSchemaChangesNothingThatExists(t *testing.T) {
	re := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS "?(\w+)"?`)
	var got []string
	for _, m := range re.FindAllStringSubmatch(schemaSQL, -1) {
		got = append(got, m[1])
	}
	assert.ElementsMatch(t, billTables, got)
	assert.Equal(t, strings.Count(schemaSQL, "\nCREATE "), strings.Count(schemaSQL, "\nCREATE TABLE IF NOT EXISTS ")+
		strings.Count(schemaSQL, "\nCREATE INDEX IF NOT EXISTS "), "every CREATE has IF NOT EXISTS")
	statements := regexp.MustCompile(`(?m)^--.*$`).ReplaceAllString(schemaSQL, "") // no comments
	for _, word := range []string{"DROP", "ALTER", "DELETE", "UPDATE", "TRUNCATE", "INSERT"} {
		assert.False(t, strings.Contains(statements, word), "the schema only creates; it has %s", word)
	}
}

type fakeBeginner struct{ err error }

func (f fakeBeginner) Begin(context.Context) (pgx.Tx, error) { return nil, f.err }

func TestErrorsBeforeAnyStatement(t *testing.T) {
	ctx := context.Background()
	_, err := Seed(ctx, nil)
	assert.ErrorContains(t, err, "no database")
	_, err = Unseed(ctx, nil)
	assert.ErrorContains(t, err, "no database")

	boom := errors.New("boom")
	n, err := Seed(ctx, fakeBeginner{err: boom})
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, Counts{}, n)
	_, err = Unseed(ctx, fakeBeginner{err: boom})
	assert.ErrorIs(t, err, boom)

	_, err = Seed(ctx, fakeBeginner{}) // a nil transaction with no error
	assert.ErrorContains(t, err, "no transaction")
}
