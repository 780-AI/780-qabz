package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"

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

// fakeDB answers the find queries in order (rows), every Exec with execErr and every Query with
// list and listErr. It records the calls.
type fakeDB struct {
	rows     []fakeRow
	execErr  error
	queries  int
	execs    int
	execArgs []any

	list      pgx.Rows
	listErr   error
	listSQL   string
	listArgs  []any
	listCalls int
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.listCalls++
	f.listSQL = sql
	f.listArgs = args
	return f.list, f.listErr
}

// fakeRows gives each element of rows as one row of 9 text columns (the listSavedBillsSQL
// columns). Scan of row scanErrAt (1-based; 0: none) fails with scanErr; Err answers err. The
// embedded nil pgx.Rows gives the other methods; List does not call them (a call panics).
type fakeRows struct {
	pgx.Rows

	rows      [][9]string
	next      int
	scanErrAt int
	scanErr   error
	err       error
	closed    bool
}

func (r *fakeRows) Close()     { r.closed = true }
func (r *fakeRows) Err() error { return r.err }

func (r *fakeRows) Next() bool {
	if r.closed || r.next >= len(r.rows) {
		return false
	}
	r.next++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErrAt == r.next {
		return r.scanErr
	}
	if r.next < 1 || r.next > len(r.rows) || len(dest) != 9 {
		return errors.New("bad Scan call")
	}
	for i, d := range dest {
		p, ok := d.(*string)
		if !ok || p == nil {
			return errors.New("bad Scan destination")
		}
		*p = r.rows[r.next-1][i]
	}
	return nil
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

// row is one listSavedBillsSQL row: id, user_id, name, type, bill_id, data, last_order_date,
// status, json_data.
func row(id, typ, billID, data, jsonData string) [9]string {
	return [9]string{id, "u", "name " + id, typ, billID, data, "1403/01/0" + id, "enable", jsonData}
}

func TestSavedBillListMapsRows(t *testing.T) {
	rows := &fakeRows{rows: [][9]string{
		row("1", "mobile", "", "989121234567", `{"Phone": "", "IsPinned": true, "Operator": "mci"}`),
		row("2", "tel", "02112345678", "", `{"isPined": true, "operator": "tci"}`), // the one real lower-case row
		row("3", "water", "123455", "", ""),                                        // NULL json_data
		row("4", "gas", "", "1234", `null`),
	}}
	db := &fakeDB{list: rows}
	got, err := fakeRepo(db).List(context.Background(), "u")
	assert.NoError(t, err)
	assert.Equal(t, []savedbill.Bill{
		{ID: "1", UserID: "u", Name: "name 1", Type: "mobile", Data: "989121234567", Operator: "mci", IsPinned: true, LastOrderDate: "1403/01/01", Status: "enable"},
		{ID: "2", UserID: "u", Name: "name 2", Type: "tel", BillID: "02112345678", Operator: "tci", LastOrderDate: "1403/01/02", Status: "enable"},
		{ID: "3", UserID: "u", Name: "name 3", Type: "water", BillID: "123455", LastOrderDate: "1403/01/03", Status: "enable"},
		{ID: "4", UserID: "u", Name: "name 4", Type: "gas", Data: "1234", LastOrderDate: "1403/01/04", Status: "enable"},
	}, got)
	assert.Equal(t, listSavedBillsSQL, db.listSQL)
	assert.Equal(t, []any{"u"}, db.listArgs)
	assert.True(t, rows.closed, "the rows are closed")
}

func TestSavedBillListEmpty(t *testing.T) {
	got, err := fakeRepo(&fakeDB{list: &fakeRows{}}).List(context.Background(), "u")
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestSavedBillListErrors(t *testing.T) {
	boom := errors.New("boom")
	good := row("1", "water", "123455", "", "")
	cases := map[string]*fakeDB{
		"query":   {listErr: boom},
		"scan":    {list: &fakeRows{rows: [][9]string{good, good}, scanErrAt: 2, scanErr: boom}},
		"rows":    {list: &fakeRows{rows: [][9]string{good}, err: boom}},
		"nilRows": {},
	}
	for name, db := range cases {
		got, err := fakeRepo(db).List(context.Background(), "u")
		assert.Error(t, err, name)
		if name != "nilRows" {
			assert.ErrorIs(t, err, boom, name)
		}
		assert.Nil(t, got, name)
	}
}

// TestSavedBillListBadJSONData: a json_data that bill cannot read fails the whole list, as in
// bill; the error names the row.
func TestSavedBillListBadJSONData(t *testing.T) {
	rows := &fakeRows{rows: [][9]string{row("1", "water", "123455", "", ""), row("2", "mobile", "", "989121234567", `{"IsPinned": "true"}`)}}
	got, err := fakeRepo(&fakeDB{list: rows}).List(context.Background(), "u")
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "bill 2")
		assert.Contains(t, err.Error(), "IsPinned")
	}
	assert.Nil(t, got)
	assert.True(t, rows.closed)
}

