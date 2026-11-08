package postgres_test

import (
	"context"
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
