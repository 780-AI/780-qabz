package httpserver

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/core/savedbill"
	grpcserver "qabz/entry/grpc"
)

// fakeBillSaver records each call. It answers ErrInvalid when invalid is set, else a bill with the
// id "fake-id" and the request's name and type.
type fakeBillSaver struct {
	mu      sync.Mutex
	users   []string
	reqs    []savedbill.InsertRequest
	invalid bool
}

func (f *fakeBillSaver) Insert(_ context.Context, userID string, req savedbill.InsertRequest) (savedbill.Bill, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userID)
	f.reqs = append(f.reqs, req)
	if f.invalid {
		return savedbill.Bill{}, false, savedbill.ErrInvalid
	}
	return savedbill.Bill{ID: "fake-id", Name: req.Name, Type: req.Type}, true, nil
}

// myBillsStack starts a gRPC server with the JWT check on, the MyBills service over saver, and a
// gateway in front of it.
func myBillsStack(t *testing.T, saver grpcserver.BillSaver) *Gateway {
	t.Helper()
	return myBillsStackDeps(t, grpcserver.MyBillsDeps{Bills: saver})
}

// myBillsStackDeps is myBillsStack with the MyBills service over deps.
func myBillsStackDeps(t *testing.T, deps grpcserver.MyBillsDeps) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	svc := grpcserver.NewMyBills(deps)
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterMyBills(svc))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterMyBillsHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

// postMyBill sends POST /v1/mybill with body, and the test token when withToken is true. It returns
// the status code and the body, and stops the test if the request fails.
func postMyBill(t *testing.T, gw *Gateway, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBill(t, gw, http.MethodPost, body, withToken)
}

// getMyBills sends GET /v1/mybill like postMyBill.
func getMyBills(t *testing.T, gw *Gateway, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBill(t, gw, http.MethodGet, "", withToken)
}

// callMyBill sends method /v1/mybill with body (none when empty), and the test token when
// withToken is true. It returns the status code and the body, and stops the test if the request
// fails.
func callMyBill(t *testing.T, gw *Gateway, method, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, method, "/v1/mybill", body, withToken)
}

// deleteMyBill sends DELETE /v1/mybill/<rawID> like postMyBill. rawID goes into the URL as given
// (escape it first when it needs escaping).
func deleteMyBill(t *testing.T, gw *Gateway, rawID string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, http.MethodDelete, "/v1/mybill/"+rawID, "", withToken)
}

// callMyBillPath is callMyBill with the URL path.
func callMyBillPath(t *testing.T, gw *Gateway, method, path, body string, withToken bool) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+gw.Addr()+path, reader)
	if !assert.NoError(t, err) || !assert.NotNil(t, req) {
		t.FailNow()
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if withToken {
		req.Header.Set("Authorization", "Bearer "+authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims()))
	}
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.NotNil(t, res.Body) {
		t.FailNow()
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return res.StatusCode, out
}

// decodeBill reads a BillResponse body. It stops the test if the body is not one.
func decodeBill(t *testing.T, body []byte) *pb.BillResponse {
	t.Helper()
	var out pb.BillResponse
	// Test code, not a hot path; protojson reads the gateway's proto JSON (conventions § 1).
	if !assert.NoError(t, protojson.Unmarshal(body, &out), string(body)) {
		t.FailNow()
	}
	return &out
}

// decodeBills reads a BillsResponse body. It stops the test if the body is not one.
func decodeBills(t *testing.T, body []byte) *pb.BillsResponse {
	t.Helper()
	var out pb.BillsResponse
	// Test code, not a hot path; protojson reads the gateway's proto JSON (conventions § 1).
	if !assert.NoError(t, protojson.Unmarshal(body, &out), string(body)) {
		t.FailNow()
	}
	return &out
}

// fakeBillLister answers List with bills and records the users.
type fakeBillLister struct {
	mu    sync.Mutex
	users []string
	bills []savedbill.Bill
}

func (f *fakeBillLister) List(_ context.Context, userID string) ([]savedbill.Bill, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userID)
	return f.bills, nil
}

