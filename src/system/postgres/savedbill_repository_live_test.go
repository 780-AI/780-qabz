package postgres_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/savedbill"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

const (
	liveUser  = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"
	liveOther = "0b9d2c4e-8a3f-4e61-b7d2-5c1a9e0f3d22"
)

// billRow is one bills row as a direct SQL query reads it. A NULL is nil.
type billRow struct {
	userID, name, typ, billID, data, status *string
	jsonData                                *string
	createdSet                              bool
}

// readBillRow reads the row with id by a direct query, not through the repository. It stops the
// test if the query fails.
func readBillRow(t *testing.T, ctx context.Context, db *postgres.Client, id string) billRow {
	t.Helper()
	var r billRow
	err := db.Pool().QueryRow(ctx, `SELECT user_id::text, name, type, bill_id, data, status, json_data::text,
		created_at IS NOT NULL FROM bills WHERE id = $1`, id).
		Scan(&r.userID, &r.name, &r.typ, &r.billID, &r.data, &r.status, &r.jsonData, &r.createdSet)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return r
}

func ptr(s string) *string { return &s }

// liveBills returns a client on a throw-away schema with an empty bills table and a repository on
// it. It skips the test without QABZ_TEST_POSTGRES_DSN and stops it if a step fails.
func liveBills(t *testing.T, ctx context.Context) (*postgres.Client, *postgres.SavedBillRepository) {
	t.Helper()
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	db, err := postgres.New(ctx, postgrestest.BillsSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)
	repo := postgres.NewSavedBillRepository(db)
	if !assert.NotNil(t, repo) {
		t.FailNow()
	}
	return db, repo
}

// TestLiveSavedBillInsert inserts bills and checks each row two ways: a direct SQL query on bills
// and a lookup through the repository. The values must match what was saved.
func TestLiveSavedBillInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, repo := liveBills(t, ctx)

	cases := []struct {
		name     string
		bill     savedbill.Bill
		wantJSON *string
		wantBill *string // bill_id; nil is NULL
	}{
		{"water, no operator", savedbill.Bill{UserID: liveUser, Name: "خانه", Type: "water", BillID: "123455", Data: "123455", Status: "enable"},
			nil, ptr("123455")},
		{"mobile with operator, no bill id", savedbill.Bill{UserID: liveUser, Name: "قبض همراه اول", Type: "mobile", Data: "989121234567", Operator: "mci", Status: "enable"},
			ptr(`{"Phone": "", "IsPinned": false, "Operator": "mci"}`), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, created, err := repo.Insert(ctx, c.bill)
			if !assert.NoError(t, err) || !assert.True(t, created) || !assert.NotEmpty(t, id) {
				return
			}

			row := readBillRow(t, ctx, db, id) // direct SQL
			assert.Equal(t, ptr(c.bill.UserID), row.userID)
			assert.Equal(t, ptr(c.bill.Name), row.name)
			assert.Equal(t, ptr(c.bill.Type), row.typ)
			assert.Equal(t, c.wantBill, row.billID)
			assert.Equal(t, ptr(c.bill.Data), row.data)
			assert.Equal(t, ptr("enable"), row.status)
			assert.Equal(t, c.wantJSON, row.jsonData)
			assert.True(t, row.createdSet)

			got, err := repo.Get(ctx, id) // through the repository
			assert.NoError(t, err)
			want := c.bill
			want.ID = id
			assert.Equal(t, want, got)
		})
	}

	_, err := repo.Get(ctx, "00000000-0000-4000-8000-000000000000")
	assert.ErrorIs(t, err, savedbill.ErrNotFound)
}

// TestLiveSavedBillDuplicates: a bill that the user already has (same bill_id or same data, any
// type) is not saved again; a bill of another user is.
func TestLiveSavedBillDuplicates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, repo := liveBills(t, ctx)

	first := savedbill.Bill{UserID: liveUser, Name: "a", Type: "water", BillID: "123455", Data: "123455", Status: "enable"}
	id, created, err := repo.Insert(ctx, first)
	if !assert.NoError(t, err) || !assert.True(t, created) {
		return
	}

	sameBillID := savedbill.Bill{UserID: liveUser, Name: "b", Type: "electricity", BillID: "123455", Data: "other", Status: "enable"}
	sameData := savedbill.Bill{UserID: liveUser, Name: "c", Type: "gas", BillID: "500003", Data: "123455", Status: "enable"}
	for _, b := range []savedbill.Bill{first, sameBillID, sameData} {
		got, created, err := repo.Insert(ctx, b)
		assert.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, id, got)
	}

	other := first
	other.UserID = liveOther
	otherID, created, err := repo.Insert(ctx, other)
	assert.NoError(t, err)
	assert.True(t, created)
	assert.NotEqual(t, id, otherID)

	var n int
	assert.NoError(t, db.Pool().QueryRow(ctx, `SELECT count(*) FROM bills`).Scan(&n))
	assert.Equal(t, 2, n)
	name := readBillRow(t, ctx, db, id).name
	assert.Equal(t, ptr("a"), name) // the existing row is not changed
}

