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
	grpcserver "qabz/entry/grpc"
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

// TestLiveMyBillListEndToEnd is the integration test of GET /v1/mybill: bills saved with POST
// /v1/mybill and rows of another user and a disabled row (direct SQL) are in one bills table (a
// throw-away schema of a real Postgres). GET returns only the enabled bills of the token's user,
// newest first, with the values that POST sent and the ids that POST returned. The list is also
// compared with a direct SQL query on bills. It needs QABZ_TEST_POSTGRES_DSN.
func TestLiveMyBillListEndToEnd(t *testing.T) {
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
	store, err := savedbill.NewStore(postgres.NewSavedBillRepository(db))
	if !assert.NoError(t, err) || !assert.NotNil(t, store) {
		t.FailNow()
	}
	gw := myBillsStackDeps(t, grpcserver.MyBillsDeps{Bills: store, Lister: store, ServiceID: 105})

	// No bills yet: HTTP 200 and an empty list.
	code, body := getMyBills(t, gw, true)
	if assert.Equal(t, http.StatusOK, code, string(body)) {
		assert.Empty(t, decodeBills(t, body).GetData())
	}

	// Three bills through POST, oldest first.
	posts := []string{
		`{"type":"water","billId":"123455","name":"آب خانه"}`,
		`{"type":"mobile","phone":"09121234567","operator":"irancell"}`,
		`{"type":"gas","billId":"500003","participateCode":"7788990","name":"گاز"}`,
	}
	var saved []*pb.BillResponse
	for _, p := range posts {
		code, body := postMyBill(t, gw, p, true)
		if !assert.Equal(t, http.StatusOK, code, string(body)) {
			t.FailNow()
		}
		saved = append(saved, decodeBill(t, body))
	}
	// Another user's bill and a disabled bill of this user: not in the list.
	_, err = db.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, name, type, bill_id, status, data) VALUES
		('00000000-0000-4000-8000-0000000000a1', '0b9d2c4e-8a3f-4e61-b7d2-5c1a9e0f3d22', 'other', 'water', '123455', 'enable', '123455'),
		('00000000-0000-4000-8000-0000000000a2', $1, 'deleted', 'electricity', '123455', 'disable', '123455')`, authtest.UserID)
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	code, body = getMyBills(t, gw, true)
	if !assert.Equal(t, http.StatusOK, code, string(body)) {
		return
	}
	res := decodeBills(t, body)
	assert.Equal(t, int32(105), res.GetServiceId())
	got := res.GetData()
	if !assert.Len(t, got, 3, string(body)) {
		return
	}
	// Newest first: the reverse of the POST order, with the POST ids and values.
	gas, mobile, water := got[0], got[1], got[2]
	assert.Equal(t, saved[2].GetId(), gas.GetId())
	assert.Equal(t, saved[1].GetId(), mobile.GetId())
	assert.Equal(t, saved[0].GetId(), water.GetId())

	assert.Equal(t, "آب خانه", water.GetName())
	assert.Equal(t, pb.BillResponse_water, water.GetType())
	assert.Equal(t, "123455", water.GetBillId())

	assert.Equal(t, "قبض همراه اول", mobile.GetName(), "the default name of POST")
	assert.Equal(t, pb.BillResponse_mobile, mobile.GetType())
	assert.Equal(t, "989121234567", mobile.GetPhone(), "the phone as stored (normalized by POST)")
	assert.Equal(t, "989121234567", mobile.GetBillId())
	assert.Equal(t, "irancell", mobile.GetOperator())

	assert.Equal(t, "گاز", gas.GetName())
	assert.Equal(t, pb.BillResponse_gas, gas.GetType())
	assert.Equal(t, "500003", gas.GetBillId())
	assert.Equal(t, "7788990", gas.GetParticipateCode())

	// A direct SQL query on bills gives the same ids in the same order.
	rows, err := db.Pool().Query(ctx, `SELECT id::text FROM bills WHERE user_id = $1 AND status = 'enable'
		ORDER BY created_at DESC, id DESC`, authtest.UserID)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	var ids []string
	for rows.Next() {
		var id string
		if assert.NoError(t, rows.Scan(&id)) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	assert.NoError(t, rows.Err())
	assert.Equal(t, []string{gas.GetId(), mobile.GetId(), water.GetId()}, ids)

	// No token: HTTP 403.
	code, body = getMyBills(t, gw, false)
	assert.Equal(t, http.StatusForbidden, code, string(body))
}
