package postgres

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"qabz/migrations"
)

const deadDSN = "postgres://u:s3cret@127.0.0.1:1/db"

func TestMigratorErrorsHideThePassword(t *testing.T) {
	m := NewMigrator(deadDSN, migrations.FS)
	for name, err := range map[string]error{
		"up":    m.Up(context.Background()),
		"force": m.Force(1),
	} {
		if assert.Error(t, err, name) {
			assert.NotContains(t, err.Error(), "s3cret", name)
		}
	}
	_, _, err := m.Version()
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "s3cret")
	}
}

func TestMigratorAppendsToExistingQuery(t *testing.T) {
	m := NewMigrator(deadDSN+"?sslmode=disable", migrations.FS)
	assert.Error(t, m.Force(1))
}

func TestMigratorBadSource(t *testing.T) {
	m := NewMigrator(deadDSN, fstest.MapFS{"bad.sql": {Data: []byte("x")}})
	assert.Error(t, m.Up(context.Background()))
}
