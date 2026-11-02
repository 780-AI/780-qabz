package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPingFailsWithoutServer(t *testing.T) {
	c := New("127.0.0.1:1", "")
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.Error(t, c.Ping(ctx))
}
