package savedbill

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testUser = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"

// fakeRepo records the bills that Insert gets and answers with id, created and err.
type fakeRepo struct {
	got     []Bill
	id      string
	created bool
	err     error
}

func (f *fakeRepo) Insert(_ context.Context, b Bill) (string, bool, error) {
	f.got = append(f.got, b)
	return f.id, f.created, f.err
}

func (f *fakeRepo) Get(context.Context, string) (Bill, error) { return Bill{}, ErrNotFound }

// newTestStore returns a Store over repo. It stops the test if NewStore fails.
func newTestStore(t *testing.T, repo *fakeRepo) *Store {
	t.Helper()
	s, err := NewStore(repo)
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	return s
}

func TestNewStoreNeedsRepository(t *testing.T) {
	s, err := NewStore(nil)
	assert.Error(t, err)
	assert.Nil(t, s)
}

// TestInsertValid: each accepted request reaches the repository as the expected row.
func TestInsertValid(t *testing.T) {
	cases := []struct {
		name string
		req  InsertRequest
		want Bill
	}{
		{"water", InsertRequest{Type: TypeWater, BillID: "123455", Name: "خانه"},
			Bill{Name: "خانه", Type: TypeWater, BillID: "123455", Data: "123455"}},
		{"electricity default name", InsertRequest{Type: TypeElectricity, BillID: "1234567890125"},
			Bill{Name: "قبض برق", Type: TypeElectricity, BillID: "1234567890125", Data: "1234567890125"}},
		{"gas with participate code", InsertRequest{Type: TypeGas, ParticipateCode: "any-code", BillID: "bad"},
			Bill{Name: "قبض گاز", Type: TypeGas, BillID: "bad", Data: "any-code"}},
		{"gas with bill id", InsertRequest{Type: TypeGas, BillID: "500003"},
			Bill{Name: "قبض گاز", Type: TypeGas, BillID: "500003", Data: "500003"}},
		{"mobile phone normalized, mci default", InsertRequest{Type: TypeMobile, Phone: "09121234567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
		{"mobile bill id as phone", InsertRequest{Type: TypeMobile, BillID: "9121234567", Operator: "irancell", Name: "n"},
			Bill{Name: "n", Type: TypeMobile, BillID: "9121234567", Data: "989121234567", Operator: "irancell"}},
		{"mobile phone wins over bill id", InsertRequest{Type: TypeMobile, BillID: "bad", Phone: "989121234567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, BillID: "bad", Data: "989121234567", Operator: OperatorMCI}},
		{"tel", InsertRequest{Type: TypeTel, Phone: "02112345678"},
			Bill{Name: "قبض تلفن", Type: TypeTel, Data: "982112345678"}},
		{"tel bill id as phone, operator kept", InsertRequest{Type: TypeTel, BillID: "2112345678", Operator: "x"},
			Bill{Name: "قبض تلفن", Type: TypeTel, BillID: "2112345678", Data: "982112345678", Operator: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{id: "new-id", created: true}
			got, created, err := newTestStore(t, repo).Insert(context.Background(), testUser, c.req)
			if !assert.NoError(t, err) || !assert.Len(t, repo.got, 1) {
				return
			}
			want := c.want
			want.UserID, want.Status = testUser, StatusEnable
			assert.Equal(t, want, repo.got[0])
			want.ID = "new-id"
			assert.Equal(t, want, got)
			assert.True(t, created)
		})
	}
}

// TestInsertInvalid: each request that breaks a rule gives ErrInvalid and does not reach the repository.
func TestInsertInvalid(t *testing.T) {
	cases := []struct {
		name string
		user string
		req  InsertRequest
	}{
		{"no user", "", InsertRequest{Type: TypeWater, BillID: "123455"}},
		{"unknown type", testUser, InsertRequest{Type: "raw", BillID: "123455"}},
		{"empty type", testUser, InsertRequest{BillID: "123455"}},
		{"water bad check digit", testUser, InsertRequest{Type: TypeWater, BillID: "123456"}},
		{"electricity empty", testUser, InsertRequest{Type: TypeElectricity}},
		{"gas no code no bill id", testUser, InsertRequest{Type: TypeGas}},
		{"gas bad bill id", testUser, InsertRequest{Type: TypeGas, BillID: "12345"}},
		{"mobile empty", testUser, InsertRequest{Type: TypeMobile}},
		{"mobile landline number", testUser, InsertRequest{Type: TypeMobile, Phone: "02112345678"}},
		{"mobile bad phone with good bill id", testUser, InsertRequest{Type: TypeMobile, Phone: "123", BillID: "09121234567"}},
		{"tel empty", testUser, InsertRequest{Type: TypeTel}},
		{"tel short", testUser, InsertRequest{Type: TypeTel, Phone: "021123"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{id: "x", created: true}
			got, created, err := newTestStore(t, repo).Insert(context.Background(), c.user, c.req)
			assert.ErrorIs(t, err, ErrInvalid)
			assert.Empty(t, repo.got)
			assert.Equal(t, Bill{}, got)
			assert.False(t, created)
		})
	}
}

func TestInsertExisting(t *testing.T) {
	repo := &fakeRepo{id: "old-id", created: false}
	got, created, err := newTestStore(t, repo).Insert(context.Background(), testUser, InsertRequest{Type: TypeWater, BillID: "123455", Name: "new name"})
	assert.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, "old-id", got.ID)
	assert.Equal(t, "new name", got.Name) // the answer echoes the request, as in bill
}

func TestInsertRepositoryError(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeRepo{err: boom}
	got, created, err := newTestStore(t, repo).Insert(context.Background(), testUser, InsertRequest{Type: TypeWater, BillID: "123455"})
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrInvalid)
	assert.False(t, created)
	assert.Equal(t, Bill{}, got)
}

func TestInsertRepositoryWithoutID(t *testing.T) {
	repo := &fakeRepo{created: true} // no id and no error: a broken repository
	_, _, err := newTestStore(t, repo).Insert(context.Background(), testUser, InsertRequest{Type: TypeWater, BillID: "123455"})
	assert.Error(t, err)
}

func TestInsertOnZeroStore(t *testing.T) {
	var s *Store
	_, _, err := s.Insert(context.Background(), testUser, InsertRequest{Type: TypeWater, BillID: "123455"})
	assert.Error(t, err)
	_, _, err = (&Store{}).Insert(context.Background(), testUser, InsertRequest{Type: TypeWater, BillID: "123455"})
	assert.Error(t, err)
}
