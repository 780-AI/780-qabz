package savedbill

import (
	"context"
	"errors"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

const testUser = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"

// fakeRepo records the bills that Insert gets and answers with id, created and err. List records
// the user and answers with list and err. Disable records the user and the id and answers with
// billType, found and err.
type fakeRepo struct {
	got       []Bill
	id        string
	created   bool
	err       error
	list      []Bill
	listUsers []string
	disabled  [][2]string // user, id
	found     bool
	billType  string
}

func (f *fakeRepo) Disable(_ context.Context, userID, id string) (string, bool, error) {
	f.disabled = append(f.disabled, [2]string{userID, id})
	return f.billType, f.found, f.err
}

func (f *fakeRepo) Insert(_ context.Context, b Bill) (string, bool, error) {
	f.got = append(f.got, b)
	return f.id, f.created, f.err
}

func (f *fakeRepo) Get(context.Context, string) (Bill, error) { return Bill{}, ErrNotFound }

func (f *fakeRepo) List(_ context.Context, userID string) ([]Bill, error) {
	f.listUsers = append(f.listUsers, userID)
	return f.list, f.err
}

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
		// Task 038: every common form of a mobile number (helpers.NormalizeMobile).
		{"mobile phone with +98", InsertRequest{Type: TypeMobile, Phone: "+989121234567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
		{"mobile phone with 0098", InsertRequest{Type: TypeMobile, Phone: "00989121234567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
		{"mobile phone with separators", InsertRequest{Type: TypeMobile, Phone: "(0912) 123-4567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
		{"mobile phone in Persian digits", InsertRequest{Type: TypeMobile, Phone: "۰۹۱۲۱۲۳۴۵۶۷"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
		{"mobile bill id as phone, with +98", InsertRequest{Type: TypeMobile, BillID: "+98 912 123 4567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, BillID: "+98 912 123 4567", Data: "989121234567", Operator: OperatorMCI}},
		// bill's rule still applies: eleven bytes lose the first byte, whatever it is.
		{"mobile phone, eleven bytes, the first is not 0 (bill)", InsertRequest{Type: TypeMobile, Phone: "19121234567"},
			Bill{Name: "قبض همراه اول", Type: TypeMobile, Data: "989121234567", Operator: OperatorMCI}},
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
		{"mobile landline number with +98", testUser, InsertRequest{Type: TypeMobile, Phone: "+982112345678"}},
		{"mobile one digit short", testUser, InsertRequest{Type: TypeMobile, Phone: "0912 123 456"}},
		{"tel with +98 (the mobile rule is not for a landline)", testUser, InsertRequest{Type: TypeTel, Phone: "+982112345678"}},
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

func TestListPassesTheUser(t *testing.T) {
	want := []Bill{{ID: "b", UserID: testUser}, {ID: "a", UserID: testUser}}
	repo := &fakeRepo{list: want}
	got, err := newTestStore(t, repo).List(context.Background(), testUser)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, []string{testUser}, repo.listUsers)
}

func TestListWithoutUser(t *testing.T) {
	repo := &fakeRepo{}
	got, err := newTestStore(t, repo).List(context.Background(), "")
	assert.ErrorIs(t, err, ErrInvalid)
	assert.Nil(t, got)
	assert.Empty(t, repo.listUsers, "the repository is not called")
}

func TestListRepositoryError(t *testing.T) {
	boom := errors.New("boom")
	got, err := newTestStore(t, &fakeRepo{err: boom}).List(context.Background(), testUser)
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrInvalid)
	assert.Nil(t, got)
}

func TestListOnZeroStore(t *testing.T) {
	var s *Store
	_, err := s.List(context.Background(), testUser)
	assert.Error(t, err)
	_, err = (&Store{}).List(context.Background(), testUser)
	assert.Error(t, err)
}

// TestDeleteIDForms: every id form that uuid.Parse (github.com/google/uuid v1.6.0, as bill) accepts
// reaches the repository as sent, with the user; the result of the repository is returned.
func TestDeleteIDForms(t *testing.T) {
	for _, id := range []string{
		"8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b",          // standard
		"8F3A1C2E-4B5D-4E6F-9A7B-1C2D3E4F5A6B",          // upper case
		"{8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b}",        // braces
		"8f3a1c2e4b5d4e6f9a7b1c2d3e4f5a6b",              // no hyphens
		"urn:uuid:8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", // urn (Postgres rejects it later)
		"00000000-0000-0000-0000-000000000000",          // nil UUID: any version is accepted
		"URN:UUID:8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b", // urn in upper case
		" 8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b ",        // 38 characters: the first and the last are not checked
		"(8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b]",        // the same
	} {
		for _, found := range []bool{true, false} {
			repo := &fakeRepo{found: found, billType: "water"}
			typ, got, err := newTestStore(t, repo).Delete(context.Background(), testUser, id)
			assert.NoError(t, err, id)
			assert.Equal(t, found, got, id)
			assert.Equal(t, "water", typ, "the type of the repository is passed through: %s", id)
			assert.Equal(t, [][2]string{{testUser, id}}, repo.disabled, "the id goes as sent: %s", id)
		}
	}
}

// TestDeleteInvalid: no user, an empty id or an id that does not parse gives ErrInvalid and the
// repository is not called.
func TestDeleteInvalid(t *testing.T) {
	cases := []struct{ name, user, id string }{
		{"no user", "", "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"},
		{"empty id", testUser, ""},
		{"not a uuid", testUser, "abc"},
		{"one digit short", testUser, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6"},
		{"bad hex", testUser, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6g"},
		{"hyphen moved", testUser, "8f3a1c2e4-b5d-4e6f-9a7b-1c2d3e4f5a6b"},
		{"one space", testUser, " 8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"},
		{"braces with no hyphens", testUser, "{8f3a1c2e4b5d4e6f9a7b1c2d3e4f5a6b}"},
		{"bad urn prefix", testUser, "urn:uuix:8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"},
	}
	for _, c := range cases {
		repo := &fakeRepo{found: true, billType: "water"}
		typ, got, err := newTestStore(t, repo).Delete(context.Background(), c.user, c.id)
		assert.ErrorIs(t, err, ErrInvalid, c.name)
		assert.False(t, got, c.name)
		assert.Empty(t, typ, c.name)
		assert.Empty(t, repo.disabled, "%s: the repository is not called", c.name)
	}
}

func TestDeleteRepositoryError(t *testing.T) {
	boom := errors.New("boom")
	_, got, err := newTestStore(t, &fakeRepo{err: boom, found: true}).Delete(context.Background(), testUser, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b")
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrInvalid)
	assert.True(t, got, "the repository result is passed through")
}

func TestDeleteOnZeroStore(t *testing.T) {
	var s *Store
	_, _, err := s.Delete(context.Background(), testUser, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b")
	assert.Error(t, err)
	_, _, err = (&Store{}).Delete(context.Background(), testUser, "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b")
	assert.Error(t, err)
}

// TestDeleteMatchesBillUUIDParse is the bill reference check of the id: for every input, Delete
// accepts the id exactly when uuid.Parse of github.com/google/uuid accepts it, and the module is
// v1.6.0, the version in bill's go.mod. The inputs are each length that uuid.Parse knows (36, 38,
// 45, 32) and lengths around them, with every position changed to each probe character.
func TestDeleteMatchesBillUUIDParse(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
	if !assert.True(t, ok) || !assert.NotNil(t, info) {
		return
	}
	version := ""
	for _, m := range info.Deps {
		if m != nil && m.Path == "github.com/google/uuid" {
			version = m.Version
		}
	}
	assert.Equal(t, "v1.6.0", version, "the uuid module of bill")

	const std = "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"
	bases := []string{
		std, strings.ToUpper(std), "{" + std + "}", "urn:uuid:" + std, "URN:UUID:" + std,
		strings.ReplaceAll(std, "-", ""), std[:35], std + "0", "{" + std, std + "}",
	}
	probes := []byte{'0', 'f', 'F', 'g', '-', '{', '}', ':', ' ', 'x'}
	inputs := []string{"", "urn:uuid:", "{}"}
	for _, b := range bases {
		inputs = append(inputs, b)
		for i := range len(b) {
			for _, p := range probes {
				inputs = append(inputs, b[:i]+string(p)+b[i+1:])
			}
		}
	}
	accepted := 0
	for _, id := range inputs {
		_, refErr := uuid.Parse(id)
		repo := &fakeRepo{}
		_, _, err := newTestStore(t, repo).Delete(context.Background(), testUser, id)
		if refErr == nil {
			accepted++
			assert.NoError(t, err, "uuid.Parse accepts %q", id)
			assert.Len(t, repo.disabled, 1, id)
		} else {
			assert.ErrorIs(t, err, ErrInvalid, "uuid.Parse rejects %q", id)
			assert.Empty(t, repo.disabled, id)
		}
	}
	assert.Greater(t, accepted, 100, "both outcomes are exercised")
	assert.Less(t, accepted, len(inputs)-100, "both outcomes are exercised")
}
