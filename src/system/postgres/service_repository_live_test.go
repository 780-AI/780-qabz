package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/billtype"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// TestLiveListServices reads mock rows from a mock services table (bill's columns) in a throw-away
// schema, so a real services table in the same database is never touched.
func TestLiveListServices(t *testing.T) {
	dsn := postgrestest.RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schemaDSN := postgrestest.ServicesSchema(t, ctx, dsn)
	c, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer c.Close()

	_, err = c.Pool().Exec(ctx, `INSERT INTO services (id, name, type, tag, "order", is_active, lang, inquiry, icon) VALUES
		(1, 'Gas',   'gas',   '',    2, false, 'fa-IR', false, 'g.svg'),
		(2, 'Water', 'water', 'new', 1, true,  'fa-IR', true,  'w.svg'),
		(3, 'Tie',   'tel',   NULL,  2, NULL,  'fa-IR', NULL,  NULL),
		(4, 'Water', 'water', '',    1, true,  'en-US', true,  'w.svg')`)
	if !assert.NoError(t, err) {
		return
	}
	repo := postgres.NewServiceRepository(c)
	got, err := repo.ListServices(ctx, "fa-IR")
	assert.NoError(t, err)
	assert.Equal(t, []billtype.Service{
		{Name: "Water", Type: "water", Tag: "new", IsActive: true, Inquiry: true, Icon: "w.svg"},
		{Name: "Gas", Type: "gas", Tag: "", IsActive: false, Inquiry: false, Icon: "g.svg"},
		{Name: "Tie", Type: "tel"}, // NULL columns read as "" and false; id breaks the order tie
	}, got)

	got, err = repo.ListServices(ctx, "de-DE")
	assert.NoError(t, err)
	assert.Equal(t, []billtype.Service{}, got, "no rows: an empty list, not nil")

	var nilRepo *postgres.ServiceRepository
	_, err = nilRepo.ListServices(ctx, "fa-IR")
	assert.Error(t, err)
}
