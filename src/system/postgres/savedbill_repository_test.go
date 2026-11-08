package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"qabz/core/savedbill"
)

// fakeRow answers Scan with one string value or an error.
type fakeRow struct {
	value string
	err   error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*string); ok && p != nil {
			*p = r.value
		}
	}
	return nil
}

// fakeDB answers the find queries in order (rows) and every Exec with execErr. It records the calls.
type fakeDB struct {
	rows     []fakeRow
	execErr  error
	queries  int
	execs    int
	execArgs []any
}

func (f *fakeDB) QueryRow(context.Context, string, ...any) pgx.Row {
	if f.queries >= len(f.rows) {
		f.queries++
		return fakeRow{err: errors.New("unexpected query")}
	}
	r := f.rows[f.queries]
	f.queries++
	return r
}

func (f *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.execs++
	f.execArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func fakeRepo(db *fakeDB) *SavedBillRepository {
	return &SavedBillRepository{db: db, newID: func() string { return "new-id" }}
}

var testBill = savedbill.Bill{UserID: "u", Name: "n", Type: "water", BillID: "123455", Data: "123455", Status: "enable"}

func TestSavedBillInsertNew(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}}}
	id, created, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.NoError(t, err)
	assert.Equal(t, "new-id", id)
	assert.True(t, created)
	assert.Equal(t, 1, db.execs)
	assert.Equal(t, []any{"new-id", "u", "n", "water", "123455", "enable", "123455", ""}, db.execArgs)
}

func TestSavedBillInsertExisting(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{value: "old-id"}}}
	id, created, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.NoError(t, err)
	assert.Equal(t, "old-id", id)
	assert.False(t, created)
	assert.Equal(t, 0, db.execs) // nothing is written
}

func TestSavedBillInsertFindError(t *testing.T) {
	boom := errors.New("boom")
	db := &fakeDB{rows: []fakeRow{{err: boom}}}
	_, _, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 0, db.execs)
}

// TestSavedBillInsertRace: the insert breaks the unique key because another call saved the same
// bill at the same time; the second find returns that row.
func TestSavedBillInsertRace(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}, {value: "other-id"}}, execErr: &pgconn.PgError{Code: "23505"}}
	id, created, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.NoError(t, err)
	assert.Equal(t, "other-id", id)
	assert.False(t, created)
	assert.Equal(t, 2, db.queries)
}

// TestSavedBillInsertDisabledConflict: the key is broken by a row that is not enabled, so the
// second find finds nothing: an error, as in bill (operator decision, task 016).
func TestSavedBillInsertDisabledConflict(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}, {err: pgx.ErrNoRows}}, execErr: &pgconn.PgError{Code: "23505"}}
	id, created, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.ErrorContains(t, err, "unique key")
	assert.Empty(t, id)
	assert.False(t, created)
}

func TestSavedBillInsertRaceFindError(t *testing.T) {
	boom := errors.New("boom")
	db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}, {err: boom}}, execErr: &pgconn.PgError{Code: "23505"}}
	_, _, err := fakeRepo(db).Insert(context.Background(), testBill)
	assert.ErrorIs(t, err, boom)
}

// TestSavedBillInsertOtherErrors: an error that is not a unique violation is returned with no
// second find.
func TestSavedBillInsertOtherErrors(t *testing.T) {
	for _, execErr := range []error{errors.New("boom"), &pgconn.PgError{Code: "23502"}} {
		db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}}, execErr: execErr}
		_, created, err := fakeRepo(db).Insert(context.Background(), testBill)
		assert.ErrorIs(t, err, execErr)
		assert.False(t, created)
		assert.Equal(t, 1, db.queries)
	}
}

func TestSavedBillGetNotFound(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{err: pgx.ErrNoRows}}}
	_, err := fakeRepo(db).Get(context.Background(), "id")
	assert.ErrorIs(t, err, savedbill.ErrNotFound)
}

func TestSavedBillGetError(t *testing.T) {
	boom := errors.New("boom")
	db := &fakeDB{rows: []fakeRow{{err: boom}}}
	_, err := fakeRepo(db).Get(context.Background(), "id")
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, savedbill.ErrNotFound)
}

func TestSavedBillRepositoryWithoutPool(t *testing.T) {
	for _, r := range []*SavedBillRepository{nil, NewSavedBillRepository(nil), NewSavedBillRepository(&Client{})} {
		_, _, err := r.Insert(context.Background(), testBill)
		assert.Error(t, err)
		_, err = r.Get(context.Background(), "id")
		assert.Error(t, err)
	}
}

func TestSavedBillNewIDIsUUID(t *testing.T) {
	r := NewSavedBillRepository(nil)
	if !assert.NotNil(t, r) || !assert.NotNil(t, r.newID) {
		return
	}
	a, b := r.newID(), r.newID()
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, a)
	assert.NotEqual(t, a, b)
}
