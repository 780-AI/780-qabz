package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRejectsBadDSN(t *testing.T) {
	_, err := New(context.Background(), "://bad")
	assert.Error(t, err)
}

func TestPingFailsWithoutServer(t *testing.T) {
	c, err := New(context.Background(), "postgres://u:p@127.0.0.1:1/db")
	assert.NoError(t, err)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.Error(t, c.Ping(ctx))
}
