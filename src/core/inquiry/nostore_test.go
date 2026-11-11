package inquiry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// noStoreRig is a rig with Options.NoStore (task 031, debug.cache_inquiries: false). Every
// Repository call fails, so a call that reaches the Repository shows in the answer or in the log,
// and the tid maker fails the test.
func noStoreRig(t *testing.T) *rig {
	t.Helper()
	r := newRigStore(t, time.Hour, true)
	boom := errors.New("the repository must not be used")
	r.repo.findErr, r.repo.saveErr, r.repo.createErr, r.repo.finishErr = boom, boom, boom, boom
	r.svc.newTID = func(string) (string, error) {
		t.Error("no tid is made with NoStore")
		return "", boom
	}
	// A valid saved row of the same bill and provider: with NoStore it does not answer.
	r.repo.seed(Row{ID: "saved", UserID: userB, InquiryID: validBill, ProviderID: qabzino.ID, ProviderName: "qabzino",
		BillType: "water", InquiryType: "regular", BillID: validBill, PayID: "1000012345", Amount: 10000000}, time.Minute, false)
	return r
}

// assertNoRepositoryUse checks that the flow made no Repository call.
func assertNoRepositoryUse(t *testing.T, r *rig) {
	t.Helper()
	assert.Equal(t, 0, r.repo.findCalls, "no FindValid")
	assert.Equal(t, 0, r.repo.saveCalls, "no SaveInquiries")
	assert.Empty(t, r.repo.order, "no CreateRequest, FinishRequest or SaveInquiries")
	assert.Empty(t, r.repo.requests)
	assert.Empty(t, r.repo.finished)
	assert.Len(t, r.repo.rows, 1, "only the seeded row")
	assert.Equal(t, 0, r.log.count("error"), "no failed database write is logged")
}

func TestNoStoreOptionIsKept(t *testing.T) {
	assert.True(t, newRigStore(t, time.Hour, true).svc.noStore)
	assert.False(t, newRig(t, time.Hour).svc.noStore, "the zero value is the flow as before")
}

// With NoStore a first and a repeated call both go to the provider, also when a valid saved row
// exists, and the database is not used.
func TestNoStoreSuccessCallsProviderEveryTime(t *testing.T) {
	r := noStoreRig(t)
	for i := 1; i <= 2; i++ {
		res, err := r.ask(userA)
		if !assert.NoError(t, err, i) || !assert.Len(t, res.Inquiries, 1, i) {
			return
		}
		assert.False(t, res.FromDatabase, i)
		assert.Equal(t, validBill, res.BillID)
		assert.Equal(t, "2500012345", res.Inquiries[0].PayID, "the provider answer, not the saved row")
		assert.Equal(t, uint64(25000000), res.Inquiries[0].Amount)
		assert.Equal(t, "2030-01-02", res.Inquiries[0].PaymentDeadline)
		assert.NotEmpty(t, res.Inquiries[0].ID)
		assert.Equal(t, i, r.prov.calls, "one provider call per request")
	}
	assertNoRepositoryUse(t, r)
	assert.Equal(t, []string{"water/qabzino/provider_ok", "water/qabzino/provider_ok"}, r.inquiry, "never db_hit")
	if assert.Len(t, r.calls, 2) {
		assert.Equal(t, CallOK, r.calls[1].result)
	}
	assert.Equal(t, 2, r.log.count("info"), "the provider call line, as before")
}

// With NoStore the failed and refused answers are the same as before and write nothing.
func TestNoStoreFailedCalls(t *testing.T) {
	cases := []struct {
		name    string
		set     func(r *rig, cancel context.CancelFunc)
		result  string
		call    string
		checkEr func(t *testing.T, err error)
	}{
		{"provider down", func(r *rig, _ context.CancelFunc) {
			r.prov.resp, r.prov.err = nil, provider.ErrProviderDown
		}, ResultProviderDown, CallDown, func(t *testing.T, err error) { assert.ErrorIs(t, err, provider.ErrProviderDown) }},
		{"refused", func(r *rig, _ context.CancelFunc) {
			r.prov.resp = &provider.InquiryResponse{Status: provider.ResponseFailed, Message: "not found", Code: 7}
		}, ResultProviderRefused, CallRefused, func(t *testing.T, err error) {
			var re *RefusedError
			if assert.ErrorAs(t, err, &re) {
				assert.Equal(t, "not found", re.Message)
			}
		}},
		{"other error", func(r *rig, _ context.CancelFunc) {
			r.prov.resp, r.prov.err = nil, errors.New("bad answer")
		}, ResultError, CallError, func(t *testing.T, err error) { assert.ErrorContains(t, err, "bad answer") }},
		{"empty answer", func(r *rig, _ context.CancelFunc) {
			r.prov.resp = nil
		}, ResultError, CallError, func(t *testing.T, err error) { assert.ErrorContains(t, err, "empty answer") }},
		{"client leaves", func(r *rig, cancel context.CancelFunc) {
			r.prov.resp, r.prov.err = nil, errors.New("canceled")
			r.prov.run = func(context.Context) { cancel() }
		}, ResultCanceled, CallCanceled, func(t *testing.T, err error) { assert.ErrorIs(t, err, context.Canceled) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := noStoreRig(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.set(r, cancel)
			res, err := r.svc.Inquire(ctx, Request{BillType: "water", InquiryID: validBill, UserID: userA})
			assert.Empty(t, res.Inquiries)
			c.checkEr(t, err)
			assert.Equal(t, 1, r.prov.calls, "the valid saved row does not answer")
			assertNoRepositoryUse(t, r)
			assert.Equal(t, []string{"water/qabzino/" + c.result}, r.inquiry)
			if assert.Len(t, r.calls, 1) {
				assert.Equal(t, c.call, r.calls[0].result)
			}
		})
	}
}

// With the store on (the zero value) the same seeded rig answers from the database: the
// difference above comes from NoStore only.
func TestStoreOnAnswersFromSavedRow(t *testing.T) {
	r := newRigStore(t, time.Hour, false)
	r.repo.seed(Row{ID: "saved", UserID: userB, InquiryID: validBill, ProviderID: qabzino.ID, ProviderName: "qabzino",
		BillType: "water", InquiryType: "regular", BillID: validBill, PayID: "1000012345", Amount: 10000000}, time.Minute, false)
	res, err := r.ask(userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 1) {
		assert.True(t, res.FromDatabase)
		assert.Equal(t, "1000012345", res.Inquiries[0].PayID)
	}
	assert.Equal(t, 0, r.prov.calls)
	assert.Equal(t, 1, r.repo.findCalls)
	assert.Equal(t, []string{"save"}, r.repo.order, "the copy for the user")
	assert.Equal(t, []string{"water/qabzino/db_hit"}, r.inquiry)
}
