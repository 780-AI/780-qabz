package fee

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

// TestCalculateBillCases are the cases of bill's pkg/helper/fee_test.go (TestGetFee and
// TestGetFee_StairMixedFixedAndPercent), with the same inputs and results.
func TestCalculateBillCases(t *testing.T) {
	mixed := `[
			{"min":1000,"max":10000,"type":"fixed","amount":150},
			{"min":10001,"max":100000,"type":"percent","percent":10}
		]`
	for _, tc := range []struct {
		name        string
		configType  string
		config      string
		amount      int64
		wantFee     int64
		wantType    string
		wantApplied string
		wantErr     string
	}{
		{"fixed fee", ConfigFixed, `{"amount":5000}`, 10000, 5000, TypeFixed, `{"amount":5000}`, ""},
		{"percent fee", ConfigPercent, `{"percent":10}`, 20000, 2000, TypePercent, `{"percent":10}`, ""},
		{"stair fixed step", ConfigStair, `[
					{"min":1000,"max":10000,"type":"fixed","amount":100},
					{"min":10001,"max":100000,"type":"fixed","amount":200}
				]`, 5000, 100, TypeStairFixed, `{"min":1000,"max":10000,"type":"fixed","amount":100}`, ""},
		{"stair percent step", ConfigStair, `[
					{"min":1000,"max":10000,"type":"percent","percent":5},
					{"min":10001,"max":100000,"type":"percent","percent":10}
				]`, 50000, 5000, TypeStairPercent, `{"min":10001,"max":100000,"type":"percent","percent":10}`, ""},
		{"stair step without type returns error", ConfigStair, `[
					{"min":1000,"max":10000,"amount":300}
				]`, 2000, 0, TypeFixed, "", "stair type is missing"},
		{"empty config returns error", "", `{}`, 10000, 0, TypeFixed, "", "config is empty"},
		{"invalid json returns error", "", `{invalid`, 10000, 0, TypeFixed, "", "invalid config json"},
		{"invalid fixed object returns error", ConfigFixed, `{"percent":10}`, 10000, 0, TypeFixed, "", "invalid config json"},
		{"invalid percent object returns error", ConfigPercent, `{"amount":5000}`, 20000, 0, TypeFixed, "", "invalid config json"},
		{"invalid stair object returns error", ConfigStair, `{"amount":5000}`, 10000, 0, TypeFixed, "", "invalid config json"},
		{"no matching stair step returns error", ConfigStair, `[
					{"min":1000,"max":10000,"type":"fixed","amount":100}
				]`, 50000, 0, TypeFixed, "", "no stair step"},
		{"mixed stair: fixed range", ConfigStair, mixed, 5000, 150, TypeStairFixed, `{"min":1000,"max":10000,"type":"fixed","amount":150}`, ""},
		{"mixed stair: percent range", ConfigStair, mixed, 50000, 5000, TypeStairPercent, `{"min":10001,"max":100000,"type":"percent","percent":10}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p fastjson.Parser
			fee, feeType, applied, err := Calculate(&p, tc.configType, []byte(tc.config), tc.amount)
			assert.Equal(t, tc.wantFee, fee)
			assert.Equal(t, tc.wantType, feeType)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantApplied, string(applied))
				return
			}
			if assert.Error(t, err) {
				assert.Contains(t, err.Error(), tc.wantErr)
			}
			assert.Nil(t, applied)
		})
	}
}

// TestCalculateDecoderRules: the rules of encoding/json v1 with DisallowUnknownFields that bill
// applies to a config (the differential test of task 039 compared them with bill's own code).
func TestCalculateDecoderRules(t *testing.T) {
	for _, tc := range []struct {
		name, configType, config string
		amount, wantFee          int64
		wantType, wantApplied    string
		wantErr                  bool
	}{
		{name: "key without case", configType: ConfigFixed, config: `{"AMOUNT":7}`, wantFee: 7, wantType: TypeFixed, wantApplied: `{"amount":7}`},
		{name: "the last key wins", configType: ConfigFixed, config: `{"amount":1,"amount":2}`, wantFee: 2, wantType: TypeFixed, wantApplied: `{"amount":2}`},
		{name: "a last null clears the field", configType: ConfigFixed, config: `{"amount":1,"amount":null}`, wantErr: true},
		{name: "null amount", configType: ConfigPercent, config: `{"percent":null}`, wantErr: true},
		{name: "unknown key", configType: ConfigFixed, config: `{"amount":1,"x":2}`, wantErr: true},
		{name: "a fraction", configType: ConfigFixed, config: `{"amount":1.0}`, wantErr: true},
		{name: "an exponent", configType: ConfigFixed, config: `{"amount":1e3}`, wantErr: true},
		{name: "out of int64", configType: ConfigFixed, config: `{"amount":9223372036854775808}`, wantErr: true},
		{name: "a string number", configType: ConfigFixed, config: `{"amount":"5"}`, wantErr: true},
		{name: "an array for fixed", configType: ConfigFixed, config: `[1]`, wantErr: true},
		{name: "white space only around", configType: ConfigFixed, config: " \n{\"amount\":3}\t", wantFee: 3, wantType: TypeFixed, wantApplied: `{"amount":3}`},
		{name: "null text", configType: ConfigFixed, config: ` null `, wantErr: true},
		{name: "empty array", configType: ConfigStair, config: `[ ]`, wantErr: true, amount: 5},
		{name: "unknown config type", configType: "other", config: `{"amount":1}`, wantErr: true},
		{name: "trailing text", configType: ConfigFixed, config: `{"amount":1} x`, wantErr: true},
		{name: "negative percent rounds toward zero", configType: ConfigPercent, config: `{"percent":-3}`, amount: 50, wantFee: -1, wantType: TypePercent, wantApplied: `{"percent":-3}`},
		{name: "percent rounds down", configType: ConfigPercent, config: `{"percent":10}`, amount: 123456, wantFee: 12345, wantType: TypePercent, wantApplied: `{"percent":10}`},
		{name: "min and max are inclusive, first step wins", configType: ConfigStair, amount: 1000000,
			config:  `[{"max":1000000,"min":0,"type":"fixed","amount":1000},{"max":1000000000,"min":1000000,"type":"percent","percent":2}]`,
			wantFee: 1000, wantType: TypeStairFixed, wantApplied: `{"min":0,"max":1000000,"type":"fixed","amount":1000}`},
		{name: "a null step is the zero step", configType: ConfigStair, config: `[null]`, amount: 0, wantErr: true},
		{name: "step keys without case and null keeps zero", configType: ConfigStair, amount: 5,
			config:  `[{"MIN":null,"Max":10,"TYPE":"percent","Percent":50,"amount":null}]`,
			wantFee: 2, wantType: TypeStairPercent, wantApplied: `{"min":0,"max":10,"type":"percent","percent":50}`},
		{name: "step type is case sensitive", configType: ConfigStair, config: `[{"min":0,"max":10,"type":"Fixed","amount":1}]`, wantErr: true},
		{name: "step type not a string", configType: ConfigStair, config: `[{"min":0,"max":10,"type":1}]`, wantErr: true},
		{name: "step min above max", configType: ConfigStair, config: `[{"min":11,"max":10,"type":"fixed","amount":1}]`, wantErr: true},
		{name: "step unknown key", configType: ConfigStair, config: `[{"min":0,"max":10,"type":"fixed","amount":1,"x":1}]`, wantErr: true},
		{name: "step not an object", configType: ConfigStair, config: `[1]`, wantErr: true},
		{name: "stair fixed with no amount is 0", configType: ConfigStair, amount: 5,
			config:  `[{"min":0,"max":10,"type":"fixed"}]`,
			wantFee: 0, wantType: TypeStairFixed, wantApplied: `{"min":0,"max":10,"type":"fixed"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p fastjson.Parser
			fee, feeType, applied, err := Calculate(&p, tc.configType, []byte(tc.config), tc.amount)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Equal(t, int64(0), fee)
				assert.Equal(t, TypeFixed, feeType)
				assert.Nil(t, applied)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantFee, fee)
			assert.Equal(t, tc.wantType, feeType)
			assert.Equal(t, tc.wantApplied, string(applied))
		})
	}
}