func TestSavedBillListWithoutPool(t *testing.T) {
	for _, r := range []*SavedBillRepository{nil, NewSavedBillRepository(nil), NewSavedBillRepository(&Client{})} {
		got, err := r.List(context.Background(), "u")
		assert.Error(t, err)
		assert.Nil(t, got)
	}
}

// TestBillData: json_data is read as encoding/json reads it into bill's entity.Data (go-pg). The
// expected values were checked against encoding/json v1 (task 017, a scratch test outside the
// repo, because conventions § 1 forbids encoding/json here).
func TestBillData(t *testing.T) {
	type want struct {
		operator string
		pinned   bool
		err      bool
	}
	cases := map[string]want{
		``:     {},
		`null`: {},
		`{}`:   {},
		`{"Operator": "mci", "Phone": "", "IsPinned": false}`:        {operator: "mci"},
		`{"Operator": "mci", "Phone": "", "IsPinned": true}`:         {operator: "mci", pinned: true},
		`{"operator": "irancell", "isPined": true}`:                  {operator: "irancell"}, // isPined is not IsPinned
		`{"OPERATOR": "x", "ISPINNED": true}`:                        {operator: "x", pinned: true},
		`{"operator": "a", "Operator": "b"}`:                         {operator: "b"}, // the later key wins
		`{"Operator": "b", "operator": "a"}`:                         {operator: "a"},
		`{"ispinned": true, "IsPinned": false}`:                      {},
		`{"Operator": null, "IsPinned": null, "Phone": null}`:        {},
		`{"Other": 1, "Extra": {"Operator": "no"}, "Operator": "x"}`: {operator: "x"},
		`{"Operator": "\u0645\u0631"}`:                               {operator: "مر"},
		`{"Operator": "a\"b"}`:                                       {operator: `a"b`},
		`[]`:                                                         {err: true},
		`"x"`:                                                        {err: true},
		`1`:                                                          {err: true},
		`true`:                                                       {err: true},
		`{"Operator": 1}`:                                            {err: true},
		`{"operator": true}`:                                         {err: true},
		`{"Operator": {}}`:                                           {err: true},
		`{"Phone": 1}`:                                               {err: true},
		`{"phone": []}`:                                              {err: true},
		`{"IsPinned": "true"}`:                                       {err: true},
		`{"IsPinned": 1}`:                                            {err: true},
		`{"IsPinned": 0}`:                                            {err: true},
		`{"Operator": "x",`:                                          {err: true}, // not JSON (jsonb never holds this)
	}
	var pool fastjson.ParserPool
	p := pool.Get()
	if !assert.NotNil(t, p) {
		t.FailNow()
	}
	for in, w := range cases {
		operator, pinned, err := billData(p, in)
		if w.err {
			assert.Error(t, err, in)
			continue
		}
		assert.NoError(t, err, in)
		assert.Equal(t, w.operator, operator, in)
		assert.Equal(t, w.pinned, pinned, in)
	}
	_, _, err := billData(nil, `{}`)
	assert.Error(t, err, "no parser")
	_, _, err = jsonString(nil)
	assert.Error(t, err, "no value")
}
