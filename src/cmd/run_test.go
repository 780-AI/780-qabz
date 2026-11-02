package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnknownCommand(t *testing.T) {
	assert.Equal(t, 2, Run([]string{"nope"}))
}

func TestMigrateWithoutAction(t *testing.T) {
	assert.Equal(t, 2, Run([]string{"migrate"}))
}

func TestStartAppDispatch(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "") // config error: exit 2 proves start-app ran
	assert.Equal(t, 2, Run(nil))
	assert.Equal(t, 2, Run([]string{"start-app"}))
}
