package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// providerRow is one row of listProvidersSQL as the fake gives it; updated nil is a NULL.
type providerRow struct {
	id                                   int64
	name, subject, billType, action, typ string
	priority, timeout                    int
	status                               string
	updated                              *time.Time
	useNats                              bool
	inquiryStep                          int
	providerFee                          int64
}

// fakeProviderRows gives rows to ListProviders. Scan of row scanErrAt (1-based; 0: none) fails with
// scanErr; Err answers err. The embedded nil pgx.Rows gives the other methods (a call panics).
type fakeProviderRows struct {
	pgx.Rows

	rows      []providerRow
	next      int
	scanErrAt int
	scanErr   error
	err       error
	closed    bool
}

func (r *fakeProviderRows) Close()     { r.closed = true }
func (r *fakeProviderRows) Err() error { return r.err }

func (r *fakeProviderRows) Next() bool {
	if r.closed || r.next >= len(r.rows) {
		return false
	}
	r.next++
	return true
}

// Scan checks the type of each destination, in the column order of listProvidersSQL.
func (r *fakeProviderRows) Scan(dest ...any) error {
	if r.scanErrAt == r.next {
		return r.scanErr
	}
	if r.next < 1 || r.next > len(r.rows) || len(dest) != 13 {
		return errors.New("bad Scan call")
	}
	row := r.rows[r.next-1]
	set := func(i int, v any) bool {
		switch p := dest[i].(type) {
		case *int64:
			x, ok := v.(int64)
			if ok && p != nil {
				*p = x
			}
			return ok && p != nil
		case *int:
			x, ok := v.(int)
			if ok && p != nil {
				*p = x
			}
			return ok && p != nil
		case *string:
			x, ok := v.(string)
			if ok && p != nil {
				*p = x
			}
			return ok && p != nil
		case *bool:
			x, ok := v.(bool)
			if ok && p != nil {
				*p = x
			}
			return ok && p != nil
		case **time.Time:
			x, ok := v.(*time.Time)
			if ok && p != nil {
				*p = x
			}
			return ok && p != nil
		}
		return false
	}
	values := []any{row.id, row.name, row.subject, row.billType, row.action, row.typ, row.priority, row.timeout,
		row.status, row.updated, row.useNats, row.inquiryStep, row.providerFee}
	for i, v := range values {
		if !set(i, v) {
			return errors.New("bad Scan destination type")
		}
	}
	return nil
}

// fakeProviderDB answers Query with rows and err and records the call.
type fakeProviderDB struct {
	rows  pgx.Rows
	err   error
	sql   string
	args  []any
	calls int
}

func (f *fakeProviderDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.calls++
	f.sql, f.args = sql, args
	return f.rows, f.err
}

func TestListProvidersSQL(t *testing.T) {
	for _, col := range []string{"COALESCE(name, '')", "COALESCE(subject, '')", "COALESCE(bill_type, '')",
		"COALESCE(action, '')", "COALESCE(type, '')", "COALESCE(priority, 0)", "COALESCE(timeout, 0)",
		"COALESCE(status, '')", "updated_at", "COALESCE(use_nats, false)", "COALESCE(inquiry_step, 0)",
		"COALESCE(provider_fee, 0)", "FROM providers"} {
		assert.Contains(t, listProvidersSQL, col)
	}
	assert.True(t, strings.HasPrefix(listProvidersSQL, "SELECT id, "))
	assert.NotContains(t, listProvidersSQL, "WHERE", "every row, enabled and disabled")
	assert.NotContains(t, listProvidersSQL, "ORDER BY", "the service sorts")
}

func TestListProvidersScansEachColumn(t *testing.T) {
	when := time.Date(2022, 5, 23, 3, 33, 28, 988789000, time.UTC)
	rows := &fakeProviderRows{rows: []providerRow{
		{id: 2, name: "tavanir", subject: "tavanir-inquiry", billType: "electricity", action: "inquiry", typ: "retryable",
			priority: 9, timeout: 100, status: "enable", updated: &when, useNats: true, inquiryStep: 1, providerFee: 250},
		{id: 123456, name: "karashop", billType: "telmidterm", action: "inquiry", priority: -3, status: ""}, // NULL updated_at
	}}
	db := &fakeProviderDB{rows: rows}
	got, err := (&ProviderRepository{db: db}).ListProviders(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []provider.Provider{
		{ID: 2, Name: "tavanir", Subject: "tavanir-inquiry", BillType: "electricity", Action: "inquiry", Type: "retryable",
			Priority: 9, Timeout: 100, Status: "enable", UpdatedAt: when, UseNats: true, InquiryStep: 1, ProviderFee: 250},
		{ID: 123456, Name: "karashop", BillType: "telmidterm", Action: "inquiry", Priority: -3},
	}, got, "TmpPriority is not set by the repository")
	assert.Equal(t, listProvidersSQL, db.sql)
	assert.Empty(t, db.args)
	assert.True(t, rows.closed)

	got, err = (&ProviderRepository{db: &fakeProviderDB{rows: &fakeProviderRows{}}}).ListProviders(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []provider.Provider{}, got, "no rows: an empty list, not nil")
}

func TestListProvidersErrors(t *testing.T) {
	boom := errors.New("boom")
	good := providerRow{id: 1, billType: "gas", action: "order"}
	for name, db := range map[string]*fakeProviderDB{
		"query":   {err: boom},
		"scan":    {rows: &fakeProviderRows{rows: []providerRow{good, good}, scanErrAt: 2, scanErr: boom}},
		"rows":    {rows: &fakeProviderRows{rows: []providerRow{good}, err: boom}},
		"nilrows": {},
	} {
		got, err := (&ProviderRepository{db: db}).ListProviders(context.Background())
		assert.Error(t, err, name)
		if name != "nilrows" {
			assert.ErrorIs(t, err, boom, name)
		}
		assert.Nil(t, got, name)
	}

	var nilRepo *ProviderRepository
	_, err := nilRepo.ListProviders(context.Background())
	assert.ErrorContains(t, err, "no database pool")
	_, err = NewProviderRepository(nil).ListProviders(context.Background())
	assert.ErrorContains(t, err, "no database pool")
	_, err = NewProviderRepository(&Client{}).ListProviders(context.Background())
	assert.ErrorContains(t, err, "no database pool")
}
