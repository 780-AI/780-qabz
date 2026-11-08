package httpserver

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/core/savedbill"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// TestLiveMyBillInsertEndToEnd is the integration test of POST /v1/mybill: a request goes through
// the gateway, the JWT check, the handler, the store and the repository into a bills table (a
// throw-away schema of a real Postgres). The row is then checked two ways: a direct SQL query and a
// lookup through the repository. A second POST of the same bill returns the same id and adds no
// row. It needs QABZ_TEST_POSTGRES_DSN.
func TestLiveMyBillInsertEndToEnd(t *testing.T) {
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.New(ctx, postgrestest.BillsSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()
	repo := postgres.NewSavedBillRepository(db)
	store, err := savedbill.NewStore(repo)
	if !assert.NoError(t, err) || !assert.NotNil(t, store) {
		t.FailNow()
	}
	gw := myBillsStack(t, store)

	code, body := postMyBill(t, gw, `{"type":"mobile","phone":"09121234567","billId":"","name":""}`, true)
	if !assert.Equal(t, http.StatusOK, code, string(body)) {
		return
	}
	res := decodeBill(t, body)
	id := res.GetId()
	assert.NotEmpty(t, id)
	assert.Equal(t, "قبض همراه اول", res.GetName())
	assert.Equal(t, pb.BillResponse_mobile, res.GetType())
	assert.Equal(t, "09121234567", res.GetPhone())

	// Direct SQL on the bills table.
	var user, name, typ, data, status, operator string
	var billIDNull bool
	err = db.Pool().QueryRow(ctx, `SELECT user_id::text, name, type, bill_id IS NULL, data, status, json_data->>'Operator'
		FROM bills WHERE id = $1`, id).Scan(&user, &name, &typ, &billIDNull, &data, &status, &operator)
	if assert.NoError(t, err) {
		assert.Equal(t, authtest.UserID, user)
		assert.Equal(t, "قبض همراه اول", name)
		assert.Equal(t, "mobile", typ)
		assert.True(t, billIDNull, "an empty billId is stored as NULL, as in bill")
		assert.Equal(t, "989121234567", data)
		assert.Equal(t, "enable", status)
		assert.Equal(t, "mci", operator)
	}

	// Lookup through the repository.
	got, err := repo.Get(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, savedbill.Bill{ID: id, UserID: authtest.UserID, Name: "قبض همراه اول", Type: "mobile",
		Data: "989121234567", Operator: "mci", Status: "enable"}, got)

	// The same phone again (as a bill ID this time): the same id, no new row.
	code, body = postMyBill(t, gw, `{"type":"mobile","billId":"9121234567"}`, true)
	if assert.Equal(t, http.StatusOK, code, string(body)) {
		assert.Equal(t, id, decodeBill(t, body).GetId())
	}
	// A bad bill: HTTP 400 and no row.
	code, body = postMyBill(t, gw, `{"type":"water","billId":"123456"}`, true)
	assert.Equal(t, http.StatusBadRequest, code, string(body))
	var n int
	assert.NoError(t, db.Pool().QueryRow(ctx, `SELECT count(*) FROM bills`).Scan(&n))
	assert.Equal(t, 1, n)
}