// TestMyBillGetAllOverHTTP: the gateway route, the JSON names of the answer, an empty list as
// "data": [] and the HTTP codes.
func TestMyBillGetAllOverHTTP(t *testing.T) {
	f := &fakeBillLister{bills: []savedbill.Bill{
		{ID: "g1", Name: "خانه", Type: "gas", BillID: "123455", Data: "998877", Operator: "x", IsPinned: true, LastOrderDate: "1403/01/01"},
		{ID: "m1", Name: "همراه", Type: "mobile", Data: "989121234567"},
	}}
	gw := myBillsStackDeps(t, grpcserver.MyBillsDeps{Lister: f, ServiceID: 105})

	code, body := getMyBills(t, gw, true)
	if !assert.Equal(t, http.StatusOK, code, string(body)) {
		return
	}
	for _, name := range []string{`"serviceId":105`, `"data":[`, `"isPined":true`, `"participateCode":"998877"`,
		`"lastOrderDate":"1403/01/01"`, `"billId":"123455"`, `"type":"gas"`, `"phone":"989121234567"`} {
		assert.Contains(t, strings.ReplaceAll(string(body), " ", ""), name)
	}
	res := decodeBills(t, body)
	if assert.Len(t, res.GetData(), 2) {
		assert.Equal(t, "g1", res.GetData()[0].GetId())
		assert.Equal(t, pb.BillResponse_mobile, res.GetData()[1].GetType())
	}
	f.mu.Lock()
	assert.Equal(t, []string{authtest.UserID}, f.users)
	f.mu.Unlock()

	// No token: HTTP 403, the lister is not called.
	code, body = getMyBills(t, gw, false)
	assert.Equal(t, http.StatusForbidden, code, string(body))
	f.mu.Lock()
	assert.Len(t, f.users, 1)
	f.bills = nil
	f.mu.Unlock()

	// No bills: HTTP 200 and an empty list in the body.
	code, body = getMyBills(t, gw, true)
	if assert.Equal(t, http.StatusOK, code, string(body)) {
		assert.Contains(t, strings.ReplaceAll(string(body), " ", ""), `"data":[]`)
		assert.Empty(t, decodeBills(t, body).GetData())
	}
}

// TestMyBillInsertOverHTTP: the gateway route, its JSON names, the enum by name and the HTTP codes.
func TestMyBillInsertOverHTTP(t *testing.T) {
	f := &fakeBillSaver{}
	gw := myBillsStack(t, f)

	code, body := postMyBill(t, gw, `{"billId":"123455","name":"خانه","type":"water","operator":"","isPined":false,"phone":""}`, true)
	if !assert.Equal(t, http.StatusOK, code, string(body)) {
		return
	}
	res := decodeBill(t, body)
	assert.Equal(t, "fake-id", res.GetId())
	assert.Equal(t, "خانه", res.GetName())
	assert.Equal(t, pb.BillResponse_water, res.GetType())
	assert.Equal(t, "123455", res.GetBillId())
	if assert.Len(t, f.users, 1) && assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.users[0])
		assert.Equal(t, "water", f.reqs[0].Type)
	}

	// The enum also comes as a number (grpc-gateway accepts both).
	code, body = postMyBill(t, gw, `{"billId":"123455","type":4}`, true)
	if assert.Equal(t, http.StatusOK, code, string(body)) {
		assert.Equal(t, pb.BillResponse_electricity, decodeBill(t, body).GetType())
	}

	// Rejected calls never reach the store.
	calls := len(f.reqs)
	code, body = postMyBill(t, gw, `{"billId":"123455","type":"water"}`, false)
	assert.Equal(t, http.StatusForbidden, code, string(body))
	code, body = postMyBill(t, gw, `{"billId":"123455","type":7}`, true)
	assert.Equal(t, http.StatusBadRequest, code, string(body))
	code, body = postMyBill(t, gw, `{"billId":`, true)
	assert.Equal(t, http.StatusBadRequest, code, string(body))
	assert.Len(t, f.reqs, calls)

	// A rule of the store: HTTP 400.
	f.invalid = true
	code, body = postMyBill(t, gw, `{"billId":"123456","type":"water"}`, true)
	assert.Equal(t, http.StatusBadRequest, code, string(body))
	assert.Contains(t, string(body), "invalid parameter")
}