func TestCalculateNoParser(t *testing.T) {
	_, _, _, err := Calculate(nil, ConfigFixed, []byte(`{"amount":1}`), 1)
	assert.Error(t, err)
}

// stubRepo is a fees table in memory. A nil error field means the call works.
type stubRepo struct {
	userIDs                         []string
	userProvider, user, providerRow *Row
	errUserIDs, errUP, errU, errP   error
	calls                           []string
}

func (s *stubRepo) UserIDs(context.Context) ([]string, error) {
	s.calls = append(s.calls, "user_ids")
	return s.userIDs, s.errUserIDs
}

func (s *stubRepo) FindUserProvider(_ context.Context, _ string, _ int64) (*Row, error) {
	s.calls = append(s.calls, "user_provider")
	return s.userProvider, s.errUP
}

func (s *stubRepo) FindUser(context.Context, string) (*Row, error) {
	s.calls = append(s.calls, "user")
	return s.user, s.errU
}

func (s *stubRepo) FindProvider(context.Context, int64) (*Row, error) {
	s.calls = append(s.calls, "provider")
	return s.providerRow, s.errP
}

type recLogger struct{ lines []string }

func (l *recLogger) Error(msg string, _ ...any) { l.lines = append(l.lines, msg) }

func ptr(n int64) *int64 { return &n }

