package postgres_test

import (
	"context"
	"fmt"
	"os"
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