// TestLiveSavedBillDisabledConflict: a disabled row with the same user, type and bill_id breaks
// the unique key and is not found by the enabled-only lookup: an error, as in bill.
func TestLiveSavedBillDisabledConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, repo := liveBills(t, ctx)

	b := savedbill.Bill{UserID: liveUser, Name: "a", Type: "water", BillID: "123455", Data: "123455", Status: "enable"}
	id, _, err := repo.Insert(ctx, b)
	if !assert.NoError(t, err) {
		return
	}
	_, err = db.Pool().Exec(ctx, `UPDATE bills SET status = 'disable' WHERE id = $1`, id)
	if !assert.NoError(t, err) {
		return
	}
	_, created, err := repo.Insert(ctx, b)
	assert.ErrorContains(t, err, "unique key")
	assert.False(t, created)
	var n int
	assert.NoError(t, db.Pool().QueryRow(ctx, `SELECT count(*) FROM bills`).Scan(&n))
	assert.Equal(t, 1, n)
}

// TestLiveSavedBillList inserts bills for two users (enabled and disabled rows, rows with the same
// created_at, a NULL created_at, the json_data shapes of the real data) and reads them with
// List. The result must match a direct SQL query on bills, value by value and in order, and the
// order that bill gives (created_at DESC; Qabz breaks a tie by id DESC).
func TestLiveSavedBillList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, repo := liveBills(t, ctx)

	// Two bills through the repository (as POST /v1/mybill saves them).
	saved, _, err := repo.Insert(ctx, savedbill.Bill{UserID: liveUser, Name: "آب", Type: "water", BillID: "123455", Data: "123455", Status: "enable"})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	savedMobile, _, err := repo.Insert(ctx, savedbill.Bill{UserID: liveUser, Name: "همراه", Type: "mobile", Data: "989121234567", Operator: "mci", Status: "enable"})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for id, at := range map[string]string{saved: "2026-01-05 10:00", savedMobile: "2026-01-06 10:00"} {
		_, err = db.Pool().Exec(ctx, `UPDATE bills SET created_at = $2 WHERE id = $1`, id, at)
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	}

	// Rows as the real data has them, by direct SQL.
	_, err = db.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, name, type, bill_id, status, last_order_date, data, json_data, created_at) VALUES
	('00000000-0000-4000-8000-000000000001', '`+liveUser+`', 'tel', 'tel', '02112345678', 'enable', '1403/02/01', NULL,
		'{"operator": "tci", "isPined": true}', '2026-01-04 10:00'),
	('00000000-0000-4000-8000-000000000002', '`+liveUser+`', 'gas a', 'gas', '999', 'enable', NULL, '1234567',
		'{"Phone": "", "IsPinned": true, "Operator": ""}', '2026-01-03 10:00'),
	('00000000-0000-4000-8000-000000000003', '`+liveUser+`', 'gas b', 'gas', '998', 'enable', NULL, NULL,
		'null', '2026-01-03 10:00'),
	('00000000-0000-4000-8000-000000000004', '`+liveUser+`', 'no date', 'electricity', '123455', 'enable', NULL, '123455',
		NULL, NULL),
	('00000000-0000-4000-8000-000000000005', '`+liveUser+`', 'deleted', 'water', '111', 'disable', NULL, '111',
		NULL, '2026-01-09 10:00'),
	('00000000-0000-4000-8000-000000000006', '`+liveOther+`', 'other', 'water', '123455', 'enable', NULL, '123455',
		NULL, '2026-01-08 10:00')`)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	// Eight bills with one created_at, inserted in mixed id order: only the id DESC tiebreak gives
	// the expected order (Postgres returns tied rows in an order that depends on the table).
	var tied []string
	for _, n := range []int{13, 10, 16, 11, 17, 12, 15, 14} {
		id := fmt.Sprintf("00000000-0000-4000-8000-0000000000%d", n)
		_, err = db.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, type, bill_id, status, data, created_at)
			VALUES ($1, $2, 'water', $3, 'enable', $3, '2026-01-02 10:00')`, id, liveUser, fmt.Sprintf("tie%d", n))
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	}
	for n := 17; n >= 10; n-- {
		tied = append(tied, fmt.Sprintf("00000000-0000-4000-8000-0000000000%d", n))
	}

	got, err := repo.List(ctx, liveUser)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	ids := make([]string, 0, len(got))
	for _, b := range got {
		ids = append(ids, b.ID)
	}
	// NULL created_at first (DESC puts NULLs first, in bill too); the same created_at: id DESC.
	assert.Equal(t, append([]string{"00000000-0000-4000-8000-000000000004", savedMobile, saved,
		"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000003",
		"00000000-0000-4000-8000-000000000002"}, tied...), ids)

	// A direct SQL query on bills, independent of the repository's query.
	sqlRows, err := db.Pool().Query(ctx, `SELECT id::text, user_id::text, COALESCE(name, ''), COALESCE(type, ''),
		COALESCE(bill_id, ''), COALESCE(data, ''), COALESCE(last_order_date, ''), status,
		COALESCE(json_data->>'Operator', json_data->>'operator', ''), COALESCE((json_data->>'IsPinned')::boolean, false)
		FROM bills WHERE user_id = $1 AND status = 'enable' ORDER BY created_at DESC NULLS FIRST, id DESC`, liveUser)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	var want []savedbill.Bill
	for sqlRows.Next() {
		var b savedbill.Bill
		if !assert.NoError(t, sqlRows.Scan(&b.ID, &b.UserID, &b.Name, &b.Type, &b.BillID, &b.Data, &b.LastOrderDate, &b.Status, &b.Operator, &b.IsPinned)) {
			t.FailNow()
		}
		want = append(want, b)
	}
	sqlRows.Close()
	assert.NoError(t, sqlRows.Err())
	assert.Equal(t, want, got)

	// Spot checks of the json_data shapes.
	if assert.Len(t, got, 6+len(tied)) {
		assert.Equal(t, "mci", got[1].Operator)
		assert.Equal(t, "tci", got[3].Operator, "a lower-case key is read, as encoding/json in bill reads it")
		assert.False(t, got[3].IsPinned, "isPined is not IsPinned")
		assert.True(t, got[5].IsPinned)
		assert.Equal(t, "1403/02/01", got[3].LastOrderDate)
	}

	// The other user sees only the own bill; a user with no bills gets an empty list.
	other, err := repo.List(ctx, liveOther)
	if assert.NoError(t, err) && assert.Len(t, other, 1) {
		assert.Equal(t, "00000000-0000-4000-8000-000000000006", other[0].ID)
	}
	none, err := repo.List(ctx, "9c1a7f0e-1d2b-4c3d-8e4f-5a6b7c8d9e0f")
	assert.NoError(t, err)
	assert.Empty(t, none)

	// A json_data value that bill cannot read fails the whole list, as in bill.
	_, err = db.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, type, bill_id, status, json_data)
		VALUES ('00000000-0000-4000-8000-000000000007', '`+liveOther+`', 'mobile', '9121112233', 'enable', '{"IsPinned": "yes"}')`)
	if assert.NoError(t, err) {
		_, err = repo.List(ctx, liveOther)
		assert.ErrorContains(t, err, "00000000-0000-4000-8000-000000000007")
	}
}

// liveBillRows reads every bills row by a direct query: id -> the whole row as JSON text. It stops
// the test if the query fails.
func liveBillRows(t *testing.T, ctx context.Context, db *postgres.Client) map[string]string {
	t.Helper()
	rows, err := db.Pool().Query(ctx, `SELECT id::text, row_to_json(b)::text FROM bills b`)
	if !assert.NoError(t, err) || !assert.NotNil(t, rows) {
		t.FailNow()
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, row string
		if !assert.NoError(t, rows.Scan(&id, &row)) {
			t.FailNow()
		}
		out[id] = row
	}
	if !assert.NoError(t, rows.Err()) {
		t.FailNow()
	}
	return out
}

// TestLiveSavedBillDisable deletes bills on a real bills table and checks every row by a direct
// SQL query after each call: only the target row changes, only its status, to disable; no row is
// removed. Each id form that Postgres accepts finds the row; urn:uuid: is a database error, as in
// bill.
func TestLiveSavedBillDisable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, repo := liveBills(t, ctx)

	const (
		target   = "00000000-0000-4000-8000-0000000000b1" // enabled bill of liveUser
		sibling  = "00000000-0000-4000-8000-0000000000b2" // another enabled bill of liveUser
		disabled = "00000000-0000-4000-8000-0000000000b3" // a disabled bill of liveUser
		other    = "00000000-0000-4000-8000-0000000000b4" // an enabled bill of liveOther
		upper    = "00000000-0000-4000-8000-0000000000c1"
		braces   = "00000000-0000-4000-8000-0000000000c2"
		noHyphen = "00000000-0000-4000-8000-0000000000c3"
		nullType = "00000000-0000-4000-8000-0000000000c4" // an enabled bill of liveUser with a NULL type
	)
	_, err := db.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, name, type, bill_id, status, last_order_date, data, json_data, created_at) VALUES
	('`+target+`', '`+liveUser+`', 'آب', 'water', '123455', 'enable', '1403/01/01', '123455', '{"Operator": "", "Phone": "", "IsPinned": true}', '2026-01-01 10:00'),
	('`+sibling+`', '`+liveUser+`', 'برق', 'electricity', '123455', 'enable', NULL, '123455', NULL, '2026-01-02 10:00'),
	('`+disabled+`', '`+liveUser+`', 'old', 'gas', '999', 'disable', NULL, '999', NULL, '2026-01-03 10:00'),
	('`+other+`', '`+liveOther+`', 'other', 'water', '123455', 'enable', NULL, '123455', NULL, '2026-01-04 10:00'),
	('`+upper+`', '`+liveUser+`', 'u', 'water', 'u1', 'enable', NULL, 'u1', NULL, '2026-01-05 10:00'),
	('`+braces+`', '`+liveUser+`', 'b', 'water', 'b1', 'enable', NULL, 'b1', NULL, '2026-01-06 10:00'),
	('`+noHyphen+`', '`+liveUser+`', 'n', 'water', 'n1', 'enable', NULL, 'n1', NULL, '2026-01-07 10:00'),
	('`+nullType+`', '`+liveUser+`', 'x', NULL, 'x1', 'enable', NULL, 'x1', NULL, '2026-01-08 10:00')`)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	want := liveBillRows(t, ctx, db)
	if !assert.Len(t, want, 8) {
		t.FailNow()
	}
	disable := func(id string) {
		want[id] = strings.Replace(want[id], `"status":"enable"`, `"status":"disable"`, 1)
	}

	cases := []struct {
		name, user, id string
		found          bool   // Postgres counts a matched row, also one that was disabled already
		typ            string // the returned bills.type ("" for NULL or no row)
		disables       string // the row that changes; "" for none
	}{
		{"own bill", liveUser, target, true, "water", target},
		{"again: already disabled", liveUser, target, true, "water", ""},
		{"a disabled bill", liveUser, disabled, true, "gas", ""},
		{"a bill of another user", liveUser, other, false, "", ""},
		{"unknown id", liveUser, "00000000-0000-4000-8000-0000000000ff", false, "", ""},
		{"upper case", liveUser, strings.ToUpper(upper), true, "water", upper},
		{"braces", liveUser, "{" + braces + "}", true, "water", braces},
		{"no hyphens", liveUser, strings.ReplaceAll(noHyphen, "-", ""), true, "water", noHyphen},
		{"a NULL type", liveUser, nullType, true, "", nullType},
	}
	for _, c := range cases {
		typ, found, err := repo.Disable(ctx, c.user, c.id)
		assert.NoError(t, err, c.name)
		assert.Equal(t, c.found, found, c.name)
		assert.Equal(t, c.typ, typ, c.name)
		if c.disables != "" {
			disable(c.disables)
		}
		assert.Equal(t, want, liveBillRows(t, ctx, db), "%s: only the target row's status changes", c.name)
	}

	// Forms that uuid.Parse accepts but Postgres does not: a database error, and nothing changes.
	for _, id := range []string{"urn:uuid:" + sibling, "URN:UUID:" + sibling, " " + sibling + " ", "(" + sibling + ")"} {
		_, found, err := repo.Disable(ctx, liveUser, id)
		if assert.Error(t, err, id) {
			assert.Contains(t, err.Error(), "invalid input syntax for type uuid", id)
		}
		assert.False(t, found, id)
	}
	// A user that is not a UUID: a database error too (bill sends it to Postgres as well).
	_, _, err = repo.Disable(ctx, "not-a-user", sibling)
	assert.Error(t, err)
	assert.Equal(t, want, liveBillRows(t, ctx, db), "the failed calls change nothing")

	// List no longer returns the disabled bills; the sibling stays.
	got, err := repo.List(ctx, liveUser)
	if assert.NoError(t, err) && assert.Len(t, got, 1) {
		assert.Equal(t, sibling, got[0].ID)
	}
}