// TestResolveLookup: each lookup case of bill's FeeResolver.Resolve gives the row of bill, the fee
// and the source, and the metric callback gets the source once.
func TestResolveLookup(t *testing.T) {
	up := &Row{ID: 1, Type: ConfigFixed, Config: []byte(`{"amount":100}`)}
	u := &Row{ID: 2, Type: ConfigFixed, Config: []byte(`{"amount":200}`)}
	p := &Row{ID: 3, Type: ConfigPercent, Config: []byte(`{"percent":10}`)}
	bad := &Row{ID: 4, Type: ConfigFixed, Config: []byte(`{"x":1}`)}
	boom := errors.New("boom")
	for _, tc := range []struct {
		name       string
		repo       *stubRepo
		user       string
		providerID int64
		want       Result
		wantErr    bool
		wantCalls  []string
		wantLog    int
	}{
		{name: "provider 0 is the default", repo: &stubRepo{providerRow: p}, user: "u1", providerID: 0,
			want: Result{Fee: DefaultFee, Source: SourceDefault}},
		{name: "user and provider row", repo: &stubRepo{userIDs: []string{"u1"}, userProvider: up, user: u, providerRow: p}, user: "u1", providerID: 26,
			want: Result{Fee: 100, Type: TypeFixed, Config: []byte(`{"amount":100}`), ID: ptr(1), Source: SourceUserProvider}, wantCalls: []string{"user_ids", "user_provider"}},
		{name: "user row", repo: &stubRepo{userIDs: []string{"u1"}, user: u, providerRow: p}, user: "u1", providerID: 26,
			want: Result{Fee: 200, Type: TypeFixed, Config: []byte(`{"amount":200}`), ID: ptr(2), Source: SourceUser}, wantCalls: []string{"user_ids", "user_provider", "user"}},
		{name: "user with a fee but no row of it: provider row", repo: &stubRepo{userIDs: []string{"u1"}, providerRow: p}, user: "u1", providerID: 26,
			want: Result{Fee: 50000, Type: TypePercent, Config: []byte(`{"percent":10}`), ID: ptr(3), Source: SourceProvider}, wantCalls: []string{"user_ids", "user_provider", "user", "provider"}},
		{name: "user without a fee: provider row only", repo: &stubRepo{userIDs: []string{"other"}, userProvider: up, providerRow: p}, user: "u1", providerID: 26,
			want: Result{Fee: 50000, Type: TypePercent, Config: []byte(`{"percent":10}`), ID: ptr(3), Source: SourceProvider}, wantCalls: []string{"user_ids", "provider"}},
		{name: "no user: no user lookup", repo: &stubRepo{userIDs: []string{""}, userProvider: up, providerRow: p}, user: "", providerID: 26,
			want: Result{Fee: 50000, Type: TypePercent, Config: []byte(`{"percent":10}`), ID: ptr(3), Source: SourceProvider}, wantCalls: []string{"provider"}},
		{name: "no row is the default", repo: &stubRepo{}, user: "u1", providerID: 26,
			want: Result{Fee: DefaultFee, Source: SourceDefault}, wantCalls: []string{"user_ids", "provider"}},
		{name: "failed user list: provider row, one log line", repo: &stubRepo{errUserIDs: boom, userProvider: up, providerRow: p}, user: "u1", providerID: 26,
			want: Result{Fee: 50000, Type: TypePercent, Config: []byte(`{"percent":10}`), ID: ptr(3), Source: SourceProvider}, wantCalls: []string{"user_ids", "provider"}, wantLog: 1},
		{name: "failed user and provider read is the default", repo: &stubRepo{userIDs: []string{"u1"}, errUP: boom}, user: "u1", providerID: 26,
			want: Result{Fee: DefaultFee, Source: SourceLookupError}, wantErr: true, wantCalls: []string{"user_ids", "user_provider"}},
		{name: "failed user read is the default", repo: &stubRepo{userIDs: []string{"u1"}, errU: boom}, user: "u1", providerID: 26,
			want: Result{Fee: DefaultFee, Source: SourceLookupError}, wantErr: true, wantCalls: []string{"user_ids", "user_provider", "user"}},
		{name: "failed provider read is the default", repo: &stubRepo{errP: boom}, user: "u1", providerID: 26,
			want: Result{Fee: DefaultFee, Source: SourceLookupError}, wantErr: true, wantCalls: []string{"user_ids", "provider"}},
		{name: "bad config is 0 with the row id", repo: &stubRepo{providerRow: bad}, user: "u1", providerID: 26,
			want: Result{Fee: 0, Type: TypeFixed, ID: ptr(4), Source: SourceCalcError}, wantErr: true, wantCalls: []string{"user_ids", "provider"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sources []string
			log := &recLogger{}
			r, err := NewResolver(Options{Repository: tc.repo, Logger: log, OnResolve: func(s string) { sources = append(sources, s) }})
			if !assert.NoError(t, err) || !assert.NotNil(t, r) {
				return
			}
			got, err := r.Resolve(context.Background(), tc.user, tc.providerID, 500000)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantErr, err != nil, err)
			assert.Equal(t, tc.wantCalls, tc.repo.calls)
			assert.Equal(t, []string{tc.want.Source}, sources)
			assert.Len(t, log.lines, tc.wantLog)
		})
	}
}

func TestResolverNeedsRepository(t *testing.T) {
	r, err := NewResolver(Options{})
	assert.Error(t, err)
	assert.Nil(t, r)
	var none *Resolver
	got, err := none.Resolve(context.Background(), "u", 1, 1)
	assert.Error(t, err)
	assert.Equal(t, DefaultFee, got.Fee)
}

// TestResolveNilContextAndNoCallbacks: a nil context and no logger or callback do not panic.
func TestResolveNilContextAndNoCallbacks(t *testing.T) {
	r, err := NewResolver(Options{Repository: &stubRepo{errUserIDs: errors.New("x")}})
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		return
	}
	//nolint:staticcheck // a nil context is the case under test
	got, err := r.Resolve(nil, "u1", 26, 1000)
	assert.NoError(t, err)
	assert.Equal(t, Result{Fee: DefaultFee, Source: SourceDefault}, got)
}