// fakeDisableRepo is a savedbill.Repository whose Disable records each call and answers with
// found. The other methods are not used by Delete.
type fakeDisableRepo struct {
	savedbill.Repository

	mu    sync.Mutex
	calls [][2]string // user, id
	found bool
}

func (f *fakeDisableRepo) Disable(_ context.Context, userID, id string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, [2]string{userID, id})
	return "water", f.found, nil
}

func (f *fakeDisableRepo) ids() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c[1])
	}
	return out
}

// TestMyBillDeleteOverHTTP: the gateway route DELETE /v1/mybill/{id}, the id from the path (escaped
// forms unescaped), bill's answers and the HTTP codes. The real store checks the id.
func TestMyBillDeleteOverHTTP(t *testing.T) {
	f := &fakeDisableRepo{found: true}
	store, err := savedbill.NewStore(f)
	if !assert.NoError(t, err) || !assert.NotNil(t, store) {
		t.FailNow()
	}
	gw := myBillsStackDeps(t, grpcserver.MyBillsDeps{Deleter: store})

	code, body := deleteMyBill(t, gw, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", true)
	if assert.Equal(t, http.StatusOK, code, string(body)) {
		var res pb.BillActionResponse
		// Test code, not a hot path; protojson reads the gateway's proto JSON (conventions § 1).
		if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
			assert.Equal(t, "خطای داخلی رخ داده است", res.GetMessage())
		}
	}
	f.mu.Lock()
	if assert.Len(t, f.calls, 1) {
		assert.Equal(t, [2]string{authtest.UserID, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"}, f.calls[0])
	}
	f.mu.Unlock()

	// Other forms reach the store as sent.
	code, body = deleteMyBill(t, gw, url.PathEscape("{8F3A1C2E-4B5D-4E6F-9A7B-1C2D3E4F5A6B}"), true)
	assert.Equal(t, http.StatusOK, code, string(body))
	code, body = deleteMyBill(t, gw, "8f3a1c2e4b5d4e6f9a7b1c2d3e4f5a6b", true)
	assert.Equal(t, http.StatusOK, code, string(body))
	assert.Equal(t, []string{"8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", "{8F3A1C2E-4B5D-4E6F-9A7B-1C2D3E4F5A6B}",
		"8f3a1c2e4b5d4e6f9a7b1c2d3e4f5a6b"}, f.ids())

	// An id that does not parse: HTTP 500 with bill's text; the repository is not called.
	calls := len(f.ids())
	code, body = deleteMyBill(t, gw, "not-a-uuid", true)
	assert.Equal(t, http.StatusInternalServerError, code, string(body))
	assert.Contains(t, string(body), "خطای داخلی رخ داده است")
	assert.Contains(t, strings.ReplaceAll(string(body), " ", ""), `"code":2`, "gRPC Unknown")
	// An empty id (DELETE /v1/mybill/): the route matches with id "", the same answer.
	code, body = deleteMyBill(t, gw, "", true)
	assert.Equal(t, http.StatusInternalServerError, code, string(body))
	assert.Contains(t, string(body), "خطای داخلی رخ داده است")
	assert.Len(t, f.ids(), calls)
	// urn:uuid: in the path is not read as a gRPC verb: the id reaches the store whole.
	code, body = deleteMyBill(t, gw, "urn:uuid:8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", true)
	assert.Equal(t, http.StatusOK, code, string(body))
	if ids := f.ids(); assert.Len(t, ids, calls+1) {
		assert.Equal(t, "urn:uuid:8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", ids[calls])
	}
	calls++

	// No token: HTTP 403 (the JWT check, task 012).
	code, body = deleteMyBill(t, gw, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", false)
	assert.Equal(t, http.StatusForbidden, code, string(body))
	assert.Len(t, f.ids(), calls)
}
